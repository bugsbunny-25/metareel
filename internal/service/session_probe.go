package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"

	"golang.org/x/time/rate"
)

// The local-URL probe (docs/mobile-app-plan.md, B4).
//
// A phone may reach metareel over a plain-HTTP home address as well as the
// public HTTPS one. Before it sends its session token over HTTP, it checks
// that the server answering there is this one: it sends its session id and a
// fresh random nonce, and this server answers with
//
//	proof = HMAC-SHA256(probe_key, nonce)
//	probe_key = HMAC-SHA256(token digest, "metareel/probe/v1")
//
// The client received probe_key at sign-in (over HTTPS) and compares. Another
// device answering on that address can't produce the proof, and the token
// never travels until it has. The probe key is derived from the stored digest,
// so nothing new is stored, and signing out ends it with the session.
//
// Probes for sessions that don't exist (or have ended) get a proof from a
// per-process random key, so the endpoint doesn't tell which session ids are
// live. It is rate limited per client IP.

var (
	// ErrProbeRateLimited: too many probes from one client IP.
	ErrProbeRateLimited = errors.New("too many probes; try again later")

	probeNonceRe = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
	probeLabel   = []byte("metareel/probe/v1")
)

// ProbeResult is the server's answer to a probe.
type ProbeResult struct {
	Proof string `json:"proof"` // hex HMAC-SHA256(probe_key, nonce)
}

// probeKey derives a session's probe key from its token digest.
func probeKey(digest string) []byte {
	m := hmac.New(sha256.New, []byte(digest))
	m.Write(probeLabel)
	return m.Sum(nil)
}

func newProbeDecoyKey() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not fail on supported platforms; a fixed key
		// would only make unknown ids distinguishable, not leak anything.
		copy(b, "metareel/probe/decoy")
	}
	return b
}

// Probe answers a local-URL probe for a session id and client nonce (16-128
// base64url / hex characters).
func (s *SessionService) Probe(ctx context.Context, sessionID int64, nonce, ip string) (*ProbeResult, error) {
	if !probeNonceRe.MatchString(nonce) {
		return nil, &ValidationError{Message: "nonce must be 16-128 characters of A-Z, a-z, 0-9, '-' or '_'"}
	}
	if sessionID <= 0 {
		return nil, &ValidationError{Message: "session_id must be a positive integer"}
	}
	if !s.allowProbe(ip) {
		return nil, ErrProbeRateLimited
	}
	key := s.probeDecoy
	digest, err := s.repo.ActiveSessionDigest(ctx, sessionID, s.cfg.idle())
	switch {
	case err == nil:
		key = probeKey(digest)
	case errors.Is(err, sql.ErrNoRows):
	default:
		return nil, fmt.Errorf("probe session: %w", err)
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte(nonce))
	return &ProbeResult{Proof: hex.EncodeToString(m.Sum(nil))}, nil
}

// allowProbe allows a burst of 20 probes per client IP, then one a second.
// A phone probes on launch, network change and foreground, so this is far
// above normal use.
func (s *SessionService) allowProbe(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.probeLimits) > 10000 {
		s.probeLimits = map[string]*rate.Limiter{}
	}
	return limiterFor(s.probeLimits, ip, time.Second, 20).Allow()
}
