package service

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestPasswordHash(t *testing.T) {
	h1, err := hashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := hashPassword("correct horse battery")
	if !strings.HasPrefix(h1, "$argon2id$v=19$m=19456,t=2,p=1$") || h1 == h2 {
		t.Fatalf("hashes = %q, %q (want argon2id PHC strings with distinct salts)", h1, h2)
	}
	if ok, err := verifyPassword("correct horse battery", h1); !ok || err != nil {
		t.Fatalf("right password: ok = %v err = %v", ok, err)
	}
	if ok, err := verifyPassword("wrong horse battery", h1); ok || err != nil {
		t.Fatalf("wrong password: ok = %v err = %v", ok, err)
	}
	for _, bad := range []string{"", "plain", "$argon2i$v=19$m=1,t=1,p=1$AA$AA", "$argon2id$v=19$m=0,t=1,p=1$AA$AA", "$argon2id$v=19$m=8,t=1,p=1$!!$AA"} {
		if ok, err := verifyPassword("x", bad); ok || !errors.Is(err, errBadPasswordHash) {
			t.Errorf("verify against %q: ok = %v err = %v", bad, ok, err)
		}
	}
}

type authFixture struct {
	sqlDB    *sql.DB
	repo     *repository.UserRepository
	keys     *APIKeyService
	sessions *SessionService
	users    *UserService
	auth     *AuthService
}

func newAuthFixture(t *testing.T) authFixture {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := newTestDB(t)
	keys := NewAPIKeyService(log, repository.NewAPIKeyRepository(db))
	if err := keys.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewUserRepository(db)
	sessions := NewSessionService(log, repo, SessionConfig{TTL: 24 * time.Hour, IdleTimeout: time.Hour})
	return authFixture{sqlDB: db, repo: repo, keys: keys, sessions: sessions,
		users: NewUserService(log, repo, sessions), auth: NewAuthService(keys, sessions)}
}

func TestUserValidation(t *testing.T) {
	ctx := context.Background()
	f := newAuthFixture(t)
	var vErr *ValidationError
	for _, tc := range []struct{ name, user, pw, role string }{
		{"short password", "alice", "short", RoleReader},
		{"bad username", "al ice", "long enough pw", RoleReader},
		{"username too short", "a", "long enough pw", RoleReader},
		{"bad role", "alice", "long enough pw", "root"},
	} {
		if _, err := f.users.Create(ctx, tc.user, tc.pw, tc.role); !errors.As(err, &vErr) {
			t.Errorf("%s: err = %v, want ValidationError", tc.name, err)
		}
	}
	if _, err := f.users.Create(ctx, "Alice", "long enough pw", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err := f.users.Create(ctx, "alice", "another long pw", RoleReader); !errors.Is(err, ErrUserExists) {
		t.Fatalf("duplicate (case-insensitive): err = %v, want ErrUserExists", err)
	}
	if err := f.users.SetRole(ctx, "nobody", RoleAdmin); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("SetRole unknown: err = %v", err)
	}

	// EnsureAdmin only acts on an empty user table.
	if created, err := f.users.EnsureAdmin(ctx, "root", "long enough pw"); created || err != nil {
		t.Fatalf("EnsureAdmin with users: created = %v err = %v", created, err)
	}
	g := newAuthFixture(t)
	if created, err := g.users.EnsureAdmin(ctx, "root", "long enough pw"); !created || err != nil {
		t.Fatalf("EnsureAdmin on empty: created = %v err = %v", created, err)
	}
	if list, _ := g.users.List(ctx); len(list) != 1 || list[0].Role != RoleAdmin {
		t.Fatalf("users after EnsureAdmin = %+v", list)
	}
}

func TestLoginAndSessions(t *testing.T) {
	ctx := context.Background()
	f := newAuthFixture(t)
	if _, err := f.users.Create(ctx, "alice", "long enough pw", RoleReader); err != nil {
		t.Fatal(err)
	}
	// Another admin, so alice may be demoted or disabled later.
	if _, err := f.users.Create(ctx, "root", "long enough pw", RoleAdmin); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ user, pw string }{{"alice", "wrong password"}, {"bob", "long enough pw"}} {
		if _, err := f.sessions.Login(ctx, tc.user, tc.pw, "test", "10.0.0.1"); !errors.Is(err, ErrInvalidLogin) {
			t.Fatalf("login %s/%s: err = %v, want ErrInvalidLogin", tc.user, tc.pw, err)
		}
	}
	var vErr *ValidationError
	if _, err := f.sessions.Login(ctx, "", "", "test", "10.0.0.1"); !errors.As(err, &vErr) {
		t.Fatalf("empty login: err = %v", err)
	}

	res, err := f.sessions.Login(ctx, "ALICE", "long enough pw", "test", "10.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !strings.HasPrefix(res.Token, SessionTokenPrefix) || len(res.Token) != sessionTokenLen || res.User.Username != "alice" {
		t.Fatalf("login result = %+v", res)
	}
	if d := time.Until(res.ExpiresAt); d < 23*time.Hour || d > 25*time.Hour {
		t.Fatalf("expires_at = %v, want ~24h from now", res.ExpiresAt)
	}

	p, err := f.auth.Authenticate(ctx, Credentials{SessionToken: res.Token, ClientIP: "10.0.0.1"})
	if err != nil || p.Kind != PrincipalUser || p.Name != "alice" || p.Role != RoleReader {
		t.Fatalf("Authenticate(session) = %+v, %v", p, err)
	}
	for _, bad := range []string{"mrs_nope", res.Token[:len(res.Token)-1] + "x", "mr_" + res.Token[4:]} {
		if _, err := f.auth.Authenticate(ctx, Credentials{SessionToken: bad}); !errors.Is(err, ErrSessionInvalid) {
			t.Errorf("Authenticate(%q): err = %v, want ErrSessionInvalid", bad, err)
		}
	}

	// A role change shows up at once.
	if err := f.users.SetRole(ctx, "alice", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if p, _ := f.auth.Authenticate(ctx, Credentials{SessionToken: res.Token}); p.Role != RoleAdmin {
		t.Fatalf("role after SetRole = %q", p.Role)
	}

	// Disabling signs the user out and blocks new logins.
	if err := f.users.SetDisabled(ctx, "alice", true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.sessions.Lookup(ctx, res.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("session of disabled user: err = %v", err)
	}
	if _, err := f.sessions.Login(ctx, "alice", "long enough pw", "test", "10.0.0.2"); !errors.Is(err, ErrInvalidLogin) {
		t.Fatalf("login while disabled: err = %v", err)
	}
	if err := f.users.SetDisabled(ctx, "alice", false); err != nil {
		t.Fatal(err)
	}

	// A new password ends existing sessions.
	res2, err := f.sessions.Login(ctx, "alice", "long enough pw", "test", "10.0.0.3")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.users.SetPassword(ctx, "alice", "a new long password", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.sessions.Lookup(ctx, res2.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("session after password change: err = %v", err)
	}

	// Logout ends only that session; unknown tokens are a no-op.
	a, _ := f.sessions.Login(ctx, "alice", "a new long password", "test", "10.0.0.4")
	b, _ := f.sessions.Login(ctx, "alice", "a new long password", "test", "10.0.0.5")
	if err := f.sessions.Logout(ctx, a.Token); err != nil {
		t.Fatal(err)
	}
	if err := f.sessions.Logout(ctx, "mrs_unknown"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.sessions.Lookup(ctx, a.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("logged-out session: err = %v", err)
	}
	if _, err := f.sessions.Lookup(ctx, b.Token); err != nil {
		t.Fatalf("other session after logout: %v", err)
	}
}

func TestSessionExpiryAndCache(t *testing.T) {
	ctx := context.Background()
	f := newAuthFixture(t)
	if _, err := f.users.Create(ctx, "alice", "long enough pw", RoleReader); err != nil {
		t.Fatal(err)
	}
	login := func() string {
		res, err := f.sessions.Login(ctx, "alice", "long enough pw", "", "10.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		return res.Token
	}
	idle, expired, live := login(), login(), login()
	db := f.sqlDB
	if _, err := db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = datetime('now', '-2 hours') WHERE token_hash = ?`, hashSessionToken(idle)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE sessions SET expires_at = datetime('now', '-1 minute') WHERE token_hash = ?`, hashSessionToken(expired)); err != nil {
		t.Fatal(err)
	}

	// A fresh service has nothing cached, so it reads the database.
	fresh := NewSessionService(f.sessions.log, f.repo, f.sessions.cfg)
	for name, tok := range map[string]string{"idle": idle, "expired": expired} {
		if _, err := fresh.Lookup(ctx, tok); !errors.Is(err, ErrSessionInvalid) {
			t.Errorf("%s session: err = %v, want ErrSessionInvalid", name, err)
		}
	}
	if _, err := fresh.Lookup(ctx, live); err != nil {
		t.Fatalf("live session: %v", err)
	}
	if n, err := fresh.PruneExpired(ctx); err != nil || n != 2 {
		t.Fatalf("PruneExpired = %d, %v; want 2", n, err)
	}

	// A revocation made by another process (e.g. the CLI) is seen once the
	// cache entry is older than sessionCacheTTL.
	if _, err := NewSessionService(f.sessions.log, f.repo, f.sessions.cfg).RevokeUser(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.Lookup(ctx, live); err != nil {
		t.Fatalf("cached session right after outside revocation: %v", err)
	}
	now := time.Now()
	fresh.now = func() time.Time { return now.Add(sessionCacheTTL + time.Second) }
	if _, err := fresh.Lookup(ctx, live); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("cached session after cache TTL: err = %v", err)
	}
}

func TestLoginRateLimit(t *testing.T) {
	ctx := context.Background()
	f := newAuthFixture(t)
	if _, err := f.users.Create(ctx, "alice", "long enough pw", RoleReader); err != nil {
		t.Fatal(err)
	}
	// Per username: burst of 5, whatever the client IP.
	var limited int
	for i := range 6 {
		ip := "10.0.1." + string(rune('1'+i))
		if _, err := f.sessions.Login(ctx, "alice", "wrong password", "", ip); errors.Is(err, ErrLoginRateLimited) {
			limited++
		}
	}
	if limited != 1 {
		t.Fatalf("rate limited %d of 6 attempts for one user, want 1", limited)
	}
	// Per IP: burst of 10, across usernames.
	limited = 0
	for i := range 11 {
		if _, err := f.sessions.Login(ctx, "user"+string(rune('a'+i)), "wrong password", "", "10.0.2.1"); errors.Is(err, ErrLoginRateLimited) {
			limited++
		}
	}
	if limited != 1 {
		t.Fatalf("rate limited %d of 11 attempts from one IP, want 1", limited)
	}
}

func TestAuthenticateKeysAndSessions(t *testing.T) {
	ctx := context.Background()
	f := newAuthFixture(t)
	key, err := f.keys.Create(ctx, "tides")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.users.Create(ctx, "alice", "long enough pw", RoleReader); err != nil {
		t.Fatal(err)
	}
	sess, err := f.sessions.Login(ctx, "alice", "long enough pw", "", "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		cred     Credentials
		wantKind string
		wantErr  error
	}{
		{"api key", Credentials{APIKey: key.Key}, PrincipalAPIKey, nil},
		{"session", Credentials{SessionToken: sess.Token}, PrincipalUser, nil},
		{"both: key wins", Credentials{APIKey: key.Key, SessionToken: "mrs_bogus"}, PrincipalAPIKey, nil},
		{"wrong key with valid session", Credentials{APIKey: "mr_wrong", SessionToken: sess.Token}, "", ErrAPIKeyInvalid},
		{"nothing", Credentials{}, "", ErrAPIKeyMissing},
		{"bad session", Credentials{SessionToken: "mrs_bogus"}, "", ErrSessionInvalid},
	}
	for _, tc := range cases {
		p, err := f.auth.Authenticate(ctx, tc.cred)
		if !errors.Is(err, tc.wantErr) || p.Kind != tc.wantKind {
			t.Errorf("%s: principal = %+v err = %v; want kind %q err %v", tc.name, p, err, tc.wantKind, tc.wantErr)
		}
		if tc.wantErr != nil && !IsUnauthorized(err) {
			t.Errorf("%s: IsUnauthorized(%v) = false", tc.name, err)
		}
	}
	if p, _ := f.auth.Authenticate(ctx, Credentials{APIKey: key.Key}); p.Name != "tides" || p.ID != key.ID {
		t.Fatalf("key principal = %+v", p)
	}

	// Keys optional: anonymous passes, a bad session still fails.
	off := false
	if _, err := f.keys.UpdateSettings(ctx, UpdateSettingsInput{RequireAPIKey: &off}); err != nil {
		t.Fatal(err)
	}
	if p, err := f.auth.Authenticate(ctx, Credentials{ClientIP: "1.2.3.4"}); err != nil || p.Kind != PrincipalAnonymous {
		t.Fatalf("anonymous with optional keys = %+v, %v", p, err)
	}
	if _, err := f.auth.Authenticate(ctx, Credentials{SessionToken: "mrs_bogus"}); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("bad session with optional keys: err = %v", err)
	}

	// The rate limit is per user, separate from the key's.
	limit := 40 // burst 10
	if _, err := f.keys.UpdateSettings(ctx, UpdateSettingsInput{RateLimitPerMinute: &limit}); err != nil {
		t.Fatal(err)
	}
	count := func(cred Credentials) (limited int) {
		for range 12 {
			if _, err := f.auth.Authenticate(ctx, cred); errors.Is(err, ErrRateLimited) {
				limited++
			}
		}
		return limited
	}
	if n := count(Credentials{SessionToken: sess.Token}); n != 2 {
		t.Fatalf("session: rate limited %d of 12, want 2", n)
	}
	if n := count(Credentials{APIKey: key.Key}); n != 2 {
		t.Fatalf("key after user hit its limit: rate limited %d of 12, want 2", n)
	}
}

func TestLastAdminGuard(t *testing.T) {
	ctx := context.Background()
	f := newAuthFixture(t)
	if _, err := f.users.Create(ctx, "root", "long enough pw", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err := f.users.Create(ctx, "alice", "long enough pw", RoleReader); err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{
		"demote":  f.users.SetRole(ctx, "root", RoleReader),
		"disable": f.users.SetDisabled(ctx, "root", true),
		"delete":  f.users.Delete(ctx, "root"),
	} {
		if !errors.Is(err, ErrLastAdmin) {
			t.Errorf("%s last admin: err = %v, want ErrLastAdmin", name, err)
		}
	}
	// Readers aren't guarded; a second admin lifts the guard.
	if err := f.users.SetDisabled(ctx, "alice", true); err != nil {
		t.Fatal(err)
	}
	if err := f.users.SetDisabled(ctx, "alice", false); err != nil {
		t.Fatal(err)
	}
	if err := f.users.SetRole(ctx, "alice", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := f.users.SetRole(ctx, "root", RoleReader); err != nil {
		t.Fatalf("demote with another admin: %v", err)
	}
	if err := f.users.Delete(ctx, "alice"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("delete the remaining admin: err = %v", err)
	}
	if n, _ := f.users.EnabledAdmins(ctx); n != 1 {
		t.Fatalf("EnabledAdmins = %d, want 1", n)
	}
}

func TestSessionListAndRevoke(t *testing.T) {
	ctx := context.Background()
	f := newAuthFixture(t)
	if _, err := f.users.Create(ctx, "alice", "long enough pw", RoleReader); err != nil {
		t.Fatal(err)
	}
	a, _ := f.sessions.Login(ctx, "alice", "long enough pw", "ua-a", "10.0.0.1")
	b, _ := f.sessions.Login(ctx, "alice", "long enough pw", "ua-b", "10.0.0.2")
	list, err := f.sessions.List(ctx)
	if err != nil || len(list) != 2 || list[0].Username != "alice" {
		t.Fatalf("List = %+v, %v", list, err)
	}
	// Warm the cache, then revoke by ID: the cached entry must go too.
	sa, err := f.sessions.Lookup(ctx, a.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.sessions.Revoke(ctx, sa.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.sessions.Lookup(ctx, a.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("revoked session: err = %v", err)
	}
	if err := f.sessions.Revoke(ctx, sa.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("revoke twice: err = %v", err)
	}
	// RevokeUserExcept keeps the given session.
	c, _ := f.sessions.Login(ctx, "alice", "long enough pw", "ua-c", "10.0.0.3")
	sb, _ := f.sessions.Lookup(ctx, b.Token)
	if n, err := f.sessions.RevokeUserExcept(ctx, "alice", sb.ID); err != nil || n != 1 {
		t.Fatalf("RevokeUserExcept = %d, %v; want 1", n, err)
	}
	if _, err := f.sessions.Lookup(ctx, b.Token); err != nil {
		t.Fatalf("kept session: %v", err)
	}
	if _, err := f.sessions.Lookup(ctx, c.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("other session: err = %v", err)
	}
}
