package semantic

import (
	"context"
	"go-s/internal/backend"
	"go-s/internal/model"
	"net/http"
)

type Adapter struct {
	name   string
	client *backend.HTTPClient
}

func New(cfg backend.Config) (backend.Backend, error) {
	c, err := backend.NewHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Adapter{name: cfg.Name, client: c}, nil
}
func (a *Adapter) Name() string { return a.name }

type request struct {
	Query backend.Query `json:"query"`
}
type response struct {
	Results []struct {
		Source string  `json:"source"`
		Name   string  `json:"name"`
		Title  string  `json:"title"`
		Score  float64 `json:"score"`
		Reason string  `json:"reason"`
	} `json:"results"`
}

func (a *Adapter) Search(ctx context.Context, q backend.Query) ([]backend.ExternalSuggestion, error) {
	var response response
	if err := a.client.DoJSON(ctx, http.MethodPost, "/search", nil, request{q}, &response); err != nil {
		return nil, err
	}
	out := make([]backend.ExternalSuggestion, 0, len(response.Results))
	for _, v := range response.Results {
		out = append(out, backend.ExternalSuggestion{Skill: model.SkillRef{Source: v.Source, Name: v.Name}, Title: v.Title, ExternalScore: v.Score, Reason: v.Reason, SourceBackend: a.name})
	}
	return out, nil
}
