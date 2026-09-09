package githubtrees

import (
	"context"
	"go-s/internal/backend"
	"go-s/internal/model"
	"net/http"
	"net/url"
	"path"
	"strings"
)

type Adapter struct {
	name   string
	client *backend.HTTPClient
}

func New(cfg backend.Config) (backend.Backend, error) {
	if cfg.URL == "" {
		cfg.URL = "https://api.github.com"
	}
	c, err := backend.NewHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Adapter{name: cfg.Name, client: c}, nil
}
func (a *Adapter) Name() string { return a.name }

type treeResponse struct {
	SHA  string `json:"sha"`
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	} `json:"tree"`
}

func (a *Adapter) Validate(ctx context.Context, skills []model.SkillRef) (map[string]backend.Validation, error) {
	out := map[string]backend.Validation{}
	for _, skill := range skills {
		owner, repo, ok := sourceParts(skill.Source)
		if !ok {
			out[skill.Key()] = backend.Validation{Status: backend.StatusUnknown, Detail: "not a GitHub source"}
			continue
		}
		var tree treeResponse
		if err := a.client.DoJSON(ctx, http.MethodGet, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo)+"/git/trees/HEAD", nil, nil, &tree); err != nil {
			return nil, err
		}
		found := false
		for _, item := range tree.Tree {
			clean := path.Clean(item.Path)
			if item.Type == "tree" && (clean == skill.Name || clean == path.Join("skills", skill.Name) || clean == path.Join(".agents", skill.Name)) {
				found = true
				break
			}
		}
		status := backend.StatusInvalid
		if found {
			status = backend.StatusValid
		}
		out[skill.Key()] = backend.Validation{Status: status, Revision: tree.SHA}
	}
	return out, nil
}

func sourceParts(source string) (string, string, bool) {
	source = strings.TrimSuffix(source, "/")
	source = strings.TrimPrefix(source, "https://github.com/")
	source = strings.TrimPrefix(source, "github.com/")
	parts := strings.Split(source, "/")
	return func() (string, string, bool) {
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return "", "", false
		}
		return parts[0], parts[1], true
	}()
}
