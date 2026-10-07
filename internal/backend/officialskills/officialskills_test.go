package officialskills

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rigerc/sift/internal/backend"
	"github.com/rigerc/sift/internal/model"
)

func registryJSON(skills ...string) string {
	body := `{"meta":{"fetched_at":"2026-09-07T00:00:00Z","source":"https://example.test/README.md","total_skills":` + fmt.Sprint(len(skills)) + `},"skills":[`
	for i, s := range skills {
		if i > 0 {
			body += ","
		}
		body += s
	}
	body += "]}"
	return body
}

func skillJSON(id, name, repo string) string {
	repoJSON := "null"
	url := "https://officialskills.sh/" + id
	if repo != "" {
		repoJSON = `"` + repo + `"`
		url = "https://github.com/" + repo
	}
	return `{"id":"` + id + `","name":"` + name + `","url":"` + url + `","description":"desc for ` + name + `","publisher":"Acme","section":"Core","repo":` + repoJSON + `}`
}

func newTestServer(t *testing.T, official, community string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/official.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(official))
	})
	mux.HandleFunc("/community.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(community))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRegistryParseAndSearch(t *testing.T) {
	out, err := ParseOutput([]byte(registryJSON(skillJSON("acme/react", "react", "acme/react-skills"), skillJSON("acme/orphan", "orphan", ""))))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Skills) != 2 {
		t.Fatalf("skills=%d", len(out.Skills))
	}
	installable := InstallableOnly(out.Skills)
	if len(installable) != 1 || installable[0].Repo() != "acme/react-skills" {
		t.Fatalf("installable=%+v", installable)
	}
	if hits := Search(installable, "react", 10); len(hits) != 1 || hits[0].Name != "react" {
		t.Fatalf("search hits=%+v", hits)
	}
	if hits := Search(installable, "nomatch-xyz", 10); len(hits) != 0 {
		t.Fatalf("unexpected hits=%+v", hits)
	}
}

func TestParseOutputRejectsMissingSkills(t *testing.T) {
	if _, err := ParseOutput([]byte(`{"meta":{}}`)); err == nil {
		t.Fatal("missing skills array accepted")
	}
}

func TestLoaderCachesAndServesStale(t *testing.T) {
	var requests int
	mux := http.NewServeMux()
	write := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			requests++
			_, _ = w.Write([]byte(body))
		}
	}
	mux.HandleFunc("/official.json", write(registryJSON(skillJSON("acme/react", "react", "acme/react-skills"))))
	mux.HandleFunc("/community.json", write(registryJSON()))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	loader := NewLoader(srv.URL, dir)
	loader.TTL = time.Hour

	first, err := loader.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Skills) != 1 || first.Offline {
		t.Fatalf("first load=%+v", first)
	}
	if requests != 2 {
		t.Fatalf("expected 2 fetches, got %d", requests)
	}

	// Fresh cache: no additional network requests.
	second, err := loader.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Skills) != 1 || requests != 2 {
		t.Fatalf("fresh cache not used: requests=%d", requests)
	}

	// Expire the cache and stop the server: stale fallback still serves.
	past := time.Now().Add(-2 * time.Hour)
	for _, name := range []string{"official.json", "community.json"} {
		if err := os.Chtimes(filepath.Join(dir, name), past, past); err != nil {
			t.Fatal(err)
		}
	}
	srv.Close()
	stale, err := loader.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(stale.Skills) != 1 || !stale.Offline {
		t.Fatalf("stale fallback=%+v", stale)
	}
}

func TestLoaderWithoutCacheFailsOnNetworkError(t *testing.T) {
	srv := newTestServer(t, registryJSON(), registryJSON())
	url := srv.URL
	srv.Close()
	loader := NewLoader(url, "")
	if _, err := loader.Load(context.Background()); err == nil {
		t.Fatal("expected network failure without cache")
	}
}

func TestAdapterSearchReturnsGitHubURL(t *testing.T) {
	official := registryJSON(
		skillJSON("acme/react", "react", "acme/react-skills"),
		skillJSON("acme/other", "other", "acme/other-skills"),
		skillJSON("acme/orphan", "orphan", ""),
	)
	srv := newTestServer(t, official, registryJSON())

	adapterRaw, err := New(backend.Config{Name: "official-skills", Type: "official-skills", URL: srv.URL, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	searcher, ok := adapterRaw.(backend.Searcher)
	if !ok {
		t.Fatal("adapter does not implement Searcher")
	}
	q := backend.Query{Unresolved: []model.Observation{{Key: "pkg:npm:react", Value: "react"}}, Limit: 10}
	got, err := searcher.Search(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("no results")
	}
	for _, s := range got {
		if s.Skill.Source == "" || s.Skill.Name == "" {
			t.Fatalf("skill identity incomplete: %+v", s)
		}
		if s.URL == "" || s.Skill.Source == "acme/orphan" {
			t.Fatalf("non-installable result leaked: %+v", s)
		}
	}
	if got[0].Skill.Source != "acme/react-skills" || got[0].URL != "https://github.com/acme/react-skills" {
		t.Fatalf("top result=%+v", got[0])
	}
	if got[0].SourceBackend != "official-skills" || got[0].Reason != "Acme · Core" {
		t.Fatalf("provenance=%+v", got[0])
	}
}

func TestAdapterProbe(t *testing.T) {
	srv := newTestServer(t, registryJSON(skillJSON("acme/react", "react", "acme/react-skills")), registryJSON())
	adapterRaw, err := New(backend.Config{Name: "official-skills", Type: "official-skills", URL: srv.URL, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	prober, ok := adapterRaw.(backend.VersionProber)
	if !ok {
		t.Fatal("adapter does not implement VersionProber")
	}
	got, err := prober.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("empty probe result")
	}
}
