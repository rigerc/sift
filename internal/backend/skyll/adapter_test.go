package skyll

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-s/internal/backend"
	"go-s/internal/model"
)

// newTestServer serves the Skyll search and health endpoints and records the
// most recent search request.
func newTestServer(t *testing.T, status int, body string) (*httptest.Server, *http.Request) {
	t.Helper()
	seen := &http.Request{}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		*seen = *r
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, seen
}

func newAdapter(t *testing.T, url, auth string) *Adapter {
	t.Helper()
	built, err := New(backend.Config{Name: "skyll", Type: "skyll", URL: url, Auth: auth})
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
		Context:    []model.MergedSignal{{Key: "typescript", Confidence: 0.9}, {Key: "node", Confidence: 0.5}},
		Limit:      limit,
	}
}

func TestSearchMapsGitHubSource(t *testing.T) {
	body := `{"query":"react","count":1,"skills":[{"id":"acme/cool-skill","title":"Cool Skill","description":"desc","source":"acme/cool-repo","relevance_score":0.9,"refs":{"github":"https://github.com/acme/cool-repo","skills_sh":"https://skyll.app/skills/cool","raw":"https://raw.example/x"}}]}`
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
	if result.URL != "https://skyll.app/skills/cool" || result.Title != "Cool Skill" {
		t.Fatalf("display = %+v", result)
	}
	if result.ExternalScore != 0.9 || result.SourceBackend != "skyll" || result.Reason != "acme/cool-repo" {
		t.Fatalf("score/provenance = %+v", result)
	}
}

func TestSearchFallsBackToDetailURL(t *testing.T) {
	body := `{"skills":[{"id":"cool-skill","title":"Cool","refs":{"skills_sh":"https://skyll.app/skills/cool"}}]}`
	srv, _ := newTestServer(t, http.StatusOK, body)
	got, err := newAdapter(t, srv.URL, "").Search(context.Background(), query(10))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Skill.Source != "https://skyll.app/skills/cool" || got[0].Skill.Name != "cool-skill" {
		t.Fatalf("results = %+v", got)
	}
	if got[0].ExternalScore != 1 {
		t.Fatalf("rank fallback score = %v", got[0].ExternalScore)
	}
}

func TestSearchBuildsQueryAndLimit(t *testing.T) {
	srv, seen := newTestServer(t, http.StatusOK, `{"skills":[]}`)
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
	if _, err := adapter.Search(context.Background(), query(0)); err != nil {
		t.Fatal(err)
	}
	if got := seen.URL.Query().Get("limit"); got != "10" {
		t.Fatalf("default limit = %q", got)
	}
}

func TestSearchEmptyQuerySkipsRequest(t *testing.T) {
	srv, seen := newTestServer(t, http.StatusOK, `{"skills":[]}`)
	got, err := newAdapter(t, srv.URL, "").Search(context.Background(), backend.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("results = %+v", got)
	}
	if seen.URL != nil {
		t.Fatal("request issued for empty query")
	}
}

func TestSearchSendsAuthOnlyWhenConfigured(t *testing.T) {
	srv, seen := newTestServer(t, http.StatusOK, `{"skills":[]}`)
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
	srv, _ := newTestServer(t, http.StatusInternalServerError, "")
	if _, err := newAdapter(t, srv.URL, "").Search(context.Background(), query(5)); err == nil {
		t.Fatal("expected non-2xx error")
	}
	srv, _ = newTestServer(t, http.StatusOK, `{not-json`)
	if _, err := newAdapter(t, srv.URL, "").Search(context.Background(), query(5)); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestSearchDropsUnusableEntries(t *testing.T) {
	body := `{"skills":[
		{"id":"acme/good","source":"acme/repo"},
		{"id":"","title":"","source":"acme/repo"},
		{"id":"acme/nosource","source":""},
		{"id":"-flag","source":"acme/repo"}
	]}`
	srv, _ := newTestServer(t, http.StatusOK, body)
	got, err := newAdapter(t, srv.URL, "").Search(context.Background(), query(10))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Skill.Name != "good" {
		t.Fatalf("results = %+v", got)
	}
}

func TestProbe(t *testing.T) {
	srv, _ := newTestServer(t, http.StatusOK, `{}`)
	got, err := newAdapter(t, srv.URL, "").Probe(context.Background())
	if err != nil || got != "healthy" {
		t.Fatalf("probe = %q, %v", got, err)
	}
	srv, _ = newTestServer(t, http.StatusBadGateway, "")
	if _, err := newAdapter(t, srv.URL, "").Probe(context.Background()); err == nil {
		t.Fatal("expected probe failure")
	}
}
