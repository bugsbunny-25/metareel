package client

import (
	"encoding/json"
	"testing"
)

func TestTMDBCertificationsAndLogo(t *testing.T) {
	var movie tmdbFullResp
	if err := json.Unmarshal([]byte(`{"original_language": "en",
	  "release_dates": {"results": [
	    {"iso_3166_1": "US", "release_dates": [
	      {"release_date": "2025-08-28T00:00:00.000Z", "type": 1, "certification": ""},
	      {"release_date": "2025-12-01T00:00:00.000Z", "type": 4, "certification": "R"},
	      {"release_date": "2025-10-24T00:00:00.000Z", "type": 3, "certification": "PG-13"}]},
	    {"iso_3166_1": "in", "release_dates": [{"release_date": "2025-10-24T00:00:00.000Z", "type": 4, "certification": " UA13+ "}]},
	    {"iso_3166_1": "GB", "release_dates": [{"release_date": "2025-10-24T00:00:00.000Z", "type": 3, "certification": ""}]},
	    {"iso_3166_1": "", "release_dates": [{"release_date": "2025-10-24T00:00:00.000Z", "type": 3, "certification": "X"}]}]},
	  "images": {"logos": [
	    {"file_path": "/de.png", "iso_639_1": "de", "vote_average": 9},
	    {"file_path": "/en-low.png", "iso_639_1": "en", "vote_average": 5.1, "vote_count": 3},
	    {"file_path": "/en-high.png", "iso_639_1": "en", "vote_average": 5.4, "vote_count": 1},
	    {"file_path": "/none.png", "iso_639_1": null, "vote_average": 10}]}}`), &movie); err != nil {
		t.Fatal(err)
	}
	d := movie.toDetails("movie", "1")
	// Theatrical wins over digital; blank certifications and bad countries are dropped.
	if len(d.Certifications) != 2 || d.Certifications["US"] != "PG-13" || d.Certifications["IN"] != "UA13+" {
		t.Errorf("movie certifications = %v", d.Certifications)
	}
	if d.LogoPath != "/en-high.png" {
		t.Errorf("logo = %q, want the best-voted English one", d.LogoPath)
	}

	var tv tmdbFullResp
	if err := json.Unmarshal([]byte(`{"original_language": "ko",
	  "content_ratings": {"results": [{"iso_3166_1": "US", "rating": "TV-MA"}, {"iso_3166_1": "KR", "rating": ""}]},
	  "images": {"logos": [{"file_path": "/ko.png", "iso_639_1": "ko"}, {"file_path": "/ja.png", "iso_639_1": "ja", "vote_average": 9}]}}`), &tv); err != nil {
		t.Fatal(err)
	}
	dt := tv.toDetails("tv", "2")
	if len(dt.Certifications) != 1 || dt.Certifications["US"] != "TV-MA" {
		t.Errorf("tv certifications = %v", dt.Certifications)
	}
	if dt.LogoPath != "/ko.png" {
		t.Errorf("tv logo = %q, want the original-language one", dt.LogoPath)
	}

	var bare tmdbFullResp
	_ = json.Unmarshal([]byte(`{"original_language": "en", "images": {"logos": [{"file_path": "/fr.png", "iso_639_1": "fr"}]}}`), &bare)
	if b := bare.toDetails("movie", "3"); b.Certifications != nil || b.LogoPath != "" {
		t.Errorf("bare = %v %q", b.Certifications, b.LogoPath)
	}
}
