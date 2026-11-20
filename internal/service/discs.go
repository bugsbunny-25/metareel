package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Physical media vocabularies. Validated here rather than by CHECKs, so a
// new value needs no table rebuild. Clients turn the keys into labels.
var (
	discMediaFormats = set("uhd_bluray", "bluray", "bluray_3d", "dvd", "hd_dvd", "other")
	// A disc inside a release can also be a soundtrack CD.
	discDiscFormats = set("uhd_bluray", "bluray", "bluray_3d", "dvd", "hd_dvd", "cd", "other")
	discPackagings  = set("", "keepcase", "steelbook", "digipak", "digibook", "mediabook", "box_set", "other")
	discCapacities  = set("", "bd100", "bd66", "bd50", "bd25", "dvd9", "dvd5", "hd_dvd30", "hd_dvd15", "cd")
	discCodecs      = set("", "hevc", "avc", "vc1", "mpeg2", "other")
	discResolutions = set("", "2160p", "1080p", "1080i", "720p", "576p", "576i", "480p", "480i")
	discHDR         = set("dolby_vision", "hdr10", "hdr10_plus", "hlg")
	discAudio       = set("dolby_atmos", "dts_x", "auro_3d", "dolby_truehd", "dts_hd_ma", "dts_hd_hr", "lpcm",
		"dolby_digital_plus", "dts", "dolby_digital", "aac", "mpeg", "other")
	discConditions = set("", "sealed", "new", "used")
	discImageKinds = set("front", "back", "slip_front", "slip_back", "spine", "disc", "booklet", "inside", "other")

	// Audio formats worth a badge on the shelf, best first.
	discImmersiveAudio = []string{"dolby_atmos", "dts_x", "auro_3d"}

	barcodeRe     = regexp.MustCompile(`^[0-9]{8}$|^[0-9]{12,14}$`)
	labelSlugRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	channelsRe    = regexp.MustCompile(`^(mono|stereo|[0-9]{1,2}\.[0-9](\.[0-9]{1,2})?)$`)
	aspectRatioRe = regexp.MustCompile(`^[0-9]{1,2}(\.[0-9]{1,3})?:1$`)
)

func set(vals ...string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return m
}

const (
	maxReleaseTitles   = 60
	maxReleaseParts    = 40
	maxReleaseDiscs    = 80
	maxReleaseFeatures = 400
	maxDiscTracks      = 40
	maxDiscSubtitles   = 80
)

// DiscSpec is one disc of a release: what it is and how it was mastered.
type DiscSpec struct {
	Number         int          `json:"number"` // 1-based, from its position
	Format         string       `json:"format"`
	Capacity       string       `json:"capacity,omitempty"`
	Name           string       `json:"name,omitempty"`        // "Feature film", "Bonus disc"
	TitleIndex     *int         `json:"title_index,omitempty"` // which of the release's titles it holds
	SeasonNumber   *int         `json:"season_number,omitempty"`
	Episodes       string       `json:"episodes,omitempty"` // "1-5"
	VideoCodec     string       `json:"video_codec,omitempty"`
	Resolution     string       `json:"resolution,omitempty"`
	HDR            []string     `json:"hdr,omitempty"`
	AspectRatio    string       `json:"aspect_ratio,omitempty"` // "2.39:1"; IMAX releases often vary
	VariableAspect bool         `json:"variable_aspect,omitempty"`
	Region         string       `json:"region,omitempty"`
	RuntimeMinutes int          `json:"runtime_minutes,omitempty"`
	Audio          []AudioTrack `json:"audio,omitempty"`
	Subtitles      []string     `json:"subtitles,omitempty"`
	Notes          string       `json:"notes,omitempty"`
}

// AudioTrack is one soundtrack on a disc.
type AudioTrack struct {
	Language    string `json:"language"`
	Format      string `json:"format"`
	Channels    string `json:"channels,omitempty"`    // 7.1, 5.1, 2.0, mono
	Description string `json:"description,omitempty"` // "Commentary with ..."
}

// SpecialFeature is one extra on a release.
type SpecialFeature struct {
	Title   string `json:"title"`
	Minutes int    `json:"minutes,omitempty"`
	Disc    int    `json:"disc,omitempty"` // disc number, 0 = unknown
	Notes   string `json:"notes,omitempty"`
}

// DiscTitleRef is a TMDB title on a release.
type DiscTitleRef struct {
	TmdbKind     string `json:"tmdb_kind"`
	TmdbID       string `json:"tmdb_id"`
	SeasonNumber *int64 `json:"season_number,omitempty"`
}

// DiscReleaseInput is a release as a client sends it. Parts make it a box
// set; a part's ID is set when it already exists.
type DiscReleaseInput struct {
	ID              int64              `json:"id,omitempty"`
	Barcode         string             `json:"barcode"`
	Title           string             `json:"title"`
	Edition         string             `json:"edition"`
	MediaFormat     string             `json:"media_format"`
	Packaging       string             `json:"packaging"`
	HasSlipcover    bool               `json:"has_slipcover"`
	Region          string             `json:"region"`
	Country         string             `json:"country"`
	Studio          string             `json:"studio"`
	Label           string             `json:"label"`
	ReleaseDate     string             `json:"release_date"`
	Discs           []DiscSpec         `json:"discs"`
	SpecialFeatures []SpecialFeature   `json:"special_features"`
	Notes           string             `json:"notes"`
	Titles          []DiscTitleRef     `json:"titles"`
	Parts           []DiscReleaseInput `json:"parts"`
}

// DiscCopyInput is the user's own part of a copy.
type DiscCopyInput struct {
	PurchasedOn string `json:"purchased_on"`
	PriceMinor  *int64 `json:"price_minor"`
	Currency    string `json:"currency"`
	Retailer    string `json:"retailer"`
	Condition   string `json:"condition"`
	Notes       string `json:"notes"`
}

// DiscImageImport is a picture to fetch for a release.
type DiscImageImport struct {
	URL       string `json:"url"`
	Kind      string `json:"kind"`
	ReleaseID int64  `json:"release_id,omitempty"` // a part of the box set; 0 = the release itself
}

// AddDiscInput adds a copy: of a release metareel knows (ReleaseID) or of a
// new one (Release), optionally fetching pictures the lookup suggested.
type AddDiscInput struct {
	DiscCopyInput
	ReleaseID    int64             `json:"release_id"`
	Release      *DiscReleaseInput `json:"release"`
	ImportImages []DiscImageImport `json:"import_images"`
}

// DiscTitleView is a title on a release, with what is known about it.
type DiscTitleView struct {
	TmdbKind     string `json:"tmdb_kind"`
	TmdbID       string `json:"tmdb_id"`
	SeasonNumber *int64 `json:"season_number,omitempty"`
	Title        string `json:"title"`
	Year         int    `json:"year,omitempty"`
	PosterPath   string `json:"poster_path,omitempty"`
}

// DiscImageView is a picture of a release.
type DiscImageView struct {
	ID       int64  `json:"id"`
	Kind     string `json:"kind"`
	Position int64  `json:"position"`
	URL      string `json:"url"`
	ThumbURL string `json:"thumb_url"`
	Width    int64  `json:"width"`
	Height   int64  `json:"height"`
	Source   string `json:"source"`
}

// DiscReleaseView is a release as the API shows it.
type DiscReleaseView struct {
	ID              int64             `json:"id"`
	ParentID        *int64            `json:"parent_id,omitempty"`
	Barcode         *string           `json:"barcode"`
	Title           string            `json:"title"`
	Edition         string            `json:"edition"`
	MediaFormat     string            `json:"media_format"`
	Packaging       string            `json:"packaging"`
	HasSlipcover    bool              `json:"has_slipcover"`
	Region          string            `json:"region"`
	Country         *string           `json:"country"`
	Studio          string            `json:"studio"`
	Label           *string           `json:"label"`
	ReleaseDate     *string           `json:"release_date"`
	Discs           []DiscSpec        `json:"discs"`
	SpecialFeatures []SpecialFeature  `json:"special_features"`
	Notes           *string           `json:"notes"`
	Source          string            `json:"source"`
	Titles          []DiscTitleView   `json:"titles"`
	Images          []DiscImageView   `json:"images"`
	Parts           []DiscReleaseView `json:"parts"`
	// DiscCount counts the release's discs and its parts'.
	DiscCount int `json:"disc_count"`
	// Highlights are the badges worth showing on a shelf: HDR formats and
	// immersive audio, across every disc (and part).
	Highlights []string `json:"highlights"`
	UpdatedAt  string   `json:"updated_at"`
}

// DiscCopyView is a user's copy with its release.
type DiscCopyView struct {
	ID          int64            `json:"id"`
	ReleaseID   int64            `json:"release_id"`
	PurchasedOn *string          `json:"purchased_on"`
	PriceMinor  *int64           `json:"price_minor"`
	Currency    *string          `json:"currency"`
	Retailer    string           `json:"retailer"`
	Condition   string           `json:"condition"`
	Notes       *string          `json:"notes"`
	CreatedAt   string           `json:"created_at"`
	UpdatedAt   string           `json:"updated_at"`
	Release     *DiscReleaseView `json:"release"`
}

// DiscShelfTotals sums up a shelf (or the filtered part of it).
type DiscShelfTotals struct {
	Copies        int              `json:"copies"`
	Discs         int              `json:"discs"`
	Titles        int              `json:"titles"`
	BoxSets       int              `json:"box_sets"`
	WithSlipcover int              `json:"with_slipcover"`
	ByFormat      map[string]int   `json:"by_format"`
	Spend         []CurrencyAmount `json:"spend"` // never converted between currencies
	Retailers     []RetailerCount  `json:"retailers"`
}

// CurrencyAmount is money in one currency.
type CurrencyAmount struct {
	Currency string `json:"currency"`
	Minor    int64  `json:"minor"`
	Copies   int    `json:"copies"`
	AvgMinor int64  `json:"avg_minor"`
}

// RetailerCount is how many copies came from a retailer.
type RetailerCount struct {
	Retailer string `json:"retailer"`
	Copies   int    `json:"copies"`
}

// ForbiddenError: the user may not change this (403).
type ForbiddenError struct{ Message string }

func (e *ForbiddenError) Error() string { return e.Message }

// BarcodeSource looks barcodes up (UPCitemdb).
type BarcodeSource interface {
	Lookup(ctx context.Context, barcode string) ([]client.UPCItem, error)
}

// collectionSource finds TMDB collections for box sets.
type collectionSource interface {
	Enabled() bool
	SearchCollections(ctx context.Context, query string) ([]client.TMDBCollection, error)
	Collection(ctx context.Context, id int) (client.TMDBCollection, error)
}

// DiscService keeps users' physical media: shared releases (with box-set
// parts, titles, specs and pictures) and each user's copies.
//
// Releases are shared reference data, like venues: whoever scans a barcode
// first fills the release in, and anyone who owns a copy may correct it.
// Copies are private to their user.
type DiscService struct {
	log         *slog.Logger
	repo        *repository.DiscsRepository
	read        *repository.DiscsRepository
	titles      *TrackedTitleService
	collections collectionSource
	barcodes    BarcodeSource // nil: lookups are off
	store       discImageStore
	fetcher     *imageFetcher
	now         func() time.Time
}

func NewDiscService(log *slog.Logger, write, read *repository.DiscsRepository, titles *TrackedTitleService,
	collections collectionSource, barcodes BarcodeSource, mediaDir, userAgent string) *DiscService {
	if read == nil {
		read = write
	}
	return &DiscService{
		log: log, repo: write, read: read, titles: titles, collections: collections, barcodes: barcodes,
		store: discImageStore{dir: mediaDir}, fetcher: newImageFetcher(userAgent), now: time.Now,
	}
}

// --- validation ---------------------------------------------------------

func trimTo(s string, n int, field string) (string, error) {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > n {
		return "", &ValidationError{Message: fmt.Sprintf("%s must be at most %d characters", field, n)}
	}
	return s, nil
}

// NormalizeBarcode turns a scanned or typed code into GTIN-13 digits: spaces
// and dashes go, a UPC-A (12 digits) gains its leading 0, an EAN-8 and a
// GTIN-14 stay as they are. The check digit must be right.
func NormalizeBarcode(raw string) (string, error) {
	b := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' {
			return -1
		}
		return r
	}, strings.TrimSpace(raw))
	if !barcodeRe.MatchString(b) {
		return "", &ValidationError{Message: "barcode must be an EAN-13, UPC-A, EAN-8 or GTIN-14 number"}
	}
	if len(b) == 12 {
		b = "0" + b
	}
	if len(b) == 14 && b[0] == '0' {
		b = b[1:]
	}
	if !gtinCheckDigitOK(b) {
		return "", &ValidationError{Message: "barcode check digit is wrong; check the number"}
	}
	return b, nil
}

func gtinCheckDigitOK(code string) bool {
	sum := 0
	n := len(code)
	for i := 0; i < n-1; i++ {
		d := int(code[n-2-i] - '0')
		if i%2 == 0 {
			d *= 3
		}
		sum += d
	}
	return (10-sum%10)%10 == int(code[n-1]-'0')
}

// upcFromGTIN is the 12-digit UPC-A of a GTIN-13 starting with 0, which is
// how US lookup services index discs.
func upcFromGTIN(code string) string {
	if len(code) == 13 && code[0] == '0' {
		return code[1:]
	}
	return code
}

func (s *DiscService) releaseWrite(ctx context.Context, in DiscReleaseInput, depth int) (repository.ReleaseWrite, error) {
	var w repository.ReleaseWrite
	var err error
	w.ID = in.ID
	if in.Barcode != "" {
		b, err := NormalizeBarcode(in.Barcode)
		if err != nil {
			return w, err
		}
		w.Barcode = &b
	}
	if w.Title, err = trimTo(in.Title, 300, "title"); err != nil {
		return w, err
	}
	if w.Title == "" {
		return w, &ValidationError{Message: "title is required"}
	}
	if w.Edition, err = trimTo(in.Edition, 120, "edition"); err != nil {
		return w, err
	}
	if !discMediaFormats[in.MediaFormat] {
		return w, &ValidationError{Message: "media_format must be one of uhd_bluray, bluray, bluray_3d, dvd, hd_dvd, other"}
	}
	w.MediaFormat = in.MediaFormat
	if !discPackagings[in.Packaging] {
		return w, &ValidationError{Message: "packaging must be one of keepcase, steelbook, digipak, digibook, mediabook, box_set, other"}
	}
	w.Packaging = in.Packaging
	w.HasSlipcover = in.HasSlipcover
	if w.Region, err = trimTo(strings.ToUpper(in.Region), 12, "region"); err != nil {
		return w, err
	}
	if c := strings.ToUpper(strings.TrimSpace(in.Country)); c != "" {
		if !countryRe.MatchString(c) {
			return w, &ValidationError{Message: "country must be an ISO 3166 two-letter code"}
		}
		w.Country = &c
	}
	if w.Studio, err = trimTo(in.Studio, 120, "studio"); err != nil {
		return w, err
	}
	if l := strings.TrimSpace(in.Label); l != "" {
		if !labelSlugRe.MatchString(l) {
			return w, &ValidationError{Message: "label must be a format slug such as criterion"}
		}
		w.Label = &l
	}
	if d := strings.TrimSpace(in.ReleaseDate); d != "" {
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return w, &ValidationError{Message: "release_date must be YYYY-MM-DD"}
		}
		w.ReleaseDate = &d
	}
	notes, err := trimTo(in.Notes, 4000, "notes")
	if err != nil {
		return w, err
	}
	w.Notes = optStr(notes)

	if len(in.Titles) > maxReleaseTitles {
		return w, &ValidationError{Message: fmt.Sprintf("a release can hold at most %d titles", maxReleaseTitles)}
	}
	seen := map[string]bool{}
	for _, t := range in.Titles {
		if err := ValidateRef(t.TmdbKind, t.TmdbID); err != nil {
			return w, err
		}
		if t.SeasonNumber != nil && (t.TmdbKind != "tv" || *t.SeasonNumber < 0 || *t.SeasonNumber > 1000) {
			return w, &ValidationError{Message: "season_number is only for TV titles, 0 (specials) or more"}
		}
		key := fmt.Sprintf("%s/%s/%d", t.TmdbKind, t.TmdbID, seasonKey(t.SeasonNumber))
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, err := s.titles.Resolve(ctx, t.TmdbKind, t.TmdbID); err != nil {
			if errors.Is(err, ErrTrackedTitleUnknown) {
				return w, &ValidationError{Message: fmt.Sprintf("TMDB has no %s %s", t.TmdbKind, t.TmdbID)}
			}
			return w, err
		}
		w.Titles = append(w.Titles, repository.ReleaseTitleRef{Kind: t.TmdbKind, ID: t.TmdbID, SeasonNumber: t.SeasonNumber})
	}

	discs, err := validDiscs(in.Discs, len(w.Titles))
	if err != nil {
		return w, err
	}
	features, err := validFeatures(in.SpecialFeatures)
	if err != nil {
		return w, err
	}
	w.Discs, w.SpecialFeatures = mustJSON(discs), mustJSON(features)

	if len(in.Parts) > 0 {
		if depth > 0 {
			return w, &ValidationError{Message: "a part of a box set can't have parts of its own"}
		}
		if len(in.Parts) > maxReleaseParts {
			return w, &ValidationError{Message: fmt.Sprintf("a box set can have at most %d parts", maxReleaseParts)}
		}
		for i, p := range in.Parts {
			pw, err := s.releaseWrite(ctx, p, depth+1)
			if err != nil {
				var v *ValidationError
				if errors.As(err, &v) {
					return w, &ValidationError{Message: fmt.Sprintf("part %d: %s", i+1, v.Message)}
				}
				return w, err
			}
			w.Parts = append(w.Parts, pw)
		}
	}
	return w, nil
}

func seasonKey(p *int64) int64 {
	if p == nil {
		return -1
	}
	return *p
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func validDiscs(in []DiscSpec, titles int) ([]DiscSpec, error) {
	if len(in) > maxReleaseDiscs {
		return nil, &ValidationError{Message: fmt.Sprintf("a release can have at most %d discs", maxReleaseDiscs)}
	}
	out := make([]DiscSpec, 0, len(in))
	for i, d := range in {
		bad := func(msg string) error {
			return &ValidationError{Message: fmt.Sprintf("disc %d: %s", i+1, msg)}
		}
		d.Number = i + 1
		switch {
		case !discDiscFormats[d.Format]:
			return nil, bad("format must be one of uhd_bluray, bluray, bluray_3d, dvd, hd_dvd, cd, other")
		case !discCapacities[d.Capacity]:
			return nil, bad("capacity must be one of bd100, bd66, bd50, bd25, dvd9, dvd5, hd_dvd30, hd_dvd15, cd")
		case !discCodecs[d.VideoCodec]:
			return nil, bad("video_codec must be one of hevc, avc, vc1, mpeg2, other")
		case !discResolutions[d.Resolution]:
			return nil, bad("resolution must be one of 2160p, 1080p, 1080i, 720p, 576p, 576i, 480p, 480i")
		case d.TitleIndex != nil && (*d.TitleIndex < 0 || *d.TitleIndex >= titles):
			return nil, bad("title_index must point at one of the release's titles")
		case d.SeasonNumber != nil && (*d.SeasonNumber < 0 || *d.SeasonNumber > 1000):
			return nil, bad("season_number must be 0 or more")
		case d.RuntimeMinutes < 0 || d.RuntimeMinutes > 2000:
			return nil, bad("runtime_minutes must be between 0 and 2000")
		case len(d.Audio) > maxDiscTracks:
			return nil, bad(fmt.Sprintf("at most %d audio tracks", maxDiscTracks))
		case len(d.Subtitles) > maxDiscSubtitles:
			return nil, bad(fmt.Sprintf("at most %d subtitle languages", maxDiscSubtitles))
		}
		d.AspectRatio = strings.ReplaceAll(strings.TrimSpace(d.AspectRatio), " ", "")
		if d.AspectRatio != "" && !aspectRatioRe.MatchString(d.AspectRatio) {
			return nil, bad("aspect_ratio must look like 2.39:1")
		}
		hdr := []string{}
		for _, h := range d.HDR {
			if !discHDR[h] {
				return nil, bad("hdr values must be dolby_vision, hdr10, hdr10_plus or hlg")
			}
			if !contains(hdr, h) {
				hdr = append(hdr, h)
			}
		}
		d.HDR = hdr
		var err error
		for _, f := range []struct {
			p    *string
			n    int
			name string
		}{{&d.Name, 120, "name"}, {&d.Episodes, 60, "episodes"}, {&d.Notes, 2000, "notes"}, {&d.Region, 12, "region"}} {
			if *f.p, err = trimTo(*f.p, f.n, f.name); err != nil {
				return nil, bad(err.Error())
			}
		}
		d.Region = strings.ToUpper(d.Region)
		for j, a := range d.Audio {
			if !discAudio[a.Format] {
				return nil, bad(fmt.Sprintf("audio %d: format must be one of dolby_atmos, dts_x, auro_3d, dolby_truehd, dts_hd_ma, dts_hd_hr, lpcm, dolby_digital_plus, dts, dolby_digital, aac, mpeg, other", j+1))
			}
			a.Channels = strings.ToLower(strings.TrimSpace(a.Channels))
			if a.Channels != "" && !channelsRe.MatchString(a.Channels) {
				return nil, bad(fmt.Sprintf("audio %d: channels must look like 7.1, 5.1, 2.0 or mono", j+1))
			}
			if a.Language, err = trimTo(a.Language, 40, "language"); err != nil {
				return nil, bad(err.Error())
			}
			if a.Description, err = trimTo(a.Description, 200, "description"); err != nil {
				return nil, bad(err.Error())
			}
			d.Audio[j] = a
		}
		subs := []string{}
		for _, sub := range d.Subtitles {
			sub, err = trimTo(sub, 40, "subtitle")
			if err != nil {
				return nil, bad(err.Error())
			}
			if sub != "" && !contains(subs, sub) {
				subs = append(subs, sub)
			}
		}
		d.Subtitles = subs
		out = append(out, d)
	}
	return out, nil
}

func validFeatures(in []SpecialFeature) ([]SpecialFeature, error) {
	if len(in) > maxReleaseFeatures {
		return nil, &ValidationError{Message: fmt.Sprintf("a release can list at most %d special features", maxReleaseFeatures)}
	}
	out := make([]SpecialFeature, 0, len(in))
	for i, f := range in {
		var err error
		if f.Title, err = trimTo(f.Title, 300, "special feature title"); err != nil {
			return nil, err
		}
		if f.Title == "" {
			return nil, &ValidationError{Message: fmt.Sprintf("special feature %d needs a title", i+1)}
		}
		if f.Notes, err = trimTo(f.Notes, 1000, "special feature notes"); err != nil {
			return nil, err
		}
		if f.Minutes < 0 || f.Minutes > 2000 || f.Disc < 0 || f.Disc > maxReleaseDiscs {
			return nil, &ValidationError{Message: fmt.Sprintf("special feature %d: minutes or disc out of range", i+1)}
		}
		out = append(out, f)
	}
	return out, nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func validCopy(in DiscCopyInput) (repository.CopyWrite, error) {
	var w repository.CopyWrite
	var err error
	if d := strings.TrimSpace(in.PurchasedOn); d != "" {
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return w, &ValidationError{Message: "purchased_on must be YYYY-MM-DD"}
		}
		w.PurchasedOn = &d
	}
	cur := strings.ToUpper(strings.TrimSpace(in.Currency))
	switch {
	case in.PriceMinor != nil && *in.PriceMinor < 0:
		return w, &ValidationError{Message: "price_minor can't be negative"}
	case in.PriceMinor != nil && cur == "":
		return w, &ValidationError{Message: "a price needs its currency (ISO 4217, e.g. USD)"}
	case cur != "" && !currencyRe.MatchString(cur):
		return w, &ValidationError{Message: "currency must be an ISO 4217 code such as USD"}
	}
	if in.PriceMinor != nil {
		w.PriceMinor, w.Currency = in.PriceMinor, &cur
	}
	if w.Retailer, err = trimTo(in.Retailer, 120, "retailer"); err != nil {
		return w, err
	}
	if !discConditions[in.Condition] {
		return w, &ValidationError{Message: "condition must be sealed, new or used"}
	}
	w.Condition = in.Condition
	notes, err := trimTo(in.Notes, 4000, "notes")
	if err != nil {
		return w, err
	}
	w.Notes = optStr(notes)
	return w, nil
}

// --- reads --------------------------------------------------------------

// views builds release views (with parts, titles and pictures) for
// top-level releases, in three queries whatever their number.
func (s *DiscService) views(ctx context.Context, repo *repository.DiscsRepository, roots []repository.DiscRelease) (map[int64]*DiscReleaseView, error) {
	ids := make([]int64, 0, len(roots))
	for _, r := range roots {
		ids = append(ids, r.ID)
	}
	parts, err := repo.Parts(ctx, ids)
	if err != nil {
		return nil, err
	}
	all := append([]int64{}, ids...)
	for _, ps := range parts {
		for _, p := range ps {
			all = append(all, p.ID)
		}
	}
	titles, err := repo.Titles(ctx, all)
	if err != nil {
		return nil, err
	}
	images, err := repo.Images(ctx, all)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]*DiscReleaseView, len(roots))
	for _, r := range roots {
		v := releaseView(r, titles[r.ID], images[r.ID])
		for _, p := range parts[r.ID] {
			pv := releaseView(p, titles[p.ID], images[p.ID])
			v.DiscCount += pv.DiscCount
			for _, h := range pv.Highlights {
				if !contains(v.Highlights, h) {
					v.Highlights = append(v.Highlights, h)
				}
			}
			v.Parts = append(v.Parts, *pv)
		}
		sortHighlights(v.Highlights)
		out[r.ID] = v
	}
	return out, nil
}

func releaseView(r repository.DiscRelease, titles []repository.DiscReleaseTitle, images []repository.DiscReleaseImage) *DiscReleaseView {
	v := &DiscReleaseView{
		ID: r.ID, ParentID: r.ParentID, Barcode: r.Barcode, Title: r.Title, Edition: r.Edition,
		MediaFormat: r.MediaFormat, Packaging: r.Packaging, HasSlipcover: r.HasSlipcover, Region: r.Region,
		Country: r.Country, Studio: r.Studio, Label: r.Label, ReleaseDate: r.ReleaseDate, Notes: r.Notes,
		Source: r.Source, UpdatedAt: r.UpdatedAt,
		Discs: []DiscSpec{}, SpecialFeatures: []SpecialFeature{}, Titles: []DiscTitleView{},
		Images: []DiscImageView{}, Parts: []DiscReleaseView{}, Highlights: []string{},
	}
	_ = json.Unmarshal([]byte(r.Discs), &v.Discs)
	_ = json.Unmarshal([]byte(r.SpecialFeatures), &v.SpecialFeatures)
	if v.Discs == nil {
		v.Discs = []DiscSpec{}
	}
	if v.SpecialFeatures == nil {
		v.SpecialFeatures = []SpecialFeature{}
	}
	v.DiscCount = len(v.Discs)
	v.Highlights = discHighlights(v.Discs)
	for _, t := range titles {
		tv := DiscTitleView{TmdbKind: t.Kind, TmdbID: t.ID, SeasonNumber: t.SeasonNumber, Title: t.Title, PosterPath: t.PosterPath}
		if len(t.ReleaseDate) >= 4 {
			fmt.Sscanf(t.ReleaseDate[:4], "%d", &tv.Year)
		}
		v.Titles = append(v.Titles, tv)
	}
	for _, im := range images {
		v.Images = append(v.Images, imageView(im))
	}
	return v
}

func imageView(im repository.DiscReleaseImage) DiscImageView {
	return DiscImageView{
		ID: im.ID, Kind: im.Kind, Position: im.Position, Width: im.Width, Height: im.Height, Source: im.Source,
		URL: DiscImageURL(im.SHA256, im.ContentType, false), ThumbURL: DiscImageURL(im.SHA256, im.ContentType, true),
	}
}

// discHighlights lists the HDR formats and immersive audio on any disc.
func discHighlights(discs []DiscSpec) []string {
	out := []string{}
	for _, d := range discs {
		for _, h := range d.HDR {
			if !contains(out, h) {
				out = append(out, h)
			}
		}
		for _, a := range d.Audio {
			if contains(discImmersiveAudio, a.Format) && !contains(out, a.Format) {
				out = append(out, a.Format)
			}
		}
	}
	sortHighlights(out)
	return out
}

var highlightOrder = map[string]int{"dolby_vision": 0, "hdr10_plus": 1, "hdr10": 2, "hlg": 3, "dolby_atmos": 4, "dts_x": 5, "auro_3d": 6}

func sortHighlights(h []string) {
	sort.SliceStable(h, func(i, j int) bool { return highlightOrder[h[i]] < highlightOrder[h[j]] })
}

func copyView(c repository.DiscCopy, rel *DiscReleaseView) DiscCopyView {
	return DiscCopyView{
		ID: c.ID, ReleaseID: c.ReleaseID, PurchasedOn: c.PurchasedOn, PriceMinor: c.PriceMinor, Currency: c.Currency,
		Retailer: c.Retailer, Condition: c.Condition, Notes: c.Notes, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		Release: rel,
	}
}

// DiscListInput filters a shelf.
type DiscListInput struct {
	MediaFormat string
	Query       string
	TmdbKind    string
	TmdbID      string
	Sort        string
}

var discSorts = set("", "added", "title", "purchased", "price", "format")

// List returns a user's shelf with its totals.
func (s *DiscService) List(ctx context.Context, userID int64, in DiscListInput) (map[string]any, error) {
	if in.MediaFormat != "" && !discMediaFormats[in.MediaFormat] {
		return nil, &ValidationError{Message: "format must be one of uhd_bluray, bluray, bluray_3d, dvd, hd_dvd, other"}
	}
	if !discSorts[in.Sort] {
		return nil, &ValidationError{Message: "sort must be added, title, purchased, price or format"}
	}
	if in.TmdbID != "" || in.TmdbKind != "" {
		if err := ValidateRef(in.TmdbKind, in.TmdbID); err != nil {
			return nil, err
		}
	}
	q, err := trimTo(in.Query, 100, "q")
	if err != nil {
		return nil, err
	}
	rows, err := s.read.Copies(ctx, userID, repository.CopyFilter{
		MediaFormat: in.MediaFormat, Query: q, TmdbKind: in.TmdbKind, TmdbID: in.TmdbID, Sort: in.Sort,
	})
	if err != nil {
		return nil, err
	}
	roots := make([]repository.DiscRelease, 0, len(rows))
	seen := map[int64]bool{}
	for _, r := range rows {
		if seen[r.ReleaseID] {
			continue
		}
		seen[r.ReleaseID] = true
		roots = append(roots, repository.DiscRelease{
			ID: r.ReleaseID, Title: r.Title, Edition: r.Edition, MediaFormat: r.MediaFormat, Packaging: r.Packaging,
			Barcode: r.Barcode, Label: r.Label, Studio: r.Studio, HasSlipcover: r.HasSlipcover, Discs: r.Discs,
			SpecialFeatures: "[]",
		})
	}
	views, err := s.views(ctx, s.read, roots)
	if err != nil {
		return nil, err
	}
	items := make([]DiscCopyView, 0, len(rows))
	for _, r := range rows {
		items = append(items, copyView(r.DiscCopy, views[r.ReleaseID]))
	}
	return map[string]any{"items": items, "totals": shelfTotals(items)}, nil
}

func shelfTotals(items []DiscCopyView) DiscShelfTotals {
	t := DiscShelfTotals{ByFormat: map[string]int{}, Spend: []CurrencyAmount{}, Retailers: []RetailerCount{}}
	titles := map[string]bool{}
	spend := map[string]*CurrencyAmount{}
	retailers := map[string]int{}
	for _, c := range items {
		t.Copies++
		r := c.Release
		if r == nil {
			continue
		}
		t.Discs += r.DiscCount
		t.ByFormat[r.MediaFormat]++
		if r.HasSlipcover {
			t.WithSlipcover++
		}
		if len(r.Parts) > 0 || r.Packaging == "box_set" || len(r.Titles) > 1 {
			t.BoxSets++
		}
		for _, ti := range r.Titles {
			titles[ti.TmdbKind+"/"+ti.TmdbID] = true
		}
		for _, p := range r.Parts {
			for _, ti := range p.Titles {
				titles[ti.TmdbKind+"/"+ti.TmdbID] = true
			}
		}
		if c.PriceMinor != nil && c.Currency != nil {
			a := spend[*c.Currency]
			if a == nil {
				a = &CurrencyAmount{Currency: *c.Currency}
				spend[*c.Currency] = a
			}
			a.Minor += *c.PriceMinor
			a.Copies++
		}
		if c.Retailer != "" {
			retailers[c.Retailer]++
		}
	}
	t.Titles = len(titles)
	for _, a := range spend {
		a.AvgMinor = a.Minor / int64(a.Copies)
		t.Spend = append(t.Spend, *a)
	}
	sort.Slice(t.Spend, func(i, j int) bool { return t.Spend[i].Copies > t.Spend[j].Copies })
	for name, n := range retailers {
		t.Retailers = append(t.Retailers, RetailerCount{Retailer: name, Copies: n})
	}
	sort.Slice(t.Retailers, func(i, j int) bool {
		if t.Retailers[i].Copies != t.Retailers[j].Copies {
			return t.Retailers[i].Copies > t.Retailers[j].Copies
		}
		return t.Retailers[i].Retailer < t.Retailers[j].Retailer
	})
	return t
}

// releaseView loads one top-level release's full view.
func (s *DiscService) fullRelease(ctx context.Context, repo *repository.DiscsRepository, id int64) (*DiscReleaseView, error) {
	r, err := repo.Release(ctx, id)
	if err != nil {
		return nil, err
	}
	views, err := s.views(ctx, repo, []repository.DiscRelease{r})
	if err != nil {
		return nil, err
	}
	return views[r.ID], nil
}

// Get returns one of the user's copies with its full release.
func (s *DiscService) Get(ctx context.Context, userID, id int64) (DiscCopyView, error) {
	c, err := s.read.Copy(ctx, userID, id)
	if err != nil {
		return DiscCopyView{}, err
	}
	rel, err := s.fullRelease(ctx, s.read, c.ReleaseID)
	if err != nil {
		return DiscCopyView{}, err
	}
	return copyView(c, rel), nil
}

// GetRelease returns a release (with the user's copies of it). A part
// answers with its box set.
func (s *DiscService) GetRelease(ctx context.Context, userID, id int64) (map[string]any, error) {
	root, err := s.rootOf(ctx, s.read, id)
	if err != nil {
		return nil, err
	}
	rel, err := s.fullRelease(ctx, s.read, root)
	if err != nil {
		return nil, err
	}
	copies, err := s.read.CopiesOfRelease(ctx, userID, root)
	if err != nil {
		return nil, err
	}
	mine := make([]DiscCopyView, 0, len(copies))
	for _, c := range copies {
		mine = append(mine, copyView(c, nil))
	}
	return map[string]any{"release": rel, "copies": mine}, nil
}

func (s *DiscService) rootOf(ctx context.Context, repo *repository.DiscsRepository, id int64) (int64, error) {
	r, err := repo.Release(ctx, id)
	if err != nil {
		return 0, err
	}
	if r.ParentID != nil {
		return *r.ParentID, nil
	}
	return r.ID, nil
}

// --- writes -------------------------------------------------------------

// Add records a copy, of a known release or a new one.
func (s *DiscService) Add(ctx context.Context, userID int64, in AddDiscInput) (DiscCopyView, error) {
	cw, err := validCopy(in.DiscCopyInput)
	if err != nil {
		return DiscCopyView{}, err
	}
	var copyID, releaseID int64
	switch {
	case in.ReleaseID != 0 && in.Release != nil:
		return DiscCopyView{}, &ValidationError{Message: "send release_id or release, not both"}
	case in.ReleaseID != 0:
		root, err := s.rootOf(ctx, s.repo, in.ReleaseID)
		if errors.Is(err, sql.ErrNoRows) {
			return DiscCopyView{}, &ValidationError{Message: "no such release"}
		}
		if err != nil {
			return DiscCopyView{}, err
		}
		if root != in.ReleaseID {
			return DiscCopyView{}, &ValidationError{Message: "that release is part of a box set; add the box set"}
		}
		releaseID = root
		if copyID, err = s.repo.AddCopy(ctx, userID, releaseID, cw); err != nil {
			return DiscCopyView{}, err
		}
	case in.Release != nil:
		if in.Release.ID != 0 {
			return DiscCopyView{}, &ValidationError{Message: "a new release has no id; send release_id to add a copy of a known one"}
		}
		w, err := s.releaseWrite(ctx, *in.Release, 0)
		if err != nil {
			return DiscCopyView{}, err
		}
		for i := range w.Parts {
			if w.Parts[i].ID != 0 {
				return DiscCopyView{}, &ValidationError{Message: "the parts of a new box set have no ids"}
			}
		}
		w.Source = "manual"
		if w.Barcode != nil {
			if _, err := s.repo.CachedLookup(ctx, *w.Barcode, "upcitemdb"); err == nil {
				w.Source = "upcitemdb"
			}
		}
		releaseID, copyID, _, err = s.repo.SaveReleaseTree(ctx, userID, w, &cw)
		if errors.Is(err, repository.ErrDiscBarcodeTaken) {
			return DiscCopyView{}, &ConflictError{Message: "a release with this barcode exists already; add a copy of it with release_id"}
		}
		if err != nil {
			return DiscCopyView{}, err
		}
	default:
		return DiscCopyView{}, &ValidationError{Message: "release_id or release is required"}
	}

	// Pictures are best effort: the copy is recorded either way, and a
	// picture that couldn't be fetched can be added again later.
	for _, im := range in.ImportImages {
		target := releaseID
		if im.ReleaseID != 0 {
			if root, err := s.rootOf(ctx, s.repo, im.ReleaseID); err != nil || root != releaseID {
				continue
			}
			target = im.ReleaseID
		}
		if _, err := s.importImage(ctx, userID, releaseID, target, im.Kind, im.URL); err != nil {
			s.log.Warn("disc picture import failed", slog.Int64("release_id", target), slog.Any("err", err))
		}
	}
	return s.Get(ctx, userID, copyID)
}

// UpdateCopy changes what the user paid, where, when and the condition.
func (s *DiscService) UpdateCopy(ctx context.Context, userID, id int64, in DiscCopyInput) (DiscCopyView, error) {
	cw, err := validCopy(in)
	if err != nil {
		return DiscCopyView{}, err
	}
	if err := s.repo.UpdateCopy(ctx, userID, id, cw); err != nil {
		return DiscCopyView{}, err
	}
	return s.Get(ctx, userID, id)
}

// DeleteCopy takes a copy off the user's shelf. The release stays, as
// reference data for the next person who scans its barcode.
func (s *DiscService) DeleteCopy(ctx context.Context, userID, id int64) error {
	return s.repo.DeleteCopy(ctx, userID, id)
}

// canEdit: a user may change a release they own a copy of.
func (s *DiscService) canEdit(ctx context.Context, userID, releaseID int64) (int64, error) {
	root, err := s.rootOf(ctx, s.repo, releaseID)
	if err != nil {
		return 0, err
	}
	copies, err := s.repo.CopiesOfRelease(ctx, userID, root)
	if err != nil {
		return 0, err
	}
	if len(copies) == 0 {
		return 0, &ForbiddenError{Message: "add a copy of this release to your shelf before editing it"}
	}
	return root, nil
}

// UpdateRelease replaces a release's details, titles and box-set parts.
// Parts sent without an id are added; stored parts not sent are deleted
// with their pictures.
func (s *DiscService) UpdateRelease(ctx context.Context, userID, id int64, in DiscReleaseInput) (map[string]any, error) {
	root, err := s.canEdit(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if root != id {
		return nil, &ValidationError{Message: "this release is part of a box set; edit the box set"}
	}
	in.ID = id
	w, err := s.releaseWrite(ctx, in, 0)
	if err != nil {
		return nil, err
	}
	_, _, removed, err := s.repo.SaveReleaseTree(ctx, userID, w, nil)
	if errors.Is(err, repository.ErrDiscBarcodeTaken) {
		return nil, &ConflictError{Message: "another release has this barcode"}
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &ValidationError{Message: "a part id doesn't belong to this box set"}
	}
	if err != nil {
		return nil, err
	}
	s.removeUnused(ctx, removed)
	return s.GetRelease(ctx, userID, id)
}

// removeUnused deletes picture files no release uses any more.
func (s *DiscService) removeUnused(ctx context.Context, shas []string) {
	for _, sha := range shas {
		used, err := s.repo.ImageFileInUse(ctx, sha)
		if err != nil || used {
			continue
		}
		s.store.removeAny(sha)
	}
}

// AddImage stores an uploaded picture of a release (or of a box set's part).
func (s *DiscService) AddImage(ctx context.Context, userID, releaseID int64, kind string, data []byte) (DiscImageView, error) {
	if !discImageKinds[kind] {
		return DiscImageView{}, &ValidationError{Message: "kind must be front, back, slip_front, slip_back, spine, disc, booklet, inside or other"}
	}
	if _, err := s.canEdit(ctx, userID, releaseID); err != nil {
		return DiscImageView{}, err
	}
	return s.storeImage(ctx, userID, releaseID, kind, data, "upload", nil)
}

func (s *DiscService) storeImage(ctx context.Context, userID, releaseID int64, kind string, data []byte, source string, sourceURL *string) (DiscImageView, error) {
	p, err := processDiscImage(ctx, data)
	if err != nil {
		return DiscImageView{}, err
	}
	if err := s.store.save(p); err != nil {
		return DiscImageView{}, err
	}
	im, err := s.repo.AddImage(ctx, repository.NewDiscImage{
		ReleaseID: releaseID, Kind: kind, SHA256: p.sha, ContentType: p.contentType,
		Width: int64(p.width), Height: int64(p.height), Bytes: int64(len(p.full)),
		Source: source, SourceURL: sourceURL, UploadedBy: userID,
	})
	if err != nil {
		s.removeUnused(ctx, []string{p.sha})
		return DiscImageView{}, err
	}
	return imageView(im), nil
}

// ImportImage fetches a picture for a release from a URL a lookup offered
// (UPCitemdb's product images) or a TMDB poster.
func (s *DiscService) ImportImage(ctx context.Context, userID, releaseID int64, kind, rawURL string) (DiscImageView, error) {
	root, err := s.canEdit(ctx, userID, releaseID)
	if err != nil {
		return DiscImageView{}, err
	}
	return s.importImage(ctx, userID, root, releaseID, kind, rawURL)
}

func (s *DiscService) importImage(ctx context.Context, userID, root, releaseID int64, kind, rawURL string) (DiscImageView, error) {
	if !discImageKinds[kind] {
		return DiscImageView{}, &ValidationError{Message: "kind must be front, back, slip_front, slip_back, spine, disc, booklet, inside or other"}
	}
	ok, err := s.offeredImage(ctx, root, releaseID, rawURL)
	if err != nil {
		return DiscImageView{}, err
	}
	if !ok {
		return DiscImageView{}, &ValidationError{Message: "only pictures a barcode lookup offered for this release, or TMDB posters, can be fetched; upload others"}
	}
	data, err := s.fetcher.fetch(ctx, rawURL)
	if err != nil {
		return DiscImageView{}, err
	}
	source := "upcitemdb"
	if strings.HasPrefix(rawURL, tmdbImageBase) {
		source = "tmdb"
	}
	return s.storeImage(ctx, userID, releaseID, kind, data, source, &rawURL)
}

const tmdbImageBase = "https://image.tmdb.org/t/p/"

var tmdbImagePathRe = regexp.MustCompile(`^https://image\.tmdb\.org/t/p/(original|w[0-9]{2,4})/[A-Za-z0-9]+\.(jpg|png)$`)

// offeredImage reports whether a URL is one metareel offered: a TMDB image,
// or a picture the cached barcode lookup of the release (or its box set)
// returned. Nothing else is fetched, so the endpoint can't be used to make
// the server request arbitrary URLs.
func (s *DiscService) offeredImage(ctx context.Context, root, releaseID int64, rawURL string) (bool, error) {
	if tmdbImagePathRe.MatchString(rawURL) {
		return true, nil
	}
	for _, id := range []int64{releaseID, root} {
		r, err := s.repo.Release(ctx, id)
		if err != nil {
			return false, err
		}
		if r.Barcode == nil {
			continue
		}
		cached, err := s.repo.CachedLookup(ctx, *r.Barcode, "upcitemdb")
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return false, err
		}
		var items []client.UPCItem
		_ = json.Unmarshal([]byte(cached.Payload), &items)
		for _, it := range items {
			if contains(it.Images, rawURL) {
				return true, nil
			}
		}
	}
	return false, nil
}

// UpdateImage changes a picture's kind or order.
func (s *DiscService) UpdateImage(ctx context.Context, userID, releaseID, imageID int64, kind string, position int64) (DiscImageView, error) {
	if !discImageKinds[kind] {
		return DiscImageView{}, &ValidationError{Message: "kind must be front, back, slip_front, slip_back, spine, disc, booklet, inside or other"}
	}
	if position < 0 || position > 1000 {
		return DiscImageView{}, &ValidationError{Message: "position must be between 0 and 1000"}
	}
	if _, err := s.canEdit(ctx, userID, releaseID); err != nil {
		return DiscImageView{}, err
	}
	if err := s.repo.UpdateImage(ctx, releaseID, imageID, kind, position); err != nil {
		return DiscImageView{}, err
	}
	im, err := s.repo.Image(ctx, releaseID, imageID)
	if err != nil {
		return DiscImageView{}, err
	}
	return imageView(im), nil
}

// DeleteImage removes a picture (and its file, when nothing else uses it).
func (s *DiscService) DeleteImage(ctx context.Context, userID, releaseID, imageID int64) error {
	if _, err := s.canEdit(ctx, userID, releaseID); err != nil {
		return err
	}
	im, err := s.repo.Image(ctx, releaseID, imageID)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteImage(ctx, releaseID, imageID); err != nil {
		return err
	}
	s.removeUnused(ctx, []string{im.SHA256})
	return nil
}

// OpenImage opens a stored picture file by its public name.
// The caller closes the file; a bad name is os.ErrNotExist.
func (s *DiscService) OpenImage(name string) (*os.File, string, error) {
	return s.store.Open(name)
}
