package client

import (
	"context"

	graphql "github.com/hasura/go-graphql-client"
)

// Date is JustWatch's Date scalar (YYYY-MM-DD).
type Date string

func (Date) GetGraphQLType() string { return "Date" }

// NewPageType selects JustWatch's "new" (added on a date) or "upcoming"
// (announced, not yet released) listings.
type NewPageType string

func (NewPageType) GetGraphQLType() string { return "NewPageType" }

const (
	NewPageTypeNew      NewPageType = "NEW"
	NewPageTypeUpcoming NewPageType = "UPCOMING"
)

// JWPackage is a streaming service as JustWatch names it in a country.
type JWPackage struct {
	PackageID     int
	ShortName     string // e.g. "nfx"; used in filters
	ClearName     string // e.g. "Netflix"
	TechnicalName string // e.g. "netflix"
}

type getPackagesQuery struct {
	Packages []JWPackage `graphql:"packages(country: $country, platform: WEB, includeAddons: false)"`
}

// GetPackages lists the streaming services JustWatch knows in a country.
func (c *JustWatchClient) GetPackages(ctx context.Context, country string) ([]JWPackage, error) {
	var q getPackagesQuery
	if err := c.client.Query(ctx, &q, map[string]interface{}{"country": Country(country)}); err != nil {
		return nil, err
	}
	return q.Packages, nil
}

// NewTitleContent is the content shared by movies, seasons and shows.
type NewTitleContent struct {
	Title               string
	ShortDescription    *string
	FullPath            *string
	OriginalReleaseYear *int
	OriginalReleaseDate *string
	Runtime             *int
	PosterURL           *string `graphql:"posterUrl(profile: S592, format: JPG)"`
	ExternalIds         ExternalIds
	Scoring             Scoring
	Genres              []JWGenre
}

type JWGenre struct {
	ShortName   string  // e.g. "drm"
	Translation *string `graphql:"translation(language: $language)"` // e.g. "Drama"
}

// NewTitleEdge is one entry of a "new" or "upcoming" listing. Node is a
// movie or a show season; for seasons, Show carries the show's content.
type NewTitleEdge struct {
	Cursor   string
	NewOffer *struct {
		MonetizationType *string
		PresentationType *string
		StandardWebURL   *string `graphql:"standardWebURL"`
		DateCreated      *string
		Package          struct{ ShortName string }
	} `graphql:"newOffer(platform: WEB)"`
	Node struct {
		MovieOrSeason struct {
			ID         string `graphql:"id"`
			ObjectType string
			Content    struct {
				NewTitleContent
				Season struct {
					SeasonNumber *int
				} `graphql:"... on SeasonContent"`
				IsReleased       *bool `graphql:"isReleasedV2(country: $country)"`
				UpcomingReleases []struct {
					ReleaseDate *string
					ReleaseType *string
					Package     *struct{ ShortName string }
				}
			} `graphql:"content(country: $country, language: $language)"`
			Season struct {
				Show *struct {
					ID      string          `graphql:"id"`
					Content NewTitleContent `graphql:"content(country: $country, language: $language)"`
				}
			} `graphql:"... on Season"`
		} `graphql:"... on MovieOrSeason"`
	}
}

type getNewTitlesQuery struct {
	NewTitles struct {
		TotalCount int
		PageInfo   PageInfo
		Edges      []NewTitleEdge
	} `graphql:"newTitles(country: $country, date: $date, filter: $filter, after: $after, first: $first, pageType: $pageType, priceDrops: false)"`
}

// NewTitlesPage is one page of a "new" or "upcoming" listing.
type NewTitlesPage struct {
	TotalCount int
	PageInfo   PageInfo
	Edges      []NewTitleEdge
}

// GetNewTitles returns one page of titles added to a service on date
// (NewPageTypeNew) or announced for it (NewPageTypeUpcoming, which ignores
// date). objectTypes may be empty for both movies and shows.
func (c *JustWatchClient) GetNewTitles(ctx context.Context, pageType NewPageType, country, language, date, pack string, objectTypes []ObjectType, after string, first int) (*NewTitlesPage, error) {
	if objectTypes == nil {
		objectTypes = []ObjectType{}
	}
	var q getNewTitlesQuery
	variables := map[string]interface{}{
		"country":  Country(country),
		"language": Language(language),
		"date":     Date(date),
		"pageType": pageType,
		"first":    graphql.Int(first),
		"after":    graphql.String(after),
		"filter": TitleFilter{
			AgeCertifications:          []string{},
			ExcludeGenres:              []string{},
			ExcludeProductionCountries: []string{},
			Genres:                     []string{},
			ObjectTypes:                objectTypes,
			ProductionCountries:        []string{},
			Packages:                   []Package{Package(pack)},
			PresentationTypes:          []string{},
			MonetizationTypes:          []string{},
		},
	}
	if err := c.client.Query(ctx, &q, variables); err != nil {
		return nil, err
	}
	return &NewTitlesPage{TotalCount: q.NewTitles.TotalCount, PageInfo: q.NewTitles.PageInfo, Edges: q.NewTitles.Edges}, nil
}
