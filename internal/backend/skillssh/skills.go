package skillssh

import (
	"context"
	"fmt"
	"go-s/internal/backend"
	"go-s/internal/model"
	"net/http"
	"net/url"
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

type searchResponse struct {
	Results []struct {
		Source string  `json:"source"`
		Name   string  `json:"name"`
		Title  string  `json:"title"`
		Score  float64 `json:"score"`
		Reason string  `json:"reason"`
	} `json:"results"`
}

func (a *Adapter) Search(ctx context.Context, q backend.Query) ([]backend.ExternalSuggestion, error) {
	values := url.Values{}
	for _, o := range q.Unresolved {
		values.Add("observation", o.Value)
	}
	for _, s := range q.Context {
		values.Add("technology", s.Key)
	}
	if q.Limit > 0 {
		values.Set("limit", fmt.Sprint(q.Limit))
	}
	var response searchResponse
	if err := a.client.DoJSON(ctx, http.MethodGet, "/v1/search", values, nil, &response); err != nil {
		return nil, err
	}
	out := make([]backend.ExternalSuggestion, 0, len(response.Results))
	for _, v := range response.Results {
		out = append(out, backend.ExternalSuggestion{Skill: model.SkillRef{Source: v.Source, Name: v.Name}, Title: v.Title, ExternalScore: v.Score, Reason: v.Reason, SourceBackend: a.name})
	}
	return out, nil
}

type validateRequest struct {
	Skills []model.SkillRef `json:"skills"`
}
type validateResponse struct {
	Results map[string]struct {
		Status    string `json:"status"`
		Revision  string `json:"revision"`
		Downloads int    `json:"downloads"`
		Detail    string `json:"detail"`
	} `json:"results"`
}

func (a *Adapter) Validate(ctx context.Context, skills []model.SkillRef) (map[string]backend.Validation, error) {
	var response validateResponse
	if err := a.client.DoJSON(ctx, http.MethodPost, "/v1/validate", nil, validateRequest{skills}, &response); err != nil {
		return nil, err
	}
	out := map[string]backend.Validation{}
	for _, skill := range skills {
		raw, ok := response.Results[skill.Key()]
		if !ok {
			continue
		}
		status := backend.StatusUnknown
		switch raw.Status {
		case "valid":
			status = backend.StatusValid
		case "invalid":
			status = backend.StatusInvalid
		}
		out[skill.Key()] = backend.Validation{Status: status, Revision: raw.Revision, Downloads: raw.Downloads, Detail: raw.Detail}
	}
	return out, nil
}
