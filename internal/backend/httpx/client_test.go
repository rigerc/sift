package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGetJSONSendsAuthOnlyWhenConfigured(t *testing.T) {
	var gotAuth, gotAgent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAgent = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	var out map[string]bool
	client := Client{BaseURL: srv.URL, Auth: "secret-token"}
	if err := client.GetJSON(context.Background(), "/search", nil, &out); err != nil {
		t.Fatal(err)
	}
	if gotAgent != "sift" {
		t.Errorf("authenticated User-Agent = %q, want sift", gotAgent)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("auth header = %q", gotAuth)
	}

	anonymous := Client{BaseURL: srv.URL}
	if err := anonymous.GetJSON(context.Background(), "/search", nil, &out); err != nil {
		t.Fatal(err)
	}
	if gotAgent != "sift" {
		t.Errorf("anonymous User-Agent = %q, want sift", gotAgent)
	}
	if gotAuth != "" {
		t.Fatalf("anonymous request sent auth header %q", gotAuth)
	}
}

func TestGetJSONBuildsQueryString(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := Client{BaseURL: srv.URL + "/"}
	var out map[string]any
	if err := client.GetJSON(context.Background(), "/api/v1/skills/search", url.Values{"q": {"react"}, "limit": {"10"}}, &out); err != nil {
		t.Fatal(err)
	}
	if gotQuery.Get("q") != "react" || gotQuery.Get("limit") != "10" {
		t.Fatalf("query = %v", gotQuery)
	}
}

func TestGetJSONErrors(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name:    "non-2xx",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) },
			want:    "HTTP 503",
		},
		{
			name:    "malformed json",
			handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{not-json`)) },
			want:    "decode response",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			client := Client{BaseURL: srv.URL}
			var out map[string]any
			err := client.GetJSON(context.Background(), "/search", nil, &out)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want contains %q", err, tc.want)
			}
		})
	}
}

func TestGetJSONRejectsOversizedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"value":"` + strings.Repeat("x", 128) + `"}`))
	}))
	defer srv.Close()

	client := Client{BaseURL: srv.URL, MaxBytes: 32}
	var out map[string]any
	err := client.GetJSON(context.Background(), "/search", nil, &out)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want size-limit error", err)
	}
}

func TestGetJSONRedactsQueryFromErrors(t *testing.T) {
	client := Client{BaseURL: "http://127.0.0.1:0"}
	var out map[string]any
	err := client.GetJSON(context.Background(), "/search", url.Values{"token": {"super-secret"}}, &out)
	if err == nil {
		t.Fatal("expected connection error")
	}
	if strings.Contains(err.Error(), "super-secret") || strings.Contains(err.Error(), "token=") {
		t.Fatalf("query leaked into error: %v", err)
	}
	if !strings.Contains(err.Error(), "http://127.0.0.1:0/search") {
		t.Fatalf("endpoint missing from error: %v", err)
	}
}

func TestGetJSONToleratesUnknownFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"known":1,"unexpected":{"nested":true}}`))
	}))
	defer srv.Close()

	var out struct {
		Known int `json:"known"`
	}
	client := Client{BaseURL: srv.URL}
	if err := client.GetJSON(context.Background(), "/search", nil, &out); err != nil {
		t.Fatal(err)
	}
	if out.Known != 1 {
		t.Fatalf("out = %+v", out)
	}
}
