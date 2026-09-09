package catalog

import (
	"context"
	"fmt"
	"go-s/internal/backend"
	"go-s/internal/model"
	"go-s/internal/rules"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

type Adapter struct {
	name, url     string
	timeoutClient *http.Client
}

func New(cfg backend.Config) (backend.Backend, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("catalog URL is required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Adapter{name: cfg.Name, url: cfg.URL, timeoutClient: &http.Client{Timeout: timeout}}, nil
}
func (a *Adapter) Name() string { return a.name }
func (a *Adapter) Catalog(ctx context.Context) (rules.Catalog, error) {
	parsed, err := url.Parse(a.url)
	if err != nil {
		return rules.Catalog{}, err
	}
	var data []byte
	switch parsed.Scheme {
	case "file":
		data, err = os.ReadFile(parsed.Path)
	case "":
		data, err = os.ReadFile(a.url)
	case "http", "https":
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, a.url, nil)
		if reqErr != nil {
			return rules.Catalog{}, reqErr
		}
		resp, reqErr := a.timeoutClient.Do(req)
		if reqErr != nil {
			return rules.Catalog{}, reqErr
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return rules.Catalog{}, fmt.Errorf("catalog returned HTTP %d", resp.StatusCode)
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, 2<<20+1))
		if int64(len(data)) > 2<<20 {
			return rules.Catalog{}, fmt.Errorf("catalog exceeds response limit")
		}
	default:
		return rules.Catalog{}, fmt.Errorf("unsupported catalog URL scheme %q", parsed.Scheme)
	}
	if err != nil {
		return rules.Catalog{}, err
	}
	return rules.Decode(data)
}

func (a *Adapter) Validate(ctx context.Context, skills []model.SkillRef) (map[string]backend.Validation, error) {
	catalog, err := a.Catalog(ctx)
	if err != nil {
		return nil, err
	}
	known := map[string]struct{}{}
	for _, rule := range catalog.SkillRules {
		for _, skill := range rule.Skills {
			known[skill.Key()] = struct{}{}
		}
	}
	for _, combo := range catalog.ComboRules {
		for _, skill := range combo.Skills {
			known[skill.Key()] = struct{}{}
		}
		for _, skill := range combo.Order {
			known[skill.Key()] = struct{}{}
		}
	}
	out := make(map[string]backend.Validation, len(skills))
	for _, skill := range skills {
		status := backend.StatusInvalid
		detail := "skill is absent from catalog"
		if _, ok := known[skill.Key()]; ok {
			status = backend.StatusValid
			detail = "skill is present in catalog"
		}
		out[skill.Key()] = backend.Validation{Status: status, Detail: detail}
	}
	return out, nil
}
