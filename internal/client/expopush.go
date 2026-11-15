package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ExpoPush sends notifications through Expo's push service
// (https://docs.expo.dev/push-notifications/sending-notifications/), which
// forwards them to APNs and FCM for the app's Expo push tokens.
//
// Push tokens and the access token are secrets: they never appear in errors
// or logs. Expo's error messages quote the push token, so only the error
// code (details.error, e.g. DeviceNotRegistered) is passed on.
type ExpoPush struct {
	http        *http.Client
	baseURL     string
	accessToken string
}

// ExpoPushMaxBatch is how many messages one send request may carry.
const ExpoPushMaxBatch = 100

// ExpoPushMaxReceipts is how many receipt IDs one request may ask for.
const ExpoPushMaxReceipts = 1000

func NewExpoPush(timeout time.Duration, baseURL, accessToken string) *ExpoPush {
	if baseURL == "" {
		baseURL = "https://exp.host"
	}
	return &ExpoPush{http: &http.Client{Timeout: timeout}, baseURL: strings.TrimRight(baseURL, "/"), accessToken: accessToken}
}

// PushMessage is one notification to one device.
type PushMessage struct {
	To        string         `json:"to"`
	Title     string         `json:"title,omitempty"`
	Body      string         `json:"body"`
	Data      map[string]any `json:"data,omitempty"`
	Sound     string         `json:"sound,omitempty"`
	ChannelID string         `json:"channelId,omitempty"`
}

// PushTicket is Expo's answer for one message: an ID to fetch the receipt
// with, or an error code.
type PushTicket struct {
	ID    string // set when OK
	OK    bool
	Error string // e.g. DeviceNotRegistered, MessageTooBig, MessageRateExceeded
}

// PushReceipt is the delivery result Expo got from APNs / FCM.
type PushReceipt struct {
	OK    bool
	Error string
}

type expoTicket struct {
	Status  string `json:"status"`
	ID      string `json:"id"`
	Details struct {
		Error string `json:"error"`
	} `json:"details"`
}

type expoErrors struct {
	Errors []struct {
		Code string `json:"code"`
	} `json:"errors"`
}

// Send sends up to ExpoPushMaxBatch messages and returns one ticket per
// message, in order.
func (c *ExpoPush) Send(ctx context.Context, msgs []PushMessage) ([]PushTicket, error) {
	if len(msgs) == 0 {
		return nil, nil
	}
	if len(msgs) > ExpoPushMaxBatch {
		return nil, fmt.Errorf("expo push: %d messages, at most %d per request", len(msgs), ExpoPushMaxBatch)
	}
	var out struct {
		Data []expoTicket `json:"data"`
		expoErrors
	}
	if err := c.post(ctx, "/--/api/v2/push/send", msgs, &out); err != nil {
		return nil, err
	}
	if len(out.Data) != len(msgs) {
		return nil, fmt.Errorf("expo push: %d tickets for %d messages%s", len(out.Data), len(msgs), out.codes())
	}
	tickets := make([]PushTicket, len(out.Data))
	for i, t := range out.Data {
		tickets[i] = PushTicket{ID: t.ID, OK: t.Status == "ok", Error: t.Details.Error}
		if !tickets[i].OK && tickets[i].Error == "" {
			tickets[i].Error = "unknown"
		}
	}
	return tickets, nil
}

// Receipts fetches the receipts of up to ExpoPushMaxReceipts tickets. IDs
// without a receipt yet are missing from the map.
func (c *ExpoPush) Receipts(ctx context.Context, ids []string) (map[string]PushReceipt, error) {
	if len(ids) == 0 {
		return map[string]PushReceipt{}, nil
	}
	if len(ids) > ExpoPushMaxReceipts {
		return nil, fmt.Errorf("expo push: %d receipt IDs, at most %d per request", len(ids), ExpoPushMaxReceipts)
	}
	var out struct {
		Data map[string]expoTicket `json:"data"`
		expoErrors
	}
	if err := c.post(ctx, "/--/api/v2/push/getReceipts", map[string]any{"ids": ids}, &out); err != nil {
		return nil, err
	}
	receipts := make(map[string]PushReceipt, len(out.Data))
	for id, r := range out.Data {
		receipts[id] = PushReceipt{OK: r.Status == "ok", Error: r.Details.Error}
	}
	return receipts, nil
}

func (e expoErrors) codes() string {
	if len(e.Errors) == 0 {
		return ""
	}
	codes := make([]string, 0, len(e.Errors))
	for _, x := range e.Errors {
		codes = append(codes, x.Code)
	}
	return " (" + strings.Join(codes, ", ") + ")"
}

func (c *ExpoPush) post(ctx context.Context, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("expo push: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("expo push: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var e expoErrors
		_ = json.Unmarshal(raw, &e)
		return fmt.Errorf("expo push: HTTP %d%s", resp.StatusCode, e.codes())
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("expo push: decode response: %w", err)
	}
	return nil
}
