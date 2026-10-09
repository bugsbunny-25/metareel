package client

import (
	"context"

	"net/http"

	graphql "github.com/hasura/go-graphql-client"
)

type JustWatchClient struct {
	client *graphql.Client
}

func NewJustWatchClient(httpClient *http.Client) *JustWatchClient {
	return &JustWatchClient{client: graphql.NewClient("https://apis.justwatch.com/graphql", httpClient)}
}

type ObjectType string

const (
	ObjectTypeMovie  ObjectType = "MOVIE"
	ObjectTypeTVShow ObjectType = "SHOW"
)

type Package string

// JustWatch package short names. Some services use a different code per
// country (e.g. Prime Video is "amp" in US/GB/DE/JP but "prv" in IN/BR).
const (
	PackageAmazonPrime          Package = "amp"
	PackageAmazonPrimeVideo     Package = "prv"
	PackageAppleTVPlus          Package = "atp"
	PackageDisneyPlus           Package = "dnp"
	PackageHBOMax               Package = "mxx"
	PackageHBOMaxUNext          Package = "mxu" // JP
	PackageNetflix              Package = "nfx"
	PackageParamountPlus        Package = "pmp"
	PackageParamountPlusPremium Package = "ppp"
	PackagePeacock              Package = "pct"
)

type Country string
type Language string
type PopularTitlesSorting string

func (c Country) GetGraphQLType() string { return "Country" }

func (l Language) GetGraphQLType() string { return "Language" }

func (s PopularTitlesSorting) GetGraphQLType() string { return "PopularTitlesSorting" }

type ExternalIds struct {
	ImdbId     *string
	TmdbId     *string
	WikidataId *string
}

// Scoring numbers are floats: JustWatch sends large counts in exponent form
// (e.g. imdbVotes 1.305019e+06), which fails to decode into an int and used
// to fail the whole query.
type Scoring struct {
	ImdbScore      *float64
	ImdbVotes      *float64
	TmdbScore      *float64
	TmdbPopularity *float64
	JwRating       *float64
	TomatoMeter    *float64
	CertifiedFresh *bool
}

type MovieOrShowFragment struct {
	Id      string
	Content struct {
		Title               string
		FullPath            *string
		OriginalReleaseYear *int
		ExternalIds         ExternalIds
		Scoring             Scoring
	} `graphql:"content(country: $country, language: $language)"`
}

type TitleFilter struct {
	AgeCertifications          []string     `json:"ageCertifications"`
	ExcludeGenres              []string     `json:"excludeGenres"`
	ExcludeProductionCountries []string     `json:"excludeProductionCountries"`
	Genres                     []string     `json:"genres"`
	ObjectTypes                []ObjectType `json:"objectTypes"`
	ProductionCountries        []string     `json:"productionCountries"`
	Packages                   []Package    `json:"packages"`
	ExcludeIrrelevantTitles    bool         `json:"excludeIrrelevantTitles"`
	PresentationTypes          []string     `json:"presentationTypes"`
	MonetizationTypes          []string     `json:"monetizationTypes"`
	SearchQuery                string       `json:"searchQuery"`
}

type GetTitlesByPathQuery struct {
	UrlV2 struct {
		Node struct {
			MovieOrShowFragment `graphql:"... on MovieOrShow"`
		}
	} `graphql:"urlV2(fullPath: $fullPath)"`
}

type GetTitleByIDQuery struct {
	Node struct {
		MovieOrShowFragment `graphql:"... on MovieOrShow"`
	} `graphql:"node(id: $id)"`
}

type PageInfo struct {
	StartCursor     string
	EndCursor       string
	HasNextPage     bool
	HasPreviousPage bool
}

type GetTitlesByTopSearchPopularQuery struct {
	PoupularTitles struct {
		TotalCount int
		PageInfo   PageInfo
		Edges      []struct {
			Cursor string
			Node   MovieOrShowFragment
		}
	} `graphql:"popularTitles(sortBy: $popularTitlesSortBy, first: $first, sortRandomSeed: $sortRandomSeed, after: $popularAfterCursor, offset: $offset, filter: $popularTitlesFilter, country: $country)"`
}

func (c *JustWatchClient) GetTitlesByPath(ctx context.Context, fullPath string, country string, language string) (*GetTitlesByPathQuery, error) {
	var query GetTitlesByPathQuery
	variables := map[string]interface{}{
		"fullPath": fullPath,
		"country":  Country(country),
		"language": Language(language),
	}
	err := c.client.Query(ctx, &query, variables)
	if err != nil {
		return nil, err
	}
	return &query, nil
}

// GetTitlesByTopSearchPopular returns up to first titles matching searchQuery
// on any of packages, ordered by JustWatch trending rank. No packages
// searches all services.
func (c *JustWatchClient) GetTitlesByTopSearchPopular(ctx context.Context, searchQuery string, first int, country string, language string, objectType ObjectType, packages []Package) (*GetTitlesByTopSearchPopularQuery, error) {
	var query GetTitlesByTopSearchPopularQuery
	if packages == nil {
		packages = []Package{}
	}
	variables := map[string]interface{}{
		"popularTitlesSortBy": PopularTitlesSorting("TRENDING"),
		"first":               graphql.Int(first),
		"sortRandomSeed":      graphql.Int(0),
		"popularAfterCursor":  graphql.String(""),
		"offset":              graphql.Int(0),
		"popularTitlesFilter": TitleFilter{
			AgeCertifications:          []string{},
			ExcludeGenres:              []string{},
			ExcludeProductionCountries: []string{},
			Genres:                     []string{},
			ObjectTypes:                []ObjectType{objectType},
			ProductionCountries:        []string{},
			Packages:                   packages,
			ExcludeIrrelevantTitles:    false,
			PresentationTypes:          []string{},
			MonetizationTypes:          []string{},
			SearchQuery:                searchQuery,
		},
		"language": Language(language),
		"country":  Country(country),
	}
	err := c.client.Query(ctx, &query, variables)
	if err != nil {
		return nil, err
	}
	return &query, nil
}

// GetTitleByID looks a title up by its JustWatch node ID (e.g. "tm1504418").
func (c *JustWatchClient) GetTitleByID(ctx context.Context, id string, country string, language string) (*MovieOrShowFragment, error) {
	var query GetTitleByIDQuery
	variables := map[string]interface{}{
		"id":       graphql.ID(id),
		"country":  Country(country),
		"language": Language(language),
	}
	if err := c.client.Query(ctx, &query, variables); err != nil {
		return nil, err
	}
	if query.Node.Id == "" {
		return nil, nil
	}
	return &query.Node.MovieOrShowFragment, nil
}
