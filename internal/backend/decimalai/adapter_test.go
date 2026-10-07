package decimalai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rigerc/sift/internal/backend"
	"github.com/rigerc/sift/internal/model"
)

func newTestServer(t *testing.T, status int, body string) (*httptest.Server, *http.Request) {
	t.Helper()
	seen := &http.Request{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = *r
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

func newAdapter(t *testing.T, url, auth string) *Adapter {
	t.Helper()
	built, err := New(backend.Config{Name: "decimalai", Type: "decimalai", URL: url, Auth: auth})
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := built.(*Adapter)
	if !ok {
		t.Fatalf("unexpected adapter type %T", built)
	}
	return adapter
}

func query(limit int) backend.Query {
	return backend.Query{
		Unresolved: []model.Observation{{Key: "pkg:npm:react", Value: "react"}},
		Context:    []model.MergedSignal{{Key: "typescript", Confidence: 0.9}},
		Limit:      limit,
	}
}

func TestSearchMapsGitHubSource(t *testing.T) {
	body := `{"items":[{"url_slug":"cool-skill","display_name":"Cool Skill","name":"cool","description":"desc","skill_badge":"Verified","github_url":"https://github.com/acme/cool-repo","effectiveness":{"skill_score":85}}],"total_hint":123,"total":200}`
	srv, _ := newTestServer(t, http.StatusOK, body)
	got, err := newAdapter(t, srv.URL, "").Search(context.Background(), query(10))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("results = %+v", got)
	}
	result := got[0]
	if result.Skill.Source != "acme/cool-repo" || result.Skill.Name != "cool-skill" {
		t.Fatalf("skill = %+v", result.Skill)
	}
	if result.URL != "https://github.com/acme/cool-repo" || result.Title != "desc" {
		t.Fatalf("display = %+v", result)
	}
	if result.Reason != "Cool Skill · Verified" || result.ExternalScore != 0.85 {
		t.Fatalf("score/provenance = %+v", result)
	}
}

func TestSearchFallsBackToDetailURL(t *testing.T) {
	body := `{"items":[{"url_slug":"cool-skill","display_name":"Cool Skill","description":"d"}]}`
	srv, _ := newTestServer(t, http.StatusOK, body)
	got, err := newAdapter(t, srv.URL, "").Search(context.Background(), query(10))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("results = %+v", got)
	}
	want := "https://app.decimal.ai/skills/cool-skill"
	if got[0].Skill.Source != want || got[0].URL != want {
		t.Fatalf("fallback = %+v", got[0])
	}
	if got[0].ExternalScore != 1 {
		t.Fatalf("rank fallback score = %v", got[0].ExternalScore)
	}
}

func TestSearchFallsBackToSlugName(t *testing.T) {
	body := `{"items":[{"display_name":"Cool Skill","repo":"acme/cool-repo"}]}`
	srv, _ := newTestServer(t, http.StatusOK, body)
	got, err := newAdapter(t, srv.URL, "").Search(context.Background(), query(10))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Skill.Name != "cool-skill" || got[0].Skill.Source != "acme/cool-repo" {
		t.Fatalf("results = %+v", got)
	}
}

func TestSearchBuildsQueryAndLimit(t *testing.T) {
	srv, seen := newTestServer(t, http.StatusOK, `{"items":[]}`)
	adapter := newAdapter(t, srv.URL, "")
	if _, err := adapter.Search(context.Background(), query(999)); err != nil {
		t.Fatal(err)
	}
	if got := seen.URL.Query().Get("q"); got != "react typescript" {
		t.Fatalf("q = %q", got)
	}
	if got := seen.URL.Query().Get("limit"); got != "50" {
		t.Fatalf("limit = %q", got)
	}
	if got := seen.URL.Query().Get("sort"); got != "recommended" {
		t.Fatalf("sort = %q", got)
	}
}

func TestSearchEmptyQuerySkipsRequest(t *testing.T) {
	srv, seen := newTestServer(t, http.StatusOK, `{"items":[]}`)
	got, err := newAdapter(t, srv.URL, "").Search(context.Background(), backend.Query{})
	if err != nil || len(got) != 0 {
		t.Fatalf("results = %+v, err = %v", got, err)
	}
	if seen.URL != nil {
		t.Fatal("request issued for empty query")
	}
}

func TestSearchSendsAuthOnlyWhenConfigured(t *testing.T) {
	srv, seen := newTestServer(t, http.StatusOK, `{"items":[]}`)
	if _, err := newAdapter(t, srv.URL, "token-123").Search(context.Background(), query(5)); err != nil {
		t.Fatal(err)
	}
	if got := seen.Header.Get("Authorization"); got != "Bearer token-123" {
		t.Fatalf("auth = %q", got)
	}
	if _, err := newAdapter(t, srv.URL, "").Search(context.Background(), query(5)); err != nil {
		t.Fatal(err)
	}
	if got := seen.Header.Get("Authorization"); got != "" {
		t.Fatalf("anonymous auth = %q", got)
	}
}

func TestSearchErrors(t *testing.T) {
	srv, _ := newTestServer(t, http.StatusServiceUnavailable, "")
	if _, err := newAdapter(t, srv.URL, "").Search(context.Background(), query(5)); err == nil {
		t.Fatal("expected non-2xx error")
	}
	srv, _ = newTestServer(t, http.StatusOK, `{not-json`)
	if _, err := newAdapter(t, srv.URL, "").Search(context.Background(), query(5)); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestSearchDropsUnusableEntries(t *testing.T) {
	body := `{"items":[
		{"url_slug":"good","display_name":"Good","github_url":"https://github.com/acme/repo"},
		{"url_slug":"","display_name":"","name":"","github_url":"https://github.com/acme/repo"},
		{"display_name":"No Source"},
		{"url_slug":"bad","display_name":"Bad","github_url":"https://github.com/acme/repo","skill_badge":"-flag"}
	]}`
	srv, _ := newTestServer(t, http.StatusOK, body)
	got, err := newAdapter(t, srv.URL, "").Search(context.Background(), query(10))
	if err != nil {
		t.Fatal(err)
	}
	// "No Source" is dropped because it has no usable source; "bad" survives
	// because only the badge (display text) is flag-shaped, not the name.
	if len(got) != 2 || got[0].Skill.Name != "good" || got[1].Skill.Name != "bad" {
		t.Fatalf("results = %+v", got)
	}
}

func TestProbe(t *testing.T) {
	srv, _ := newTestServer(t, http.StatusOK, `{"items":[],"total_hint":123}`)
	got, err := newAdapter(t, srv.URL, "").Probe(context.Background())
	if err != nil || got != "123 skills" {
		t.Fatalf("probe = %q, %v", got, err)
	}
	srv, _ = newTestServer(t, http.StatusOK, `{"items":[]}`)
	got, err = newAdapter(t, srv.URL, "").Probe(context.Background())
	if err != nil || got != "online" {
		t.Fatalf("probe without count = %q, %v", got, err)
	}
	srv, _ = newTestServer(t, http.StatusServiceUnavailable, "")
	if _, err := newAdapter(t, srv.URL, "").Probe(context.Background()); err == nil {
		t.Fatal("expected probe failure")
	}
}
