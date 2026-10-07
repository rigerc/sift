// Package httpx provides a small bounded JSON-over-HTTP client shared by
// discovery adapters. It never logs or echoes credentials, and it redacts
// URL query parameters from errors.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultTimeout bounds a single request when the caller supplies no client.
	DefaultTimeout = 15 * time.Second
	// DefaultMaxBytes bounds a single response body.
	DefaultMaxBytes int64 = 8 << 20
)

// Client performs bounded GET requests against one provider base URL.
type Client struct {
	BaseURL  string
	Auth     string
	HTTP     *http.Client
	MaxBytes int64
}

// Get performs a bounded GET request and returns the raw response body.
func (c Client) Get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	endpoint, err := c.endpoint(path, params)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("httpx: GET %s: %w", c.safe(path), err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "sift")
	if c.Auth != "" {
		req.Header.Set("Authorization", "Bearer "+c.Auth)
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("httpx: GET %s: %w", c.safe(path), underlying(err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode > 299 {
		return nil, fmt.Errorf("httpx: GET %s: HTTP %d %s", c.safe(path), resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	max := c.MaxBytes
	if max <= 0 {
		max = DefaultMaxBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, fmt.Errorf("httpx: GET %s: read body: %w", c.safe(path), err)
	}
	if int64(len(body)) > max {
		return nil, fmt.Errorf("httpx: GET %s: response exceeds %d bytes", c.safe(path), max)
	}
	return body, nil
}

// GetJSON performs a bounded GET and decodes the response into out. Unknown
// fields are tolerated because registries evolve their payloads.
func (c Client) GetJSON(ctx context.Context, path string, params url.Values, out any) error {
	body, err := c.Get(ctx, path, params)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("httpx: GET %s: decode response: %w", c.safe(path), err)
	}
	return nil
}

func (c Client) endpoint(path string, params url.Values) (string, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return "", fmt.Errorf("httpx: empty base URL")
	}
	endpoint := strings.TrimSuffix(c.BaseURL, "/") + path
	if encoded := params.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	return endpoint, nil
}

// safe renders scheme://host/path for error messages, dropping the query.
func (c Client) safe(path string) string {
	base := strings.TrimSuffix(c.BaseURL, "/")
	u, err := url.Parse(base + path)
	if err != nil {
		return base + path
	}
	return u.Scheme + "://" + u.Host + u.Path
}

// underlying strips *url.Error wrappers so the raw request URL (and its query)
// never reaches the caller's error text.
func underlying(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err
	}
	return err
}
