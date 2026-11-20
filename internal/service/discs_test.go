package service

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

type fakeBarcodes struct {
	items map[string][]client.UPCItem
	calls int
	err   error
}

func (f *fakeBarcodes) Lookup(_ context.Context, code string) ([]client.UPCItem, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.items[code], nil
}

type fakeCollections struct{}

func (fakeCollections) Enabled() bool { return true }
func (fakeCollections) SearchCollections(context.Context, string) ([]client.TMDBCollection, error) {
	return []client.TMDBCollection{{ID: 263, Name: "The Dark Knight Collection"}}, nil
}
func (fakeCollections) Collection(context.Context, int) (client.TMDBCollection, error) {
	return client.TMDBCollection{ID: 263, Name: "The Dark Knight Collection", Parts: []client.TMDBSearchResult{
		{ID: 272, Title: "Batman Begins", Year: 2005, ReleaseDate: "2005-06-10"},
		{ID: 155, Title: "The Dark Knight", Year: 2008, ReleaseDate: "2008-07-16"},
	}}, nil
}

func newDiscEnv(t *testing.T) (*DiscService, *fakeBarcodes, int64, int64, *sql.DB) {
	t.Helper()
	_, src, alice, bob, db := newTrackingEnvDB(t)
	src.details["movie/272"] = client.TMDBTitleDetails{Title: "Batman Begins", ReleaseDate: "2005-06-10", Year: 2005, Found: true}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	trepo := repository.NewTrackingRepository(db)
	titles := NewTrackedTitleService(log, src, trepo, trepo)
	bc := &fakeBarcodes{items: map[string][]client.UPCItem{
		"883929640010": {{Title: "The Dark Knight Trilogy (4K Ultra HD + Blu-ray) Steelbook", Brand: "Warner Bros",
			Images: []string{"https://example.com/front.jpg"}}},
	}}
	repo := repository.NewDiscsRepository(db)
	s := NewDiscService(log, repo, repo, titles, fakeCollections{}, bc, t.TempDir(), "test")
	return s, bc, alice, bob, db
}

func iptr(v int) *int { return &v }

func trilogy() *DiscReleaseInput {
	return &DiscReleaseInput{
		Barcode: "883929640010", Title: "The Dark Knight Trilogy", MediaFormat: "uhd_bluray", Packaging: "box_set",
		HasSlipcover: true, Region: "free",
		Parts: []DiscReleaseInput{
			{Title: "Batman Begins", MediaFormat: "uhd_bluray", Packaging: "steelbook",
				Titles: []DiscTitleRef{{TmdbKind: "movie", TmdbID: "272"}},
				Discs: []DiscSpec{{Format: "uhd_bluray", Capacity: "bd66", TitleIndex: iptr(0), VideoCodec: "hevc",
					Resolution: "2160p", HDR: []string{"hdr10", "dolby_vision", "hdr10"}, AspectRatio: "2.39 : 1",
					Audio: []AudioTrack{{Language: "English", Format: "dolby_atmos", Channels: "7.1"}}}}},
			{Title: "The Dark Knight", MediaFormat: "uhd_bluray", Packaging: "steelbook",
				Titles: []DiscTitleRef{{TmdbKind: "movie", TmdbID: "155"}},
				Discs: []DiscSpec{{Format: "uhd_bluray", VariableAspect: true, AspectRatio: "1.78:1"},
					{Format: "bluray", Name: "Special features"}},
				SpecialFeatures: []SpecialFeature{{Title: "Gotham Uncovered", Minutes: 64, Disc: 2}}},
		},
	}
}

func TestDiscBoxSetLifecycle(t *testing.T) {
	s, _, alice, bob, _ := newDiscEnv(t)
	ctx := context.Background()

	got, err := s.Add(ctx, alice, AddDiscInput{
		DiscCopyInput: DiscCopyInput{PriceMinor: i64(5999), Currency: "usd", Retailer: "Best Buy", PurchasedOn: "2026-11-01", Condition: "sealed"},
		Release:       trilogy(),
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	r := got.Release
	if r == nil || len(r.Parts) != 2 || r.DiscCount != 3 || *got.Currency != "USD" {
		t.Fatalf("release = %+v", r)
	}
	if want := []string{"dolby_vision", "hdr10", "dolby_atmos"}; len(r.Highlights) != 3 || r.Highlights[0] != want[0] || r.Highlights[2] != want[2] {
		t.Errorf("highlights = %v, want %v", r.Highlights, want)
	}
	if d := r.Parts[0].Discs[0]; d.AspectRatio != "2.39:1" || len(d.HDR) != 2 || d.Number != 1 {
		t.Errorf("disc 1 = %+v", d)
	}
	if r.Parts[1].Titles[0].Title != "The Dark Knight" {
		t.Errorf("part 2 titles = %+v", r.Parts[1].Titles)
	}

	// The shelf finds the box set by a film in one of its parts.
	list, err := s.List(ctx, alice, DiscListInput{TmdbKind: "movie", TmdbID: "155"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	items := list["items"].([]DiscCopyView)
	totals := list["totals"].(DiscShelfTotals)
	if len(items) != 1 || totals.Discs != 3 || totals.Titles != 2 || totals.BoxSets != 1 || totals.Spend[0].Minor != 5999 {
		t.Fatalf("list = %d items, totals %+v", len(items), totals)
	}
	if other, _ := s.List(ctx, bob, DiscListInput{}); len(other["items"].([]DiscCopyView)) != 0 {
		t.Error("bob sees alice's copies")
	}

	// A second scan of the barcode finds the release; bob can add a copy but
	// can't edit it until he owns one.
	look, err := s.Lookup(ctx, bob, "883929640010")
	if err != nil || look.Status != "known" || look.Release.ID != r.ID || len(look.OwnedCopyIDs) != 0 {
		t.Fatalf("lookup = %+v, %v", look, err)
	}
	var forbidden *ForbiddenError
	if _, err := s.UpdateRelease(ctx, bob, r.ID, *trilogy()); !errors.As(err, &forbidden) {
		t.Fatalf("bob edits without a copy: %v", err)
	}
	if _, err := s.Add(ctx, bob, AddDiscInput{ReleaseID: r.Parts[0].ID}); err == nil {
		t.Error("a copy of a box set's part was accepted")
	}
	if _, err := s.Add(ctx, bob, AddDiscInput{ReleaseID: r.ID, DiscCopyInput: DiscCopyInput{Condition: "used"}}); err != nil {
		t.Fatalf("bob adds a copy: %v", err)
	}

	// Editing keeps part 2 (by id), drops part 1 and adds a new part.
	in := *trilogy()
	in.Parts = []DiscReleaseInput{in.Parts[1], {Title: "The Dark Knight Rises", MediaFormat: "uhd_bluray"}}
	in.Parts[0].ID = r.Parts[1].ID
	out, err := s.UpdateRelease(ctx, bob, r.ID, in)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	upd := out["release"].(*DiscReleaseView)
	if len(upd.Parts) != 2 || upd.Parts[0].ID != r.Parts[1].ID || upd.Parts[1].Title != "The Dark Knight Rises" {
		t.Fatalf("parts after update = %+v", upd.Parts)
	}

	// Another release can't take the barcode.
	dup := &DiscReleaseInput{Barcode: "0883929640010", Title: "Dup", MediaFormat: "bluray"}
	var conflict *ConflictError
	if _, err := s.Add(ctx, alice, AddDiscInput{Release: dup}); !errors.As(err, &conflict) {
		t.Errorf("duplicate barcode: %v", err)
	}

	// Deleting a copy keeps the release.
	if err := s.DeleteCopy(ctx, alice, got.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteCopy(ctx, alice, got.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("second delete: %v", err)
	}
	if _, err := s.GetRelease(ctx, alice, r.ID); err != nil {
		t.Errorf("release gone with the copy: %v", err)
	}
}

func TestDiscValidation(t *testing.T) {
	s, _, alice, _, _ := newDiscEnv(t)
	ctx := context.Background()
	base := func() *DiscReleaseInput { return &DiscReleaseInput{Title: "X", MediaFormat: "bluray"} }
	cases := map[string]AddDiscInput{
		"no release":       {},
		"bad format":       {Release: &DiscReleaseInput{Title: "X", MediaFormat: "laserdisc"}},
		"price no ccy":     {Release: base(), DiscCopyInput: DiscCopyInput{PriceMinor: i64(100)}},
		"bad barcode":      {Release: &DiscReleaseInput{Title: "X", MediaFormat: "bluray", Barcode: "883929640017"}},
		"unknown tmdb":     {Release: &DiscReleaseInput{Title: "X", MediaFormat: "bluray", Titles: []DiscTitleRef{{TmdbKind: "movie", TmdbID: "1"}}}},
		"title index":      {Release: &DiscReleaseInput{Title: "X", MediaFormat: "bluray", Discs: []DiscSpec{{Format: "bluray", TitleIndex: iptr(0)}}}},
		"bad audio":        {Release: &DiscReleaseInput{Title: "X", MediaFormat: "bluray", Discs: []DiscSpec{{Format: "bluray", Audio: []AudioTrack{{Format: "dolby_surround_9000"}}}}}},
		"bad aspect":       {Release: &DiscReleaseInput{Title: "X", MediaFormat: "bluray", Discs: []DiscSpec{{Format: "bluray", AspectRatio: "wide"}}}},
		"nested box parts": {Release: &DiscReleaseInput{Title: "X", MediaFormat: "bluray", Parts: []DiscReleaseInput{{Title: "Y", MediaFormat: "dvd", Parts: []DiscReleaseInput{{Title: "Z", MediaFormat: "dvd"}}}}}},
		"season on movie":  {Release: &DiscReleaseInput{Title: "X", MediaFormat: "bluray", Titles: []DiscTitleRef{{TmdbKind: "movie", TmdbID: "155", SeasonNumber: i64(1)}}}},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			var v *ValidationError
			if _, err := s.Add(ctx, alice, in); !errors.As(err, &v) {
				t.Errorf("err = %v, want a validation error", err)
			}
		})
	}
}

func TestDiscLookupCachesAndSuggests(t *testing.T) {
	s, bc, alice, _, _ := newDiscEnv(t)
	ctx := context.Background()
	look, err := s.Lookup(ctx, alice, "883929640010")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	sg := look.Suggestion
	if look.Status != "found" || sg == nil || sg.MediaFormat != "uhd_bluray" || !sg.BoxSet || sg.Packaging != "steelbook" || sg.Studio != "Warner Bros" {
		t.Fatalf("lookup = %+v, suggestion %+v", look, sg)
	}
	if len(look.Collections) != 1 || len(look.Collections[0].Parts) != 2 || len(look.Candidates) == 0 {
		t.Errorf("collections = %+v, candidates = %+v", look.Collections, look.Candidates)
	}
	if _, err := s.Lookup(ctx, alice, "0883929640010"); err != nil || bc.calls != 1 {
		t.Errorf("second lookup called the service again (%d calls, %v)", bc.calls, err)
	}
	// A miss is cached too; a rate limit is not.
	if l, _ := s.Lookup(ctx, alice, "5051892230841"); l.Status != "not_found" {
		t.Errorf("miss status = %s", l.Status)
	}
	bc.err = client.ErrBarcodeRateLimited
	if l, _ := s.Lookup(ctx, alice, "4006381333931"); l.Status != "rate_limited" {
		t.Errorf("rate limited status = %s", l.Status)
	}
	if l, _ := s.Lookup(ctx, alice, "5051892230841"); l.Status != "not_found" || bc.calls != 3 {
		t.Errorf("cached miss: status %s, %d calls", l.Status, bc.calls)
	}
	// Expired cache entries are asked again.
	s.now = func() time.Time { return time.Now().Add(lookupNotFoundTTL + time.Hour) }
	bc.err = nil
	_, _ = s.Lookup(ctx, alice, "5051892230841")
	if bc.calls != 4 {
		t.Errorf("expired miss not retried: %d calls", bc.calls)
	}
}

func TestDiscImages(t *testing.T) {
	s, _, alice, bob, _ := newDiscEnv(t)
	ctx := context.Background()
	got, err := s.Add(ctx, alice, AddDiscInput{Release: trilogy()})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	part := got.Release.Parts[0].ID

	// A 3000x1000 photo stored sideways (EXIF orientation 6) comes out
	// upright and scaled to 2000 on its long side.
	photo := jpegWithOrientation(t, 3000, 1000, 6)
	im, err := s.AddImage(ctx, alice, part, "front", photo)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if im.Width != 666 || im.Height != 2000 {
		t.Errorf("stored %dx%d, want 666x2000 (rotated, scaled)", im.Width, im.Height)
	}
	f, ct, err := s.OpenImage(filepath.Base(im.URL))
	if err != nil || ct != "image/jpeg" {
		t.Fatalf("open: %v %s", err, ct)
	}
	stored, _ := io.ReadAll(f)
	f.Close()
	if bytes.Contains(stored, []byte("Exif")) {
		t.Error("stored file kept its EXIF")
	}
	if _, _, err := s.OpenImage("../../etc/passwd"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("path traversal: %v", err)
	}

	// Transparent disc art stays PNG.
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 40, 40)))
	if pim, err := s.AddImage(ctx, alice, part, "disc", buf.Bytes()); err != nil || filepath.Ext(pim.URL) != ".png" {
		t.Errorf("png upload: %+v %v", pim, err)
	}

	var forbidden *ForbiddenError
	if _, err := s.AddImage(ctx, bob, part, "front", photo); !errors.As(err, &forbidden) {
		t.Errorf("bob uploads without a copy: %v", err)
	}
	var v *ValidationError
	if _, err := s.AddImage(ctx, alice, part, "front", []byte("<svg/>")); !errors.As(err, &v) {
		t.Errorf("non-image accepted: %v", err)
	}
	if _, err := s.ImportImage(ctx, alice, got.Release.ID, "front", "http://169.254.169.254/latest/meta-data"); !errors.As(err, &v) {
		t.Errorf("arbitrary URL accepted for import: %v", err)
	}

	// Deleting the picture removes its file.
	if err := s.DeleteImage(ctx, alice, part, im.ID); err != nil {
		t.Fatalf("delete image: %v", err)
	}
	if _, _, err := s.OpenImage(filepath.Base(im.URL)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file kept after delete: %v", err)
	}
}

// jpegWithOrientation encodes a w×h JPEG with an EXIF orientation tag.
func jpegWithOrientation(t *testing.T, w, h, orientation int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, 0, color.RGBA{R: 255, A: 255})
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	// APP1 Exif segment: big-endian TIFF, one IFD entry (0x0112, SHORT, 1).
	tiff := []byte{'M', 'M', 0, 42, 0, 0, 0, 8, 0, 1, 0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, byte(orientation), 0, 0, 0, 0, 0, 0}
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	seg = append(seg, payload...)
	return append(append([]byte{0xFF, 0xD8}, seg...), raw[2:]...)
}
