package skillsmp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	built, err := New(backend.Config{Name: "skillsmp", Type: "skillsmp", URL: url, Auth: auth})
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

const okBody = `{"success":true,"data":{
	"skills":[{"name":"acme/cool-skill","description":"desc","author":"Acme","stars":42,"githubUrl":"https://github.com/acme/cool-repo/tree/main/skills/cool","skillUrl":"https://skillsmp.com/skills/cool"}],
	"pagination":{"page":1,"total":1},
	"usage":{"plan":"free","remaining":99}
}}`

func TestSearchMapsGitHubSource(t *testing.T) {
	srv, _ := newTestServer(t, http.StatusOK, okBody)
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
	if result.URL != "https://skillsmp.com/skills/cool" || result.Title != "desc" {
		t.Fatalf("display = %+v", result)
	}
	if result.Reason != "Acme · ★42" || result.SourceBackend != "skillsmp" {
		t.Fatalf("provenance = %+v", result)
	}
}

func TestSearchFallsBackToDetailURL(t *testing.T) {
	body := `{"success":true,"data":{"skills":[{"name":"cool-skill","description":"d","skillUrl":"https://skillsmp.com/skills/cool"}]}}`
	srv, _ := newTestServer(t, http.StatusOK, body)
	got, err := newAdapter(t, srv.URL, "").Search(context.Background(), query(10))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Skill.Source != "https://skillsmp.com/skills/cool" {
		t.Fatalf("results = %+v", got)
	}
}

func TestSearchBuildsQueryAndLimit(t *testing.T) {
	srv, seen := newTestServer(t, http.StatusOK, `{"success":true,"data":{"skills":[]}}`)
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
	if got := seen.URL.Query().Get("sortBy"); got != "relevance" {
		t.Fatalf("sortBy = %q", got)
	}
}

func TestSearchEmptyQuerySkipsRequest(t *testing.T) {
	srv, seen := newTestServer(t, http.StatusOK, `{"success":true,"data":{"skills":[]}}`)
	got, err := newAdapter(t, srv.URL, "").Search(context.Background(), backend.Query{})
	if err != nil || len(got) != 0 {
		t.Fatalf("results = %+v, err = %v", got, err)
	}
	if seen.URL != nil {
		t.Fatal("request issued for empty query")
	}
}

func TestSearchSendsAuthOnlyWhenConfigured(t *testing.T) {
	srv, seen := newTestServer(t, http.StatusOK, `{"success":true,"data":{"skills":[]}}`)
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
	srv, _ := newTestServer(t, http.StatusBadGateway, "")
	if _, err := newAdapter(t, srv.URL, "").Search(context.Background(), query(5)); err == nil {
		t.Fatal("expected non-2xx error")
	}
	srv, _ = newTestServer(t, http.StatusOK, `{"success":false,"error":{"code":"RATE_LIMITED","message":"slow down"}}`)
	_, err := newAdapter(t, srv.URL, "").Search(context.Background(), query(5))
	if err == nil || !strings.Contains(err.Error(), "RATE_LIMITED") || !strings.Contains(err.Error(), "slow down") {
		t.Fatalf("error = %v", err)
	}
	srv, _ = newTestServer(t, http.StatusOK, `{not-json`)
	if _, err := newAdapter(t, srv.URL, "").Search(context.Background(), query(5)); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestSearchDropsUnusableEntries(t *testing.T) {
	body := `{"success":true,"data":{"skills":[
		{"name":"acme/good","githubUrl":"https://github.com/acme/repo"},
		{"name":"","githubUrl":"https://github.com/acme/repo"},
		{"name":"acme/nosource"},
		{"name":"-flag","githubUrl":"https://github.com/acme/repo"}
	]}}`
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
	srv, _ := newTestServer(t, http.StatusOK, okBody)
	got, err := newAdapter(t, srv.URL, "").Probe(context.Background())
	if err != nil || got != "plan free, 99 remaining" {
		t.Fatalf("probe = %q, %v", got, err)
	}
	srv, _ = newTestServer(t, http.StatusOK, `{"success":true,"data":{"skills":[]}}`)
	got, err = newAdapter(t, srv.URL, "").Probe(context.Background())
	if err != nil || got != "online" {
		t.Fatalf("probe without usage = %q, %v", got, err)
	}
	srv, _ = newTestServer(t, http.StatusBadGateway, "")
	if _, err := newAdapter(t, srv.URL, "").Probe(context.Background()); err == nil {
		t.Fatal("expected probe failure")
	}
}
