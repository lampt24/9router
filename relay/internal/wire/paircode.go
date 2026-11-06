package wire

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
)

// PairCodePrefix marks a version-1 pairing code.
const PairCodePrefix = "9r1."

// MaxInviteTTL caps invite lifetime (same bound as Orca's pairing offers).
const MaxInviteTTL = 10 * time.Minute

// PairCode is what an admin hands to a new device. It is a secret until it
// expires: the invite admits exactly one device, and it carries the
// Cloudflare Access service token for the outer layer.
type PairCode struct {
	V         int    `json:"v"`
	URL       string `json:"u"`            // wss://host/v1/tunnel
	ServerKey string `json:"k"`            // base64url, 32 bytes, pinned by the client
	Invite    string `json:"i"`            // base64url, 32 random bytes
	Exp       int64  `json:"e"`            // unix seconds
	CFID      string `json:"ci,omitempty"` // Cloudflare Access service token id
	CFSecret  string `json:"cs,omitempty"` // Cloudflare Access service token secret
}

// Encode renders the code as a single copy-pasteable string.
func (c PairCode) Encode() string {
	c.V = 1
	b, _ := json.Marshal(c)
	return PairCodePrefix + base64.RawURLEncoding.EncodeToString(b)
}

// DecodePairCode parses and validates a pairing code.
func DecodePairCode(s string, now time.Time) (PairCode, error) {
	var c PairCode
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, PairCodePrefix) {
		return c, errors.New("not a 9r-relay pairing code")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(s, PairCodePrefix))
	if err != nil {
		return c, errors.New("pairing code is corrupted")
	}
	if err := json.Unmarshal(raw, &c); err != nil || c.V != 1 {
		return c, errors.New("pairing code is corrupted or from a newer version")
	}
	u, err := url.Parse(c.URL)
	if err != nil || (u.Scheme != "wss" && u.Scheme != "https") || u.Host == "" {
		return c, errors.New("pairing code has an invalid relay URL (must be wss://)")
	}
	if k, err := base64.RawURLEncoding.DecodeString(c.ServerKey); err != nil || len(k) != 32 {
		return c, errors.New("pairing code has an invalid server key")
	}
	if i, err := base64.RawURLEncoding.DecodeString(c.Invite); err != nil || len(i) != 32 {
		return c, errors.New("pairing code has an invalid invite")
	}
	if now.Unix() >= c.Exp {
		return c, errors.New("pairing code has expired — create a new one")
	}
	if time.Unix(c.Exp, 0).Sub(now) > MaxInviteTTL+time.Minute {
		return c, errors.New("pairing code expiry is too far in the future")
	}
	return c, nil
}

// ServerKeyBytes returns the pinned server key.
func (c PairCode) ServerKeyBytes() []byte {
	k, _ := base64.RawURLEncoding.DecodeString(c.ServerKey)
	return k
}
