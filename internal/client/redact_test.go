package client

import (
	"errors"
	"testing"
)

func TestRedactErr(t *testing.T) {
	base := errors.New(`Get "https://api.themoviedb.org/3/search/movie?api_key=SECRET&query=x": timeout; https://api.mdblist.com/tmdb/movie/1?apikey=OTHER`)
	got := redactErr(base).Error()
	want := `Get "https://api.themoviedb.org/3/search/movie?api_key=REDACTED&query=x": timeout; https://api.mdblist.com/tmdb/movie/1?apikey=REDACTED`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if !errors.Is(redactErr(base), base) {
		t.Fatal("redacted error should unwrap to the original")
	}
	if redactErr(nil) != nil {
		t.Fatal("nil should stay nil")
	}
}
