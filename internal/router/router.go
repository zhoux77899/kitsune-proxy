// Package router resolves public model names to immutable upstream routes.
package router

import (
	"fmt"
	"net/url"
	"sort"

	"github.com/zhoux77899/kitsune-proxy/internal/config"
)

// Route contains everything the proxy needs after resolving a public model.
type Route struct {
	PublicModel   string
	UpstreamModel string
	UpstreamName  string
	BaseURL       *url.URL
	AuthMode      string
	APIKey        string
	SkipTLSVerify bool
}

// ModelInfo is the stable public model metadata exposed by /v1/models.
type ModelInfo struct {
	ID    string
	Owner string
}

// Table is an immutable exact-match routing table.
type Table struct {
	routes map[string]Route
	models []ModelInfo
}

// New builds an immutable routing table from a validated configuration.
func New(cfg config.Config) (*Table, error) {
	routes := make(map[string]Route)
	models := make([]ModelInfo, 0)

	for upstreamName, upstream := range cfg.Upstreams {
		baseURL, err := url.Parse(upstream.URL)
		if err != nil {
			return nil, fmt.Errorf("parse upstream %s base URL: %w", upstreamName, err)
		}

		apiKey := ""
		if upstream.Auth.APIKey != nil {
			apiKey = *upstream.Auth.APIKey
		}
		for _, model := range upstream.Models {
			publicName := model.PublicName()
			if _, exists := routes[publicName]; exists {
				return nil, fmt.Errorf("duplicate public model %q", publicName)
			}
			routes[publicName] = Route{
				PublicModel:   publicName,
				UpstreamModel: model.ID,
				UpstreamName:  upstreamName,
				BaseURL:       baseURL,
				AuthMode:      upstream.Auth.Mode,
				APIKey:        apiKey,
				SkipTLSVerify: upstream.TLS.SkipVerify,
			}
			models = append(models, ModelInfo{ID: publicName, Owner: upstreamName})
		}
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	return &Table{routes: routes, models: models}, nil
}

// Resolve performs a case-sensitive exact lookup.
func (t *Table) Resolve(publicModel string) (Route, bool) {
	route, ok := t.routes[publicModel]
	return route, ok
}

// Models returns a defensive copy of sorted public model metadata.
func (t *Table) Models() []ModelInfo {
	return append([]ModelInfo(nil), t.models...)
}

// Len returns the number of public model names.
func (t *Table) Len() int {
	return len(t.routes)
}
