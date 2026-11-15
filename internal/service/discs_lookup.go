package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
)

const (
	// How long a barcode service's answer is reused. Products don't change;
	// a miss is retried sooner because the service may add the product.
	lookupFoundTTL    = 180 * 24 * time.Hour
	lookupNotFoundTTL = 14 * 24 * time.Hour
	lookupProvider    = "upcitemdb"
)

// DiscLookup is what metareel knows about a scanned barcode.
type DiscLookup struct {
	Barcode string `json:"barcode"`
	// Status: known (a release has it), found (the barcode service knows the
	// product), not_found, rate_limited, unavailable (lookups are off or
	// failed). Only known and found carry data.
	Status string `json:"status"`
	// Release is the release with this barcode; OwnedCopyIDs are the
	// user's copies of it.
	Release      *DiscReleaseView `json:"release,omitempty"`
	OwnedCopyIDs []int64          `json:"owned_copy_ids"`
	// Suggestion is a release to start from, worked out from the product.
	Suggestion *DiscSuggestion `json:"suggestion,omitempty"`
	// Candidates are TMDB titles the product may be; Collections are TMDB
	// collections for a box set.
	Candidates  []TitleSearchResult `json:"candidates"`
	Collections []DiscCollection    `json:"collections"`
}

// DiscSuggestion is a release draft from a barcode service's product.
type DiscSuggestion struct {
	ProductTitle string   `json:"product_title"`
	Title        string   `json:"title"` // the product title without format and marketing words
	Edition      string   `json:"edition"`
	MediaFormat  string   `json:"media_format"` // "" when the product doesn't say
	Packaging    string   `json:"packaging"`
	HasSlipcover bool     `json:"has_slipcover"`
	Studio       string   `json:"studio"`
	Label        string   `json:"label"`
	BoxSet       bool     `json:"box_set"`
	Seasons      []int    `json:"seasons"`
	Images       []string `json:"images"` // retailer pictures; fetch with import_images
	Description  string   `json:"description"`
	Source       string   `json:"source"`
}

// DiscCollection is a TMDB collection with its films.
type DiscCollection struct {
	ID         int                 `json:"id"`
	Name       string              `json:"name"`
	PosterPath string              `json:"poster_path,omitempty"`
	Parts      []TitleSearchResult `json:"parts"`
}

// Lookup answers a scanned barcode: the release metareel already has, or a
// draft from the barcode service with TMDB candidates.
func (s *DiscService) Lookup(ctx context.Context, userID int64, raw string) (*DiscLookup, error) {
	code, err := NormalizeBarcode(raw)
	if err != nil {
		return nil, err
	}
	out := &DiscLookup{Barcode: code, OwnedCopyIDs: []int64{}, Candidates: []TitleSearchResult{}, Collections: []DiscCollection{}}

	rel, err := s.read.ReleaseByBarcode(ctx, code)
	switch {
	case err == nil:
		root := rel.ID
		if rel.ParentID != nil {
			root = *rel.ParentID
		}
		if out.Release, err = s.fullRelease(ctx, s.read, root); err != nil {
			return nil, err
		}
		copies, err := s.read.CopiesOfRelease(ctx, userID, root)
		if err != nil {
			return nil, err
		}
		for _, c := range copies {
			out.OwnedCopyIDs = append(out.OwnedCopyIDs, c.ID)
		}
		out.Status = "known"
		return out, nil
	case !errors.Is(err, sql.ErrNoRows):
		return nil, err
	}

	items, status := s.lookupProduct(ctx, code)
	out.Status = status
	if len(items) == 0 {
		return out, nil
	}
	out.Suggestion = suggestionFrom(items)
	s.addCandidates(ctx, out)
	return out, nil
}

// lookupProduct asks the barcode service (or its cache) about a code.
func (s *DiscService) lookupProduct(ctx context.Context, code string) ([]client.UPCItem, string) {
	cached, err := s.repo.CachedLookup(ctx, code, lookupProvider)
	if err == nil {
		fetched, perr := time.Parse("2006-01-02 15:04:05", cached.FetchedAt)
		ttl := lookupNotFoundTTL
		if cached.Found {
			ttl = lookupFoundTTL
		}
		if perr == nil && s.now().Sub(fetched) < ttl {
			if !cached.Found {
				return nil, "not_found"
			}
			var items []client.UPCItem
			if json.Unmarshal([]byte(cached.Payload), &items) == nil && len(items) > 0 {
				return items, "found"
			}
		}
	}
	if s.barcodes == nil {
		return nil, "unavailable"
	}
	items, err := s.barcodes.Lookup(ctx, upcFromGTIN(code))
	if errors.Is(err, client.ErrBarcodeRateLimited) {
		return nil, "rate_limited"
	}
	if err != nil {
		s.log.Warn("barcode lookup failed", slog.String("provider", lookupProvider), slog.String("barcode", code), slog.Any("err", err))
		return nil, "unavailable"
	}
	payload, _ := json.Marshal(items)
	if err := s.repo.SaveLookup(ctx, code, lookupProvider, len(items) > 0, string(payload)); err != nil {
		s.log.Warn("cache barcode lookup", slog.String("barcode", code), slog.Any("err", err))
	}
	if len(items) == 0 {
		return nil, "not_found"
	}
	return items, "found"
}

func (s *DiscService) addCandidates(ctx context.Context, out *DiscLookup) {
	sg := out.Suggestion
	if sg.Title == "" || s.titles == nil {
		return
	}
	kind := "movie"
	if len(sg.Seasons) > 0 || tvHintRe.MatchString(sg.ProductTitle) {
		kind = "tv"
	}
	query := sg.Title
	if kind == "tv" {
		query = strings.TrimSpace(seasonRe.ReplaceAllString(query, ""))
	}
	if sg.BoxSet && kind == "movie" {
		query = strings.TrimSpace(boxWordsRe.ReplaceAllString(query, ""))
		if cols, err := s.SearchCollections(ctx, query, 3); err == nil {
			// TMDB's collection search is fuzzy; a scan only offers ones named
			// after what the box says.
			for _, c := range cols {
				if strings.Contains(strings.ToLower(c.Name), strings.ToLower(query)) {
					out.Collections = append(out.Collections, c)
				}
			}
		} else {
			s.log.Warn("tmdb collection search", slog.String("query", query), slog.Any("err", err))
		}
	}
	if query == "" {
		return
	}
	hits, err := s.titles.Search(ctx, query, kind, 8)
	if err != nil {
		s.log.Warn("tmdb search for a disc", slog.String("query", query), slog.Any("err", err))
		return
	}
	out.Candidates = hits
}

// SearchCollections finds TMDB collections (with their films) for a box set.
func (s *DiscService) SearchCollections(ctx context.Context, query string, limit int) ([]DiscCollection, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, &ValidationError{Message: "q is required"}
	}
	if s.collections == nil || !s.collections.Enabled() {
		return nil, fmt.Errorf("%w: TMDB_API_KEY is not set", ErrUpstream)
	}
	if limit <= 0 || limit > 10 {
		limit = 5
	}
	found, err := s.collections.SearchCollections(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w: tmdb collection search: %v", ErrUpstream, err)
	}
	if len(found) > limit {
		found = found[:limit]
	}
	out := make([]DiscCollection, 0, len(found))
	for _, f := range found {
		col, err := s.collections.Collection(ctx, f.ID)
		if err != nil {
			return nil, fmt.Errorf("%w: tmdb collection: %v", ErrUpstream, err)
		}
		dc := DiscCollection{ID: col.ID, Name: col.Name, PosterPath: col.PosterPath, Parts: []TitleSearchResult{}}
		for _, p := range col.Parts {
			dc.Parts = append(dc.Parts, TitleSearchResult{
				Kind: "movie", ID: strconv.Itoa(p.ID), Title: p.Title, OriginalTitle: p.OriginalTitle,
				Year: p.Year, ReleaseDate: p.ReleaseDate, PosterPath: p.PosterPath,
			})
		}
		out = append(out, dc)
	}
	return out, nil
}

var (
	uhdRe        = regexp.MustCompile(`(?i)\b(4k|uhd|ultra[ -]?hd)\b`)
	bluRe        = regexp.MustCompile(`(?i)\bblu[- ]?ray\b|\bbd\b`)
	threeDRe     = regexp.MustCompile(`(?i)\b3[- ]?d\b`)
	hdDVDRe      = regexp.MustCompile(`(?i)\bhd[- ]?dvd\b`)
	dvdRe        = regexp.MustCompile(`(?i)\bdvd\b`)
	steelbookRe  = regexp.MustCompile(`(?i)\bsteel ?book\b`)
	slipRe       = regexp.MustCompile(`(?i)\bslip ?(cover|case)\b`)
	boxSetRe     = regexp.MustCompile(`(?i)\b(trilogy|quadrilogy|quintology|saga|anthology|box ?set|collection|complete series|\d+[- ]film|\d+[- ]movie|double feature|seasons? \d+ ?(-|to|&) ?\d+)\b`)
	boxWordsRe   = regexp.MustCompile(`(?i)\b(the )?(complete |ultimate |4k |[0-9]+[- ](film|movie) )?(trilogy|quadrilogy|quintology|saga|anthology|box ?set|collection|double feature)\b`)
	tvHintRe     = regexp.MustCompile(`(?i)\b(season|seasons|series|the complete series|miniseries|episodes)\b`)
	seasonRe     = regexp.MustCompile(`(?i)\s*[:\-,]?\s*\b(?:the\s+)?(?:complete\s+series|(?:complete\s+)?(?:first|second|third|fourth|fifth|sixth|seventh|eighth|ninth|tenth|final)\s+season|(?:complete\s+)?seasons?\s+\d{1,2}(?:\s*(?:-|to|&)\s*\d{1,2})?)\b`)
	seasonNumRe  = regexp.MustCompile(`(?i)\bseasons?\s+(\d{1,2})(?:\s*(?:-|to|&)\s*(\d{1,2}))?`)
	editionRe    = regexp.MustCompile(`(?i)\b((?:(?:ultimate|collector'?s|special|limited|deluxe|definitive|signature|premium|extended|theatrical|director'?s|final|remastered|restored|unrated|uncut|anniversary|\d+(?:st|nd|rd|th)|international|criterion|steelbook|standard)[ -]){1,4}(?:edition|cut))\b`)
	bracketRe    = regexp.MustCompile(`\s*[\[(][^\])]*[\])]`)
	noiseRe      = regexp.MustCompile(`(?i)\b(4k|uhd|ultra[ -]?hd|blu[- ]?ray|hd[- ]?dvd|dvd|digital( copy| code| hd)?|\+|combo( pack)?|widescreen|full ?screen|region (free|[a-c1-6])|includes?|w/|with|steel ?book|slip ?(cover|case)|3[- ]?d|2[- ]?d|hdr|dolby vision|atmos|[0-9]+[- ]?dis[ck]s?|disc|set|limited|exclusive|import|brand new|new|sealed|movie)\b`)
	spaceRe      = regexp.MustCompile(`\s{2,}`)
	multiSpaceRe = regexp.MustCompile(`\s{2,}|\t`)
	trailPunctRe = regexp.MustCompile(`^[\s:;,\-–—/|]+|[\s:;,\-–—/|]+$`)
)

var ordinalSeasons = map[string]int{"first": 1, "second": 2, "third": 3, "fourth": 4, "fifth": 5, "sixth": 6, "seventh": 7, "eighth": 8, "ninth": 9, "tenth": 10}

// labelHints maps boutique labels named in product titles to their format slugs.
var labelHints = []struct {
	re   *regexp.Regexp
	slug string
}{
	{regexp.MustCompile(`(?i)\bcriterion\b`), "criterion"},
	{regexp.MustCompile(`(?i)\barrow (video|academy|films?)\b|\barrow\b`), "arrow-video"},
	{regexp.MustCompile(`(?i)\bshout!? ?factory\b|\bscream factory\b`), "shout-factory"},
	{regexp.MustCompile(`(?i)\bkino lorber\b`), "kino-lorber"},
	{regexp.MustCompile(`(?i)\bvinegar syndrome\b`), "vinegar-syndrome"},
	{regexp.MustCompile(`(?i)\bsecond sight\b`), "second-sight"},
	{regexp.MustCompile(`(?i)\beureka\b|\bmasters of cinema\b`), "eureka"},
	{regexp.MustCompile(`(?i)\bindicator\b|\bpowerhouse films\b`), "indicator"},
	{regexp.MustCompile(`(?i)\b88 films\b`), "88-films"},
	{regexp.MustCompile(`(?i)\bseverin\b`), "severin"},
	{regexp.MustCompile(`(?i)\bradiance films\b`), "radiance"},
	{regexp.MustCompile(`(?i)\bumbrella entertainment\b`), "umbrella"},
	{regexp.MustCompile(`(?i)\bimprint films\b`), "imprint"},
	{regexp.MustCompile(`(?i)\bbfi\b`), "bfi"},
	{regexp.MustCompile(`(?i)\bstudiocanal\b`), "studiocanal"},
}

// suggestionFrom drafts a release from a barcode service's products (the
// first one, filling gaps from the rest).
func suggestionFrom(items []client.UPCItem) *DiscSuggestion {
	first := items[0]
	sg := parseProductTitle(first.Title)
	sg.Studio = strings.TrimSpace(first.Brand)
	sg.Description = strings.TrimSpace(first.Description)
	if len([]rune(sg.Description)) > 2000 {
		sg.Description = string([]rune(sg.Description)[:2000])
	}
	sg.Source = lookupProvider
	sg.Images = []string{}
	for _, it := range items {
		for _, im := range it.Images {
			if (strings.HasPrefix(im, "https://") || strings.HasPrefix(im, "http://")) && !contains(sg.Images, im) {
				sg.Images = append(sg.Images, im)
			}
		}
		if sg.MediaFormat == "" {
			sg.MediaFormat = parseProductTitle(it.Title).MediaFormat
		}
	}
	if len(sg.Images) > 12 {
		sg.Images = sg.Images[:12]
	}
	return sg
}

// parseProductTitle reads what a retailer's product title says about a
// release: "The Dark Knight Trilogy (4K Ultra HD + Blu-ray) [Steelbook]".
func parseProductTitle(product string) *DiscSuggestion {
	product = strings.TrimSpace(product)
	sg := &DiscSuggestion{ProductTitle: product, Seasons: []int{}}
	switch {
	case uhdRe.MatchString(product):
		sg.MediaFormat = "uhd_bluray"
	case hdDVDRe.MatchString(product):
		sg.MediaFormat = "hd_dvd"
	case bluRe.MatchString(product) && threeDRe.MatchString(product):
		sg.MediaFormat = "bluray_3d"
	case bluRe.MatchString(product):
		sg.MediaFormat = "bluray"
	case dvdRe.MatchString(product):
		sg.MediaFormat = "dvd"
	}
	if steelbookRe.MatchString(product) {
		sg.Packaging = "steelbook"
	}
	sg.HasSlipcover = slipRe.MatchString(product)
	sg.BoxSet = boxSetRe.MatchString(product)
	if sg.BoxSet && sg.Packaging == "" {
		sg.Packaging = "box_set"
	}
	for _, l := range labelHints {
		if l.re.MatchString(product) {
			sg.Label = l.slug
			break
		}
	}
	if m := editionRe.FindStringSubmatch(product); m != nil {
		ed := strings.TrimSpace(noiseRe.ReplaceAllString(m[1], " "))
		ed = strings.TrimSpace(trailPunctRe.ReplaceAllString(spaceRe.ReplaceAllString(ed, " "), ""))
		if ed != "" && !strings.EqualFold(ed, "edition") && !strings.EqualFold(ed, "cut") {
			sg.Edition = ed
		}
	}
	if sg.Edition == "" && sg.Packaging == "steelbook" && len(sg.Seasons) == 0 {
		sg.Edition = "Steelbook"
	}
	if m := seasonNumRe.FindStringSubmatch(product); m != nil {
		from, _ := strconv.Atoi(m[1])
		to := from
		if m[2] != "" {
			to, _ = strconv.Atoi(m[2])
		}
		for n := from; n <= to && n-from < 40; n++ {
			sg.Seasons = append(sg.Seasons, n)
		}
	} else {
		lower := strings.ToLower(product)
		for word, n := range ordinalSeasons {
			if strings.Contains(lower, word+" season") {
				sg.Seasons = []int{n}
				break
			}
		}
	}

	// Some listings append "  Genre  Studio" after runs of spaces; the title
	// is what comes before.
	title := product
	if parts := multiSpaceRe.Split(title, 2); len(parts) == 2 && strings.TrimSpace(parts[0]) != "" {
		title = parts[0]
	}
	title = bracketRe.ReplaceAllString(title, " ")
	if m := seasonRe.FindString(title); m != "" {
		if sg.Edition == "" {
			sg.Edition = strings.TrimSpace(trailPunctRe.ReplaceAllString(m, ""))
		}
		title = strings.Replace(title, m, " ", 1)
	}
	if sg.Edition != "" {
		title = strings.Replace(title, sg.Edition, " ", 1)
	}
	title = editionRe.ReplaceAllString(title, " ")
	title = noiseRe.ReplaceAllString(title, " ")
	title = spaceRe.ReplaceAllString(title, " ")
	title = trailPunctRe.ReplaceAllString(strings.TrimSpace(title), "")
	sg.Title = strings.TrimSpace(title)
	return sg
}
