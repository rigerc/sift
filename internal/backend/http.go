package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxResponseBytes int64 = 2 << 20

type HTTPClient struct {
	Client  *http.Client
	BaseURL string
	Auth    string
}

func NewHTTPClient(cfg Config) (*HTTPClient, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("missing backend URL")
	}
	parsed, err := url.Parse(cfg.URL)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid backend URL")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	auth := cfg.Auth
	if strings.HasPrefix(auth, "env:") {
		var ok bool
		auth, ok = os.LookupEnv(strings.TrimPrefix(auth, "env:"))
		if !ok {
			return nil, fmt.Errorf("auth environment variable is not set")
		}
	}
	return &HTTPClient{Client: &http.Client{Timeout: timeout}, BaseURL: strings.TrimRight(cfg.URL, "/"), Auth: auth}, nil
}

func (c *HTTPClient) DoJSON(ctx context.Context, method, path string, query url.Values, body any, dst any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(data))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	if query != nil {
		req.URL.RawQuery = query.Encode()
	}
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}
	if c.Auth != "" {
		req.Header.Set("authorization", "Bearer "+c.Auth)
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("backend request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	limited := io.LimitReader(resp.Body, maxResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read backend response: %w", err)
	}
	if int64(len(data)) > maxResponseBytes {
		return fmt.Errorf("backend response exceeds %d bytes", maxResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("backend returned HTTP %d", resp.StatusCode)
	}
	if dst == nil {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode backend response: %w", err)
	}
	return nil
}
