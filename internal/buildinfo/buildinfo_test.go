package buildinfo

import "testing"

func TestSetAndGet(t *testing.T) {
	t.Cleanup(func() { version, revision = "dev", "" })
	if got := Get(); got.Version != "dev" || got.Go == "" {
		t.Fatalf("default = %+v, want version dev and a Go release", got)
	}
	Set("v1.4.0", "0123456789abcdef0123")
	if got := Get(); got.Version != "v1.4.0" || got.Revision != "0123456789ab" {
		t.Fatalf("stamped = %+v, want v1.4.0 at 0123456789ab", got)
	}
	Set("", "")
	if got := Get(); got.Version != "v1.4.0" {
		t.Fatalf("empty Set changed the version: %+v", got)
	}
}
