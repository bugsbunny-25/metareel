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

const (
	PackageAmazonPrime   Package = "amp"
	PackageAppleTVPlus   Package = "apt"
	PackageDisneyPlus    Package = "dnp"
	PackageHBOMax        Package = "mxx"
	PackageNetflix       Package = "nfx"
	PackageParamountPlus Package = "ppp"
	PackagePeacock       Package = "pct"
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

type Scoring struct {
	ImdbScore      *float64
	ImdbVotes      *int
	TmdbScore      *float64
	TmdbPopularity *float64
	JwRating       *float64
	TomatoMeter    *int
	CertifiedFresh *bool
}

type MovieOrShowFragment struct {
	Id      string
	Content struct {
		Title       string
		ExternalIds ExternalIds
		Scoring     Scoring
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

func (c *JustWatchClient) GetTitlesByTopSearchPopular(ctx context.Context, slug string, country string, language string, objectType ObjectType, pack Package) (*GetTitlesByTopSearchPopularQuery, error) {
	var query GetTitlesByTopSearchPopularQuery
	variables := map[string]interface{}{
		"popularTitlesSortBy": PopularTitlesSorting("TRENDING"),
		"first":               graphql.Int(1),
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
			Packages:                   []Package{pack},
			ExcludeIrrelevantTitles:    false,
			PresentationTypes:          []string{},
			MonetizationTypes:          []string{},
			SearchQuery:                slug,
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
