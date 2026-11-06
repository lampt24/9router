package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flynn/noise"
	"github.com/lampt24/9router/relay/internal/wire"
)

// Device is a paired client. Its credential is the X25519 static public key.
type Device struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Pub     string     `json:"pub"` // base64url
	Created time.Time  `json:"created"`
	Revoked *time.Time `json:"revoked,omitempty"`
}

// Invite admits one device until Exp. Only the SHA-256 of the secret is stored.
type Invite struct {
	Hash string    `json:"hash"`
	Name string    `json:"name"`
	Exp  time.Time `json:"exp"`
}

// State is persisted as JSON (mode 0600).
type State struct {
	ServerPriv string   `json:"server_priv"`
	ServerPub  string   `json:"server_pub"`
	Devices    []Device `json:"devices"`
	Invites    []Invite `json:"invites"`
}

// Key returns the server static key pair.
func (s *State) Key() (noise.DHKey, error) {
	priv, err1 := base64.RawURLEncoding.DecodeString(s.ServerPriv)
	pub, err2 := base64.RawURLEncoding.DecodeString(s.ServerPub)
	if err1 != nil || err2 != nil || len(priv) != 32 || len(pub) != 32 {
		return noise.DHKey{}, errors.New("state: invalid server key")
	}
	return noise.DHKey{Private: priv, Public: pub}, nil
}

// FindDevice returns the device with this public key.
func (s *State) FindDevice(pub []byte) *Device {
	enc := base64.RawURLEncoding.EncodeToString(pub)
	for i := range s.Devices {
		if subtle.ConstantTimeCompare([]byte(s.Devices[i].Pub), []byte(enc)) == 1 {
			return &s.Devices[i]
		}
	}
	return nil
}

// Store persists State with an exclusive file lock so CLI commands
// (pair-new, revoke) can run while the server is serving.
type Store struct{ Path string }

func (st *Store) lockPath() string { return st.Path + ".lock" }

func (st *Store) read() (*State, error) {
	b, err := os.ReadFile(st.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("state: %w", err)
	}
	return &s, nil
}

func (st *Store) write(s *State) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := st.Path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return os.Rename(tmp, st.Path)
}

// Update runs fn under the lock and saves the result. A missing state file is
// initialised with a fresh server key.
func (st *Store) Update(fn func(*State) error) error {
	if err := os.MkdirAll(filepath.Dir(st.Path), 0o700); err != nil {
		return err
	}
	unlock, err := lockFile(st.lockPath())
	if err != nil {
		return err
	}
	defer unlock()
	s, err := st.read()
	if err != nil {
		return err
	}
	if s == nil {
		k, err := wire.GenerateKey()
		if err != nil {
			return err
		}
		s = &State{
			ServerPriv: base64.RawURLEncoding.EncodeToString(k.Private),
			ServerPub:  base64.RawURLEncoding.EncodeToString(k.Public),
		}
	}
	if err := fn(s); err != nil {
		return err
	}
	return st.write(s)
}

// Load returns a snapshot (initialising the file if needed).
func (st *Store) Load() (*State, error) {
	s, err := st.read()
	if err == nil && s != nil {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var out State
	err = st.Update(func(s *State) error { out = *s; return nil })
	return &out, err
}

// NewInvite records an invite and returns its secret (base64url, 32 bytes).
func (st *Store) NewInvite(name string, ttl time.Duration, now time.Time) (secret string, s State, err error) {
	if ttl <= 0 || ttl > wire.MaxInviteTTL {
		return "", s, fmt.Errorf("ttl must be between 1s and %s", wire.MaxInviteTTL)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", s, err
	}
	secret = base64.RawURLEncoding.EncodeToString(raw)
	err = st.Update(func(cur *State) error {
		cur.pruneInvites(now)
		cur.Invites = append(cur.Invites, Invite{Hash: hashInvite(secret), Name: name, Exp: now.Add(ttl)})
		s = *cur
		return nil
	})
	return secret, s, err
}

// Redeem consumes a valid invite and registers the device key. Invites are
// single use; an expired or unknown invite is rejected.
func (st *Store) Redeem(secret string, pub []byte, nameHint string, now time.Time) (Device, error) {
	var dev Device
	err := st.Update(func(s *State) error {
		s.pruneInvites(now)
		h := hashInvite(secret)
		idx := -1
		for i, inv := range s.Invites {
			if subtle.ConstantTimeCompare([]byte(inv.Hash), []byte(h)) == 1 {
				idx = i
			}
		}
		if idx < 0 {
			return wire.ErrUnauthorized
		}
		inv := s.Invites[idx]
		s.Invites = append(s.Invites[:idx], s.Invites[idx+1:]...)
		if d := s.FindDevice(pub); d != nil {
			if d.Revoked != nil {
				return wire.ErrUnauthorized // a revoked key may not re-pair; generate a new one
			}
			dev = *d
			return nil
		}
		name := inv.Name
		if name == "" {
			name = sanitizeName(nameHint)
		}
		id := make([]byte, 6)
		_, _ = rand.Read(id)
		dev = Device{ID: hex.EncodeToString(id), Name: name, Pub: base64.RawURLEncoding.EncodeToString(pub), Created: now.UTC()}
		s.Devices = append(s.Devices, dev)
		return nil
	})
	return dev, err
}

// Revoke marks a device (by id or name) as revoked.
func (st *Store) Revoke(idOrName string, now time.Time) (Device, error) {
	var dev Device
	err := st.Update(func(s *State) error {
		var match []*Device
		for i := range s.Devices {
			d := &s.Devices[i]
			if d.ID == idOrName || strings.EqualFold(d.Name, idOrName) {
				match = append(match, d)
			}
		}
		switch len(match) {
		case 0:
			return fmt.Errorf("no device %q", idOrName)
		case 1:
		default:
			return fmt.Errorf("%q matches %d devices, use the id", idOrName, len(match))
		}
		if match[0].Revoked == nil {
			t := now.UTC()
			match[0].Revoked = &t
		}
		dev = *match[0]
		return nil
	})
	return dev, err
}

func (s *State) pruneInvites(now time.Time) {
	kept := s.Invites[:0]
	for _, inv := range s.Invites {
		if now.Before(inv.Exp) {
			kept = append(kept, inv)
		}
	}
	s.Invites = kept
}

func hashInvite(secret string) string {
	sum := sha256.Sum256([]byte("9r-relay/v1/invite\x00" + secret))
	return hex.EncodeToString(sum[:])
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r != 0x7f && b.Len() < 64 {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "device"
	}
	return b.String()
}

// FindDevicePub returns the public key (base64url) of a device id.
func (s *State) FindDevicePub(id string) string {
	for _, d := range s.Devices {
		if d.ID == id {
			return d.Pub
		}
	}
	return ""
}
