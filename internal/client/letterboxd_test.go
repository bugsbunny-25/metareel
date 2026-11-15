package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLetterboxdResolve(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/film/dune-part-two/":
			hits++
			_, _ = w.Write([]byte(`<body class="film" data-tmdb-id="693134" data-tmdb-type="movie">`))
		case "/film/no-tmdb/":
			_, _ = w.Write([]byte(`<body class="film">`))
		case "/film/challenge/":
			w.WriteHeader(http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewLetterboxd(0, "metareel-test")
	c.base = srv.URL
	ctx := context.Background()

	for _, link := range []string{
		"https://letterboxd.com/film/dune-part-two/",
		"https://www.letterboxd.com/film/dune-part-two",
		"https://letterboxd.com/someuser/film/dune-part-two/",
	} {
		ref, err := c.Resolve(ctx, link)
		if err != nil || ref != (LetterboxdRef{Kind: "movie", TmdbID: "693134"}) {
			t.Errorf("Resolve(%q) = %+v, %v", link, ref, err)
		}
	}
	if hits != 1 {
		t.Errorf("film page fetched %d times, want once (cached)", hits)
	}
	for _, link := range []string{
		"https://letterboxd.com/someuser/list/best/",
		"https://letterboxd.com/tag/science-fiction/",
		"https://example.com/film/dune-part-two/",
		"https://boxd.it:22/abcd",
		"https://user@letterboxd.com/film/dune-part-two/",
		"ftp://letterboxd.com/film/dune-part-two/",
		"not a url",
		// boxd.it paths that aren't a single short code are refused before any request.
		"https://boxd.it/a/b",
		"https://boxd.it/../film/x",
		"https://boxd.it/",
		"https://boxd.it/abc%2Fdef",
	} {
		if _, err := c.Resolve(ctx, link); !errors.Is(err, ErrLetterboxdNotFilm) {
			t.Errorf("Resolve(%q): err = %v, want ErrLetterboxdNotFilm", link, err)
		}
	}
	if _, err := c.Resolve(ctx, "https://letterboxd.com/film/no-tmdb/"); !errors.Is(err, ErrLetterboxdNoTMDB) {
		t.Errorf("no tmdb id: err = %v", err)
	}
	if _, err := c.Resolve(ctx, "https://letterboxd.com/film/challenge/"); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("blocked page: err = %v, want an HTTP 403 error", err)
	}
	if !IsLetterboxdURL("https://boxd.it/aRvI") || IsLetterboxdURL("https://boxd.it.evil.com/x") {
		t.Error("IsLetterboxdURL host check")
	}
}
