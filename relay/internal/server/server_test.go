package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lampt24/9router/relay/internal/wire"
)

// echo upstream: whatever arrives is written back.
func startEcho(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	return ln.Addr().String()
}

type env struct {
	srv   *Server
	store *Store
	url   string
	key   []byte
}

func newEnv(t *testing.T) *env {
	t.Helper()
	store := &Store{Path: filepath.Join(t.TempDir(), "state.json")}
	srv := New(store, startEcho(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.RevokeCheck = 100 * time.Millisecond
	hs := httptest.NewServer(srv)
	t.Cleanup(hs.Close)
	st, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	k, _ := st.Key()
	return &env{srv: srv, store: store, url: "ws" + strings.TrimPrefix(hs.URL, "http") + wire.TunnelPath, key: k.Public}
}

func (e *env) dial(t *testing.T, dev []byte, devPub []byte, invite string, serverKey []byte) (*wire.Session, wire.Welcome, error) {
	t.Helper()
	if serverKey == nil {
		serverKey = e.key
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	kp, err := wire.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if dev != nil {
		kp.Private, kp.Public = dev, devPub
	}
	return wire.Dial(ctx, wire.DialConfig{URL: e.url, ServerKey: serverKey, Device: kp, Hello: wire.Hello{Invite: invite, Name: "test-pc"}})
}

func TestPairAndRelayData(t *testing.T) {
	e := newEnv(t)
	inv, _, err := e.store.NewInvite("company-pc", 5*time.Minute, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	dev, _ := wire.GenerateKey()

	s, w, err := e.dial(t, dev.Private, dev.Public, inv, nil)
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	if !w.Paired || w.Name != "company-pc" || w.DeviceID == "" {
		t.Fatalf("unexpected welcome %+v", w)
	}

	// 1 MiB round trip through the encrypted tunnel to the echo upstream.
	payload := make([]byte, 1<<20)
	_, _ = rand.Read(payload)
	go func() { _, _ = s.Write(payload) }()
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(s, got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("payload mismatch")
	}
	s.Close()

	// Invite is single use: a different key cannot reuse it.
	if _, _, err := e.dial(t, nil, nil, inv, nil); err == nil {
		t.Fatal("reused invite accepted")
	}
	// Paired device reconnects without an invite.
	s2, w2, err := e.dial(t, dev.Private, dev.Public, "", nil)
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if w2.Paired || w2.DeviceID != w.DeviceID {
		t.Fatalf("unexpected reconnect welcome %+v", w2)
	}
	s2.Close()
}

func TestRejections(t *testing.T) {
	e := newEnv(t)
	// Unknown device, no invite.
	if _, _, err := e.dial(t, nil, nil, "", nil); err == nil {
		t.Fatal("unknown device accepted")
	}
	// Bogus invite.
	bogus := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	if _, _, err := e.dial(t, nil, nil, bogus, nil); err == nil {
		t.Fatal("bogus invite accepted")
	}
	// Expired invite.
	inv, _, _ := e.store.NewInvite("x", time.Second, time.Now().Add(-time.Hour))
	if _, _, err := e.dial(t, nil, nil, inv, nil); err == nil {
		t.Fatal("expired invite accepted")
	}
	// Client pinned the wrong server key (MITM / wrong server): must fail.
	inv2, _, _ := e.store.NewInvite("y", time.Minute, time.Now())
	wrong, _ := wire.GenerateKey()
	if _, _, err := e.dial(t, nil, nil, inv2, wrong.Public); err == nil {
		t.Fatal("handshake succeeded with wrong pinned server key")
	}
	// ...and the invite was not consumed by that failed attempt.
	if _, _, err := e.dial(t, nil, nil, inv2, nil); err != nil {
		t.Fatalf("valid invite rejected after failed attempt: %v", err)
	}
}

func TestRevokeKillsLiveSessionAndBlocksReconnect(t *testing.T) {
	e := newEnv(t)
	inv, _, _ := e.store.NewInvite("pc", time.Minute, time.Now())
	dev, _ := wire.GenerateKey()
	s, w, err := e.dial(t, dev.Private, dev.Public, inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.Revoke(w.DeviceID, time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Done():
	case <-time.After(3 * time.Second):
		// Done fires on local close; a read error is the observable signal.
		buf := make([]byte, 1)
		if _, err := s.Read(buf); err == nil {
			t.Fatal("revoked session still open")
		}
	}
	if _, _, err := e.dial(t, dev.Private, dev.Public, "", nil); err == nil {
		t.Fatal("revoked device reconnected")
	}
	// A revoked key cannot come back through a new invite either.
	inv2, _, _ := e.store.NewInvite("pc2", time.Minute, time.Now())
	if _, _, err := e.dial(t, dev.Private, dev.Public, inv2, nil); err == nil {
		t.Fatal("revoked key re-paired")
	}
}

func TestRateLimitAfterFailures(t *testing.T) {
	e := newEnv(t)
	e.srv.FailLimit = 3
	for i := 0; i < 3; i++ {
		_, _, _ = e.dial(t, nil, nil, "", nil)
	}
	_, _, err := e.dial(t, nil, nil, "", nil)
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected HTTP 429 after repeated failures, got %v", err)
	}
}

func TestPairCodeRoundTrip(t *testing.T) {
	now := time.Now()
	key := make([]byte, 32)
	inv := make([]byte, 32)
	_, _ = rand.Read(key)
	_, _ = rand.Read(inv)
	c := wire.PairCode{URL: "wss://gw.example/v1/tunnel", ServerKey: base64.RawURLEncoding.EncodeToString(key),
		Invite: base64.RawURLEncoding.EncodeToString(inv), Exp: now.Add(5 * time.Minute).Unix(), CFID: "id", CFSecret: "sec"}
	got, err := wire.DecodePairCode(c.Encode(), now)
	if err != nil || got.CFSecret != "sec" || !bytes.Equal(got.ServerKeyBytes(), key) {
		t.Fatalf("roundtrip: %v %+v", err, got)
	}
	if _, err := wire.DecodePairCode(c.Encode(), now.Add(10*time.Minute)); err == nil {
		t.Fatal("expired code accepted")
	}
	bad := c
	bad.URL = "http://plain/v1/tunnel"
	if _, err := wire.DecodePairCode(bad.Encode(), now); err == nil {
		t.Fatal("non-wss url accepted")
	}
}
