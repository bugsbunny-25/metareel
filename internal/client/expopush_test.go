package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExpoPushSend(t *testing.T) {
	var auth string
	var got []PushMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/--/api/v2/push/send" {
			t.Errorf("path %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"data":[{"status":"ok","id":"T1"},
			{"status":"error","message":"\"ExponentPushToken[secret123]\" is not a registered push notification recipient","details":{"error":"DeviceNotRegistered"}}]}`))
	}))
	defer srv.Close()
	c := NewExpoPush(5*time.Second, srv.URL, "access-token")
	tickets, err := c.Send(context.Background(), []PushMessage{
		{To: "ExponentPushToken[a]", Title: "t", Body: "b", Data: map[string]any{"url": "/title/movie/1"}, ChannelID: "arrivals"},
		{To: "ExponentPushToken[secret123]", Body: "b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer access-token" {
		t.Errorf("authorization header %q", auth)
	}
	if len(got) != 2 || got[0].ChannelID != "arrivals" || got[0].Data["url"] != "/title/movie/1" {
		t.Errorf("request body %+v", got)
	}
	if !tickets[0].OK || tickets[0].ID != "T1" || tickets[1].OK || tickets[1].Error != "DeviceNotRegistered" {
		t.Errorf("tickets %+v", tickets)
	}
}

func TestExpoPushErrorsCarryNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"code":"PUSH_TOO_MANY_EXPERIENCE_IDS","message":"ExponentPushToken[secret123] belongs to another project"}]}`))
	}))
	defer srv.Close()
	_, err := NewExpoPush(5*time.Second, srv.URL, "").Send(context.Background(), []PushMessage{{To: "ExponentPushToken[secret123]", Body: "b"}})
	if err == nil || strings.Contains(err.Error(), "secret123") || !strings.Contains(err.Error(), "PUSH_TOO_MANY_EXPERIENCE_IDS") {
		t.Fatalf("error %v", err)
	}
}

func TestExpoPushReceipts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ IDs []string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path != "/--/api/v2/push/getReceipts" || len(body.IDs) != 2 {
			t.Errorf("request %s %+v", r.URL.Path, body)
		}
		_, _ = w.Write([]byte(`{"data":{"A":{"status":"ok"},"B":{"status":"error","details":{"error":"DeviceNotRegistered"}}}}`))
	}))
	defer srv.Close()
	r, err := NewExpoPush(5*time.Second, srv.URL, "").Receipts(context.Background(), []string{"A", "B"})
	if err != nil || !r["A"].OK || r["B"].OK || r["B"].Error != "DeviceNotRegistered" {
		t.Fatalf("receipts %+v, %v", r, err)
	}
	if _, err := NewExpoPush(time.Second, srv.URL, "").Send(context.Background(), make([]PushMessage, ExpoPushMaxBatch+1)); err == nil {
		t.Fatal("expected a batch size error")
	}
}
