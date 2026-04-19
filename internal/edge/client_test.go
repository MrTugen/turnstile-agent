package edge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(url string) *Client {
	return New(Options{
		URL:            url,
		APIKey:         "test-key",
		DeviceName:     "turnstile-test-01",
		RequestTimeout: 2 * time.Second,
	})
}

func TestVerifyScanGrantedViaGrantedField(t *testing.T) {
	var gotUID, gotDevice, gotAPIKey, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("X-API-Key")
		gotContentType = r.Header.Get("Content-Type")

		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		gotUID, _ = req["uid"].(string)
		gotDevice, _ = req["deviceName"].(string)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"granted":true,"reason":"member_ok"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	d, err := c.VerifyScan(context.Background(), " aa:bb:cc ")
	if err != nil {
		t.Fatalf("VerifyScan: %v", err)
	}
	if !d.Granted {
		t.Error("Granted = false, want true")
	}
	if d.Reason != "member_ok" {
		t.Errorf("Reason = %q, want member_ok", d.Reason)
	}
	if gotUID != "AABBCC" {
		t.Errorf("uid sent = %q, want AABBCC", gotUID)
	}
	if gotDevice != "turnstile-test-01" {
		t.Errorf("deviceName sent = %q", gotDevice)
	}
	if gotAPIKey != "test-key" {
		t.Errorf("X-API-Key = %q", gotAPIKey)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q", gotContentType)
	}
}

func TestVerifyScanAcceptsLegacyFieldNames(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"access", `{"access":true}`, true},
		{"allowed", `{"allowed":true}`, true},
		{"granted_false", `{"granted":false,"reason":"not_member"}`, false},
		{"none_set", `{"reason":"no_such_user"}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()
			d, err := newTestClient(srv.URL).VerifyScan(context.Background(), "AA")
			if err != nil {
				t.Fatalf("VerifyScan: %v", err)
			}
			if d.Granted != c.want {
				t.Errorf("Granted = %v, want %v", d.Granted, c.want)
			}
		})
	}
}

func TestVerifyScanOmitsAPIKeyWhenEmpty(t *testing.T) {
	var gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("X-API-Key")
		_, _ = w.Write([]byte(`{"granted":false}`))
	}))
	defer srv.Close()

	c := New(Options{URL: srv.URL, DeviceName: "d", RequestTimeout: time.Second})
	if _, err := c.VerifyScan(context.Background(), "AA"); err != nil {
		t.Fatal(err)
	}
	if gotAPIKey != "" {
		t.Errorf("X-API-Key should be absent, got %q", gotAPIKey)
	}
}

func TestVerifyScanNon2xxErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).VerifyScan(context.Background(), "AA")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestVerifyScanTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(500 * time.Millisecond):
		}
	}))
	defer srv.CloseClientConnections()
	defer srv.Close()

	c := New(Options{URL: srv.URL, DeviceName: "d", RequestTimeout: 50 * time.Millisecond})
	_, err := c.VerifyScan(context.Background(), "AA")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
