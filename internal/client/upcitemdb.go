package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
)

// UPCItemDB looks barcodes up on UPCitemdb (https://www.upcitemdb.com).
// Without a key it uses the free trial endpoint (100 lookups a day, a few a
// minute, per IP); with UPCITEMDB_API_KEY the paid one.
type UPCItemDB struct {
	http   *resty.Client
	apiKey string
	base   string
}

func NewUPCItemDB(httpClient *resty.Client, apiKey string) *UPCItemDB {
	return &UPCItemDB{http: httpClient, apiKey: apiKey, base: "https://api.upcitemdb.com/prod"}
}

// WithBaseURL points the client at another server (tests).
func (c *UPCItemDB) WithBaseURL(u string) *UPCItemDB { c.base = u; return c }

// UPCItem is one product UPCitemdb knows for a barcode.
type UPCItem struct {
	EAN         string   `json:"ean"`
	UPC         string   `json:"upc"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Brand       string   `json:"brand"`
	Model       string   `json:"model"`
	Category    string   `json:"category"`
	Images      []string `json:"images"`
}

// ErrBarcodeRateLimited means the lookup quota is spent for now.
var ErrBarcodeRateLimited = errors.New("barcode lookup rate limited")

// Lookup returns the products for a barcode; none is not an error.
func (c *UPCItemDB) Lookup(ctx context.Context, barcode string) ([]UPCItem, error) {
	var body struct {
		Code    string    `json:"code"`
		Message string    `json:"message"`
		Items   []UPCItem `json:"items"`
	}
	req := c.http.R().SetContext(ctx).SetQueryParam("upc", barcode).SetResult(&body).SetError(&body)
	path := "/trial/lookup"
	if c.apiKey != "" {
		path = "/v1/lookup"
		req.SetHeader("user_key", c.apiKey).SetHeader("key_type", "3scale")
	}
	resp, err := req.Get(c.base + path)
	if err != nil {
		// The key travels in a header, so the error's URL doesn't carry it.
		return nil, redactErr(fmt.Errorf("upcitemdb lookup: %w", err))
	}
	switch resp.StatusCode() {
	case http.StatusOK:
		return body.Items, nil
	case http.StatusNotFound:
		return nil, nil
	case http.StatusTooManyRequests:
		return nil, ErrBarcodeRateLimited
	case http.StatusBadRequest:
		if body.Code == "INVALID_UPC" {
			return nil, nil
		}
	}
	return nil, fmt.Errorf("upcitemdb lookup: status %d %s", resp.StatusCode(), body.Code)
}
