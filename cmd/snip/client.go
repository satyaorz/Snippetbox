package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// client talks to the Snippetbox JSON API.
type client struct {
	baseURL string
	token   string
	http    *http.Client
}

// newClient creates an API client. If caCertFile is set, that certificate is
// trusted in addition to the system roots, which is how you use the
// self-signed development certificate in ./tls.
func newClient(baseURL, token, caCertFile string) (*client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid server URL %q", baseURL)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()

	if caCertFile != "" {
		pem, err := os.ReadFile(caCertFile)
		if err != nil {
			return nil, fmt.Errorf("reading CA certificate: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificates found in %s", caCertFile)
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool}
	}

	return &client{
		baseURL: strings.TrimRight(u.String(), "/"),
		token:   token,
		http:    &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}, nil
}

// apiError is an error response from the server.
type apiError struct {
	Status  int
	Message string            `json:"error"`
	Fields  map[string]string `json:"fields"`
}

func (e *apiError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	if len(e.Fields) > 0 {
		names := make([]string, 0, len(e.Fields))
		for name := range e.Fields {
			names = append(names, name)
		}
		sort.Strings(names)

		var parts []string
		for _, name := range names {
			parts = append(parts, name+": "+e.Fields[name])
		}
		msg += " (" + strings.Join(parts, "; ") + ")"
	}
	return msg
}

// do sends a request with an optional JSON body and decodes a JSON response
// into out (if out isn't nil).
func (c *client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		js, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(js)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		apiErr := &apiError{Status: res.StatusCode}
		// Best effort: errors from a proxy might not be JSON.
		json.NewDecoder(res.Body).Decode(apiErr)
		return apiErr
	}

	if out == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// snippet mirrors the API's JSON representation.
type snippet struct {
	Slug       string    `json:"slug"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	Language   string    `json:"language"`
	Visibility string    `json:"visibility"`
	Author     string    `json:"author"`
	Created    time.Time `json:"created"`
	Expires    time.Time `json:"expires"`
	URL        string    `json:"url"`
	RawURL     string    `json:"raw_url"`
}

type newSnippet struct {
	Title         string `json:"title"`
	Content       string `json:"content"`
	Language      string `json:"language"`
	Visibility    string `json:"visibility"`
	ExpiresInDays int    `json:"expires_in_days"`
}

func (c *client) createToken(ctx context.Context, email, password, name string) (string, time.Time, error) {
	var out struct {
		Token   string    `json:"token"`
		Expires time.Time `json:"expires"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/tokens", map[string]string{"email": email, "password": password, "name": name}, &out)
	return out.Token, out.Expires, err
}

func (c *client) revokeToken(ctx context.Context) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/tokens/current", nil, nil)
}

func (c *client) me(ctx context.Context) (name, email string, err error) {
	var out struct {
		User struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"user"`
	}
	err = c.do(ctx, http.MethodGet, "/api/v1/me", nil, &out)
	return out.User.Name, out.User.Email, err
}

func (c *client) createSnippet(ctx context.Context, s newSnippet) (snippet, error) {
	var out struct {
		Snippet snippet `json:"snippet"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/snippets", s, &out)
	return out.Snippet, err
}

func (c *client) getSnippet(ctx context.Context, slug string) (snippet, error) {
	var out struct {
		Snippet snippet `json:"snippet"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/snippets/"+url.PathEscape(slug), nil, &out)
	return out.Snippet, err
}

func (c *client) mySnippets(ctx context.Context) ([]snippet, error) {
	var out struct {
		Snippets []snippet `json:"snippets"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/me/snippets", nil, &out)
	return out.Snippets, err
}

func (c *client) searchSnippets(ctx context.Context, query string) ([]snippet, int, error) {
	var out struct {
		Snippets []snippet `json:"snippets"`
		Metadata struct {
			Total int `json:"total"`
		} `json:"metadata"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/snippets?page_size=50&q="+url.QueryEscape(query), nil, &out)
	return out.Snippets, out.Metadata.Total, err
}

func (c *client) deleteSnippet(ctx context.Context, slug string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/snippets/"+url.PathEscape(slug), nil, nil)
}

// isUnauthorized reports whether err is a 401 from the server.
func isUnauthorized(err error) bool {
	var apiErr *apiError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized
}
