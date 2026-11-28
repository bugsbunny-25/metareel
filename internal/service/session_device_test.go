package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// "Remember this device" sessions get the device lifetime and idle timeout;
// the device name is cleaned and listed.
func TestDeviceSessions(t *testing.T) {
	ctx := context.Background()
	f := newAuthFixture(t) // TTL 24 h, idle 1 h; device TTL defaults to 180 days
	if _, err := f.users.Create(ctx, "alice", "long enough pw", RoleReader); err != nil {
		t.Fatal(err)
	}
	web, err := f.sessions.Login(ctx, LoginInput{Username: "alice", Password: "long enough pw", IP: "10.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	phone, err := f.sessions.Login(ctx, LoginInput{
		Username: "alice", Password: "long enough pw", IP: "10.0.0.2",
		DeviceName: "  iPhone\x00 Duo " + strings.Repeat("x", 100), RememberDevice: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !phone.RememberDevice || web.RememberDevice {
		t.Errorf("remember_device: phone %v, web %v", phone.RememberDevice, web.RememberDevice)
	}
	if d := time.Until(phone.ExpiresAt); d < 179*24*time.Hour || d > 181*24*time.Hour {
		t.Errorf("phone session expires in %v, want about 180 days", d)
	}
	if d := time.Until(web.ExpiresAt); d > 25*time.Hour {
		t.Errorf("web session expires in %v, want about 24 h", d)
	}
	if phone.SessionID == 0 || phone.SessionID == web.SessionID || len(phone.ProbeKey) != 64 {
		t.Errorf("session ids %d/%d, probe key %q", phone.SessionID, web.SessionID, phone.ProbeKey)
	}

	list, err := f.sessions.ListForUser(ctx, phone.User.ID, phone.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	var listed *SessionResponse
	for i := range list {
		if list[i].ID == phone.SessionID {
			listed = &list[i]
		}
	}
	if listed == nil || !listed.RememberDevice || !listed.Current {
		t.Fatalf("phone session not listed as a current remembered device: %+v", list)
	}
	if !strings.HasPrefix(listed.DeviceName, "iPhone Duo x") || len([]rune(listed.DeviceName)) != maxDeviceNameLen {
		t.Errorf("device name = %q, want it cleaned and capped at %d", listed.DeviceName, maxDeviceNameLen)
	}

	// Two hours unused: past the 1 h idle timeout, inside the device one.
	if _, err := f.sqlDB.Exec(`UPDATE sessions SET last_seen_at = datetime('now', '-2 hours')`); err != nil {
		t.Fatal(err)
	}
	fresh := NewSessionService(slog.New(slog.NewTextHandler(io.Discard, nil)), f.repo,
		SessionConfig{TTL: 24 * time.Hour, IdleTimeout: time.Hour}) // no cached sessions
	if _, err := fresh.Lookup(ctx, web.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Errorf("idle web session: err = %v, want ErrSessionInvalid", err)
	}
	if _, err := fresh.Lookup(ctx, phone.Token); err != nil {
		t.Errorf("idle phone session: %v, want still valid", err)
	}
	if n, err := fresh.PruneExpired(ctx); err != nil || n != 1 {
		t.Errorf("prune = %d, %v; want the web session only", n, err)
	}
}

func TestProbe(t *testing.T) {
	ctx := context.Background()
	f := newAuthFixture(t)
	if _, err := f.users.Create(ctx, "alice", "long enough pw", RoleReader); err != nil {
		t.Fatal(err)
	}
	res, err := f.sessions.Login(ctx, LoginInput{Username: "alice", Password: "long enough pw", IP: "10.0.0.1", RememberDevice: true})
	if err != nil {
		t.Fatal(err)
	}
	key, err := hex.DecodeString(res.ProbeKey)
	if err != nil {
		t.Fatal(err)
	}
	expect := func(nonce string) string {
		m := hmac.New(sha256.New, key)
		m.Write([]byte(nonce))
		return hex.EncodeToString(m.Sum(nil))
	}
	const nonce = "abcdefghijklmnop_0123-XYZ"

	got, err := f.sessions.Probe(ctx, res.SessionID, nonce, "10.1.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Proof != expect(nonce) {
		t.Errorf("proof = %s, want HMAC(probe_key, nonce)", got.Proof)
	}

	// An unknown session gets a proof too (it doesn't say which ids exist),
	// just one that never matches.
	unknown, err := f.sessions.Probe(ctx, res.SessionID+100, nonce, "10.1.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Proof == expect(nonce) || len(unknown.Proof) != 64 {
		t.Errorf("unknown session proof = %s", unknown.Proof)
	}

	// Signing out ends the probe key with the session.
	if err := f.sessions.Logout(ctx, res.Token); err != nil {
		t.Fatal(err)
	}
	after, err := f.sessions.Probe(ctx, res.SessionID, nonce, "10.1.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if after.Proof == expect(nonce) {
		t.Error("probe still answers for a signed-out session")
	}

	var vErr *ValidationError
	for _, bad := range []string{"", "short", "has spaces in it!!", strings.Repeat("a", 129)} {
		if _, err := f.sessions.Probe(ctx, res.SessionID, bad, "10.1.0.2"); !errors.As(err, &vErr) {
			t.Errorf("nonce %q: err = %v, want ValidationError", bad, err)
		}
	}
	if _, err := f.sessions.Probe(ctx, 0, nonce, "10.1.0.2"); !errors.As(err, &vErr) {
		t.Errorf("session 0: err = %v, want ValidationError", err)
	}

	limited := false
	for range 30 {
		if _, err := f.sessions.Probe(ctx, res.SessionID, nonce, "10.1.0.3"); errors.Is(err, ErrProbeRateLimited) {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("30 quick probes from one IP were never rate limited")
	}
}
