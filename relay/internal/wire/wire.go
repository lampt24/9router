// Package wire is the 9r-relay protocol shared by client and server.
//
// Transport: one WebSocket (binary messages, no compression) per relayed TCP
// connection, reached through Cloudflare (Access service token on the HTTP
// upgrade = outer layer).
//
// Inner layer, modelled on Orca's mobile relay: a Noise_IK_25519_ChaChaPoly_SHA256
// handshake. The client pins the server's static key (delivered by the pairing
// code); the client's static key travels encrypted in message 1 and is the
// device credential — no bearer token ever crosses the wire after pairing.
// After the handshake every WebSocket message is one AEAD-sealed chunk, so the
// relay path (Cloudflare, corporate TLS inspection) only sees ciphertext.
package wire

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/flynn/noise"
)

const (
	// Prologue binds the handshake to this protocol/version.
	Prologue = "9r-relay/v1"
	// TunnelPath is the WebSocket endpoint on the server.
	TunnelPath = "/v1/tunnel"
	// MaxChunk is the plaintext size per sealed message (Noise limit is 65535 incl. tag).
	MaxChunk = 32 * 1024
	// ReadLimit bounds a single WebSocket message.
	ReadLimit = 64 * 1024
	// HandshakeTimeout bounds the Noise handshake.
	HandshakeTimeout = 15 * time.Second
	// PingInterval keeps idle tunnels alive (Cloudflare drops idle WebSockets after ~100s).
	PingInterval = 25 * time.Second

	// Close codes (4000-4999 are application defined).
	CloseUnauthorized websocket.StatusCode = 4401
	CloseRateLimited  websocket.StatusCode = 4429
	CloseUpstream     websocket.StatusCode = 4502
	CloseRevoked      websocket.StatusCode = 4403
)

// Suite is the Noise cipher suite used by both sides.
var Suite = noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256)

// Hello is the (encrypted) payload of handshake message 1.
type Hello struct {
	V      int    `json:"v"`
	Invite string `json:"invite,omitempty"` // only when pairing
	Name   string `json:"name,omitempty"`   // device name suggestion when pairing
	Client string `json:"client,omitempty"` // client version, informational
}

// Welcome is the (encrypted) payload of handshake message 2.
type Welcome struct {
	V        int    `json:"v"`
	DeviceID string `json:"device_id"`
	Name     string `json:"name,omitempty"`
	Paired   bool   `json:"paired,omitempty"`
}

// GenerateKey creates a new X25519 static key pair.
func GenerateKey() (noise.DHKey, error) { return noise.DH25519.GenerateKeypair(rand.Reader) }

// Session is an encrypted, ordered byte stream over a WebSocket.
type Session struct {
	ws   *websocket.Conn
	ctx  context.Context
	stop context.CancelFunc

	wmu  sync.Mutex
	send *noise.CipherState

	recv *noise.CipherState // only touched by the single reader
	buf  []byte

	closeOnce sync.Once
}

func newSession(ws *websocket.Conn, send, recv *noise.CipherState) *Session {
	ctx, stop := context.WithCancel(context.Background())
	s := &Session{ws: ws, ctx: ctx, stop: stop, send: send, recv: recv}
	go s.keepalive()
	return s
}

func (s *Session) keepalive() {
	t := time.NewTicker(PingInterval)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
			err := s.ws.Ping(pctx)
			cancel()
			if err != nil {
				_ = s.ws.CloseNow()
				return
			}
		}
	}
}

// Read returns decrypted bytes.
func (s *Session) Read(p []byte) (int, error) {
	for len(s.buf) == 0 {
		typ, data, err := s.ws.Read(s.ctx)
		if err != nil {
			if st := websocket.CloseStatus(err); st == websocket.StatusNormalClosure || st == websocket.StatusGoingAway {
				return 0, io.EOF
			}
			return 0, err
		}
		if typ != websocket.MessageBinary {
			return 0, errors.New("wire: unexpected text message")
		}
		pt, err := s.recv.Decrypt(nil, nil, data)
		if err != nil {
			_ = s.ws.Close(websocket.StatusPolicyViolation, "decrypt")
			return 0, fmt.Errorf("wire: decrypt: %w", err)
		}
		s.buf = pt
	}
	n := copy(p, s.buf)
	s.buf = s.buf[n:]
	return n, nil
}

// Write seals p in chunks of at most MaxChunk bytes.
func (s *Session) Write(p []byte) (int, error) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	written := 0
	for len(p) > 0 {
		n := len(p)
		if n > MaxChunk {
			n = MaxChunk
		}
		ct, err := s.send.Encrypt(nil, nil, p[:n])
		if err != nil {
			return written, err
		}
		if err := s.ws.Write(s.ctx, websocket.MessageBinary, ct); err != nil {
			return written, err
		}
		written += n
		p = p[n:]
	}
	return written, nil
}

// Close ends the session with a normal closure.
func (s *Session) Close() error { return s.CloseWith(websocket.StatusNormalClosure, "") }

// CloseWith ends the session with a specific code.
func (s *Session) CloseWith(code websocket.StatusCode, reason string) error {
	var err error
	s.closeOnce.Do(func() {
		s.stop()
		err = s.ws.Close(code, reason)
	})
	return err
}

// Done is closed when the session ends.
func (s *Session) Done() <-chan struct{} { return s.ctx.Done() }

// DialConfig configures a client connection.
type DialConfig struct {
	URL        string       // wss://host/v1/tunnel
	ServerKey  []byte       // pinned server static public key (32 bytes)
	Device     noise.DHKey  // this device's static key pair
	Header     http.Header  // e.g. Cloudflare Access service-token headers
	HTTPClient *http.Client // proxy settings etc.
	Hello      Hello
}

// Dial opens a tunnel and completes the Noise handshake.
func Dial(ctx context.Context, cfg DialConfig) (*Session, Welcome, error) {
	var w Welcome
	if len(cfg.ServerKey) != 32 {
		return nil, w, errors.New("wire: server key must be 32 bytes")
	}
	hctx, cancel := context.WithTimeout(ctx, HandshakeTimeout)
	defer cancel()
	ws, resp, err := websocket.Dial(hctx, cfg.URL, &websocket.DialOptions{
		HTTPClient:      cfg.HTTPClient,
		HTTPHeader:      cfg.Header,
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if resp != nil {
			return nil, w, fmt.Errorf("wire: websocket dial: HTTP %d: %w", resp.StatusCode, err)
		}
		return nil, w, fmt.Errorf("wire: websocket dial: %w", err)
	}
	ws.SetReadLimit(ReadLimit)

	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:   Suite,
		Random:        rand.Reader,
		Pattern:       noise.HandshakeIK,
		Initiator:     true,
		Prologue:      []byte(Prologue),
		StaticKeypair: cfg.Device,
		PeerStatic:    cfg.ServerKey,
	})
	if err != nil {
		_ = ws.CloseNow()
		return nil, w, err
	}
	cfg.Hello.V = 1
	payload, _ := json.Marshal(cfg.Hello)
	msg1, _, _, err := hs.WriteMessage(nil, payload)
	if err != nil {
		_ = ws.CloseNow()
		return nil, w, err
	}
	if err := ws.Write(hctx, websocket.MessageBinary, msg1); err != nil {
		_ = ws.CloseNow()
		return nil, w, fmt.Errorf("wire: send hello: %w", err)
	}
	typ, msg2, err := ws.Read(hctx)
	if err != nil {
		_ = ws.CloseNow()
		switch websocket.CloseStatus(err) {
		case CloseUnauthorized:
			return nil, w, errors.New("wire: rejected by server (unknown/revoked device, invalid or expired invite, or wrong server key)")
		case CloseRateLimited:
			return nil, w, errors.New("wire: rate limited by server")
		}
		return nil, w, fmt.Errorf("wire: handshake: %w", err)
	}
	if typ != websocket.MessageBinary {
		_ = ws.CloseNow()
		return nil, w, errors.New("wire: unexpected handshake message")
	}
	pl, cs1, cs2, err := hs.ReadMessage(nil, msg2)
	if err != nil {
		_ = ws.CloseNow()
		return nil, w, errors.New("wire: server failed authentication (key mismatch)")
	}
	if err := json.Unmarshal(pl, &w); err != nil || w.V != 1 || w.DeviceID == "" {
		_ = ws.CloseNow()
		return nil, w, errors.New("wire: bad welcome")
	}
	return newSession(ws, cs1, cs2), w, nil
}

// Authorizer decides whether a peer static key (and its hello) may connect.
type Authorizer func(peerStatic []byte, hello Hello) (Welcome, error)

// ErrUnauthorized is returned by Accept when the authorizer refused the peer.
var ErrUnauthorized = errors.New("wire: unauthorized")

// Accept runs the responder side of the handshake on an accepted WebSocket.
// On failure the caller is responsible for closing ws.
func Accept(ctx context.Context, ws *websocket.Conn, static noise.DHKey, authorize Authorizer) (*Session, Welcome, error) {
	var w Welcome
	ws.SetReadLimit(ReadLimit)
	hctx, cancel := context.WithTimeout(ctx, HandshakeTimeout)
	defer cancel()
	typ, msg1, err := ws.Read(hctx)
	if err != nil {
		return nil, w, err
	}
	if typ != websocket.MessageBinary {
		return nil, w, ErrUnauthorized
	}
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:   Suite,
		Random:        rand.Reader,
		Pattern:       noise.HandshakeIK,
		Initiator:     false,
		Prologue:      []byte(Prologue),
		StaticKeypair: static,
	})
	if err != nil {
		return nil, w, err
	}
	pl, _, _, err := hs.ReadMessage(nil, msg1)
	if err != nil {
		// Garbage, or the client pinned a different server key.
		return nil, w, ErrUnauthorized
	}
	var hello Hello
	if err := json.Unmarshal(pl, &hello); err != nil || hello.V != 1 {
		return nil, w, ErrUnauthorized
	}
	w, err = authorize(hs.PeerStatic(), hello)
	if err != nil {
		return nil, w, err
	}
	w.V = 1
	resp, _ := json.Marshal(w)
	msg2, cs1, cs2, err := hs.WriteMessage(nil, resp)
	if err != nil {
		return nil, w, err
	}
	if err := ws.Write(hctx, websocket.MessageBinary, msg2); err != nil {
		return nil, w, err
	}
	return newSession(ws, cs2, cs1), w, nil
}
