package server

import (
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/lampt24/9router/relay/internal/wire"
)

// Server terminates relay tunnels and forwards the decrypted stream to Upstream.
type Server struct {
	Store        *Store
	Upstream     string // e.g. 127.0.0.1:20129 (Caddy: only /v1, forces X-Forwarded-For)
	Log          *slog.Logger
	MaxSessions  int // global concurrent tunnels
	MaxPerDevice int // concurrent tunnels per device
	// FailLimit failed handshakes per FailWindow per client IP before 429.
	FailLimit  int
	FailWindow time.Duration
	// RevokeCheck is how often live sessions re-check revocation.
	RevokeCheck time.Duration
	Now         func() time.Time

	mu       sync.Mutex
	total    int
	byDevice map[string]int
	fails    map[string][]time.Time
}

// New returns a server with sensible limits.
func New(store *Store, upstream string, log *slog.Logger) *Server {
	return &Server{
		Store: store, Upstream: upstream, Log: log,
		MaxSessions: 256, MaxPerDevice: 64,
		FailLimit: 20, FailWindow: 5 * time.Minute,
		RevokeCheck: 30 * time.Second,
		Now:         time.Now,
	}
}

func (s *Server) clientIP(r *http.Request) string {
	// Only reachable through cloudflared on loopback, which sets this header.
	if ip := r.Header.Get("Cf-Connecting-Ip"); ip != "" {
		return ip
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

func (s *Server) rateLimited(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fails == nil {
		return false
	}
	cut := s.Now().Add(-s.FailWindow)
	kept := s.fails[ip][:0]
	for _, t := range s.fails[ip] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(s.fails, ip)
		return false
	}
	s.fails[ip] = kept
	return len(kept) >= s.FailLimit
}

func (s *Server) recordFail(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fails == nil {
		s.fails = map[string][]time.Time{}
	}
	if len(s.fails) > 10000 { // bound memory under a flood
		s.fails = map[string][]time.Time{}
	}
	s.fails[ip] = append(s.fails[ip], s.Now())
}

func (s *Server) acquire(device string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byDevice == nil {
		s.byDevice = map[string]int{}
	}
	if s.total >= s.MaxSessions || s.byDevice[device] >= s.MaxPerDevice {
		return false
	}
	s.total++
	s.byDevice[device]++
	return true
}

func (s *Server) release(device string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total--
	if s.byDevice[device]--; s.byDevice[device] <= 0 {
		delete(s.byDevice, device)
	}
}

// authorize admits a registered, non-revoked device or redeems an invite.
func (s *Server) authorize(peer []byte, hello wire.Hello) (wire.Welcome, error) {
	st, err := s.Store.Load()
	if err != nil {
		return wire.Welcome{}, err
	}
	if d := st.FindDevice(peer); d != nil && d.Revoked == nil {
		return wire.Welcome{DeviceID: d.ID, Name: d.Name}, nil
	}
	if hello.Invite == "" {
		return wire.Welcome{}, wire.ErrUnauthorized
	}
	d, err := s.Store.Redeem(hello.Invite, peer, hello.Name, s.Now())
	if err != nil {
		return wire.Welcome{}, wire.ErrUnauthorized
	}
	s.Log.Info("device paired", "device", d.ID, "name", d.Name)
	return wire.Welcome{DeviceID: d.ID, Name: d.Name, Paired: true}, nil
}

func (s *Server) revoked(deviceID string) bool {
	st, err := s.Store.Load()
	if err != nil {
		return false
	}
	for _, d := range st.Devices {
		if d.ID == deviceID {
			return d.Revoked != nil
		}
	}
	return true
}

// ServeHTTP handles GET /v1/tunnel (WebSocket) and /healthz.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "ok\n")
		return
	case wire.TunnelPath:
	default:
		http.NotFound(w, r)
		return
	}
	ip := s.clientIP(r)
	if s.rateLimited(ip) {
		http.Error(w, "too many failed handshakes", http.StatusTooManyRequests)
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	st, err := s.Store.Load()
	if err != nil {
		s.Log.Error("load state", "err", err)
		_ = ws.Close(websocket.StatusInternalError, "state")
		return
	}
	key, err := st.Key()
	if err != nil {
		s.Log.Error("server key", "err", err)
		_ = ws.Close(websocket.StatusInternalError, "key")
		return
	}
	sess, welcome, err := wire.Accept(r.Context(), ws, key, s.authorize)
	if err != nil {
		s.recordFail(ip)
		code := wire.CloseUnauthorized
		if !errors.Is(err, wire.ErrUnauthorized) {
			code = websocket.StatusProtocolError
		}
		s.Log.Warn("handshake rejected", "ip", ip, "err", err)
		_ = ws.Close(code, "unauthorized")
		return
	}
	if !s.acquire(welcome.DeviceID) {
		_ = sess.CloseWith(wire.CloseRateLimited, "too many sessions")
		return
	}
	defer s.release(welcome.DeviceID)

	up, err := net.DialTimeout("tcp", s.Upstream, 5*time.Second)
	if err != nil {
		s.Log.Error("upstream dial", "err", err)
		_ = sess.CloseWith(wire.CloseUpstream, "upstream unavailable")
		return
	}
	s.Log.Info("tunnel open", "device", welcome.DeviceID, "name", welcome.Name, "ip", ip)
	start := s.Now()
	pipe(sess, up, func() bool { return s.revoked(welcome.DeviceID) }, s.RevokeCheck)
	s.Log.Info("tunnel closed", "device", welcome.DeviceID, "dur", s.Now().Sub(start).Round(time.Millisecond))
}

// pipe copies both directions until either side ends, the session drops, or
// the device gets revoked.
func pipe(sess *wire.Session, up net.Conn, revoked func() bool, every time.Duration) {
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(up, sess); done <- struct{}{} }()
	go func() { _, _ = io.Copy(sess, up); done <- struct{}{} }()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-done:
			_ = up.Close()
			_ = sess.Close()
			return
		case <-sess.Done():
			_ = up.Close()
			return
		case <-t.C:
			if revoked() {
				_ = up.Close()
				_ = sess.CloseWith(wire.CloseRevoked, "revoked")
				return
			}
		}
	}
}

// PublicKey is a helper for CLIs.
func PublicKey(st *State) string { return st.ServerPub }

// KeyFingerprint shortens a base64url key for display.
func KeyFingerprint(b64 string) string {
	raw, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil || len(raw) < 8 {
		return "?"
	}
	return base64.RawURLEncoding.EncodeToString(raw[:8])
}
