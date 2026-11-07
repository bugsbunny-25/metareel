package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Passwords are hashed with argon2id (OWASP's minimum: 19 MiB, 2 passes,
// 1 thread) and stored as a PHC string:
// $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash> (unpadded base64).
const (
	argonMemoryKiB = 19 * 1024
	argonTime      = 2
	argonThreads   = 1
	argonKeyLen    = 32
	argonSaltLen   = 16

	minPasswordLen = 10
	maxPasswordLen = 256
)

// passwordSlots bounds concurrent hashes, so a burst of logins can't take
// more than a few × 19 MiB of memory.
var passwordSlots = make(chan struct{}, 4)

func acquirePasswordSlot(ctx context.Context) (release func(), err error) {
	select {
	case passwordSlots <- struct{}{}:
		return func() { <-passwordSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func validatePassword(pw string) error {
	n := utf8.RuneCountInString(pw)
	switch {
	case n < minPasswordLen:
		return &ValidationError{Message: fmt.Sprintf("password must be at least %d characters", minPasswordLen)}
	case len(pw) > maxPasswordLen:
		return &ValidationError{Message: fmt.Sprintf("password must be at most %d bytes", maxPasswordLen)}
	}
	return nil
}

func hashPassword(pw string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemoryKiB, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

var errBadPasswordHash = errors.New("unrecognised password hash")

// verifyPassword reports whether pw matches a hash made by hashPassword
// (with whatever parameters it was made with).
func verifyPassword(pw, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errBadPasswordHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errBadPasswordHash
	}
	var memory, passes uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &passes, &threads); err != nil || memory == 0 || passes == 0 || threads == 0 {
		return false, errBadPasswordHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errBadPasswordHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, errBadPasswordHash
	}
	got := argon2.IDKey([]byte(pw), salt, passes, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// dummyPasswordHash is verified against when a login names an unknown or
// disabled user, so the response takes as long as for a wrong password.
var dummyPasswordHash = sync.OnceValue(func() string {
	h, err := hashPassword("metareel-dummy-password")
	if err != nil {
		return ""
	}
	return h
})
