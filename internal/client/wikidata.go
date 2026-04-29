package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-resty/resty/v2"
)

type WikidataTitleKind string

const (
	WikidataTitleKindUnknown WikidataTitleKind = "unknown"
	WikidataTitleKindMovie   WikidataTitleKind = "movie"
	WikidataTitleKindTVShow  WikidataTitleKind = "tv_show"
)

type WikidataStatement struct {
	ID       string                `json:"id"`
	Property WikidataProperty      `json:"property"`
	Value    WikidataPropertyValue `json:"value"`
}

type WikidataProperty struct {
	ID       string `json:"id"`
	DataType string `json:"data_type"`
}

type WikidataPropertyValue struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type WikidataClient struct {
	http *resty.Client
}

func NewWikidata(httpClient *resty.Client) *WikidataClient {
	return &WikidataClient{http: httpClient}
}

func (c *WikidataClient) Enabled() bool { return c != nil && c.http != nil }

// GetRottenTomatoesID verifies that wikidataID refers to the given kind and returns its P1258 value.
// Returns an error if the entity kind doesn't match or the property is absent.
func (c *WikidataClient) GetRottenTomatoesID(ctx context.Context, wikidataID string, kind WikidataTitleKind) (string, error) {
	if kind != WikidataTitleKindMovie && kind != WikidataTitleKindTVShow {
		return "", fmt.Errorf("invalid kind %q: must be %q or %q", kind, WikidataTitleKindMovie, WikidataTitleKindTVShow)
	}

	if !c.Enabled() {
		return "", fmt.Errorf("wikidata client is not enabled")
	}

	var out map[string][]WikidataStatement

	resp, err := c.http.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"property": "P1258",
		}).
		SetResult(&out).
		Get(fmt.Sprintf("https://www.wikidata.org/w/rest.php/wikibase/v1/entities/items/%s/statements", wikidataID))
	if err != nil {
		return "", fmt.Errorf("wikidata get rotten tomatoes id: %w", err)
	}
	if resp.IsError() {
		return "", fmt.Errorf("wikidata get rotten tomatoes id status %d", resp.StatusCode())
	}

	val, ok := out["P1258"]
	if !ok {
		return "", fmt.Errorf("wikidata %s has no statements for Rotten Tomatoes ID (P1258)", wikidataID)
	} else {
		for _, statement := range val {
			if statement.Property.ID == "P1258" && statement.Property.DataType == "external-id" && statement.Value.Type == "value" {
				if kind == WikidataTitleKindMovie && strings.HasPrefix(statement.Value.Content, "m/") {
					return statement.Value.Content, nil
				} else if kind == WikidataTitleKindTVShow && strings.HasPrefix(statement.Value.Content, "tv/") {
					return statement.Value.Content, nil
				} else {
					return "", fmt.Errorf("wikidata %s has no Rotten Tomatoes ID (P1258)", wikidataID)
				}
			}
		}
		return "", fmt.Errorf("wikidata %s has no Rotten Tomatoes ID (P1258)", wikidataID)
	}
}
