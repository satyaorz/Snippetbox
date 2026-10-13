package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/satyaorz/snippetbox/internal/assert"
	"github.com/satyaorz/snippetbox/internal/models/mocks"

	"golang.org/x/time/rate"
)

// apiRequest sends a JSON request (body may be nil) with an optional bearer
// token and decodes the JSON response into a map.
func (ts *testServer) apiRequest(t *testing.T, method, path, token string, body any) (int, http.Header, map[string]any) {
	t.Helper()

	var buf bytes.Buffer
	if body != nil {
		if s, ok := body.(string); ok {
			buf.WriteString(s)
		} else if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}

	req, err := http.NewRequest(method, ts.URL+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	res := ts.do(t, req)

	var decoded map[string]any
	if res.body != "" {
		if err := json.Unmarshal([]byte(res.body), &decoded); err != nil {
			t.Fatalf("response is not JSON: %q", res.body)
		}
	}
	return res.status, res.headers, decoded
}

func TestAPITokens(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	tests := []struct {
		name      string
		body      any
		wantCode  int
		wantError string
	}{
		{"Valid", map[string]string{"email": "alice@example.com", "password": "pa$$word", "name": "laptop"}, http.StatusCreated, ""},
		{"Wrong password", map[string]string{"email": "alice@example.com", "password": "nope"}, http.StatusUnauthorized, "invalid email or password"},
		{"Missing fields", map[string]string{}, http.StatusUnprocessableEntity, "validation failed"},
		{"Bad JSON", `{"email": `, http.StatusBadRequest, "badly-formed JSON"},
		{"Unknown field", `{"email": "a", "password": "b", "admin": true}`, http.StatusBadRequest, `unknown field "admin"`},
		{"Wrong type", `{"email": 42, "password": "b"}`, http.StatusBadRequest, `wrong JSON type for field "email"`},
		{"Two values", `{"email": "a", "password": "b"} {}`, http.StatusBadRequest, "single JSON value"},
		{"Empty body", "", http.StatusBadRequest, "must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, body := ts.apiRequest(t, http.MethodPost, "/api/v1/tokens", "", tt.body)

			assert.Equal(t, status, tt.wantCode)
			if tt.wantError != "" {
				assert.StringContains(t, body["error"].(string), tt.wantError)
			} else {
				assert.Equal[any](t, body["token"], mocks.ValidToken)
			}
		})
	}
}

func TestAPIAuthentication(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	t.Run("No token", func(t *testing.T) {
		status, headers, body := ts.apiRequest(t, http.MethodGet, "/api/v1/me", "", nil)

		assert.Equal(t, status, http.StatusUnauthorized)
		assert.Equal(t, headers.Get("WWW-Authenticate"), "Bearer")
		assert.StringContains(t, body["error"].(string), "needs an API token")
	})

	t.Run("Invalid token", func(t *testing.T) {
		// Rejected even on endpoints that don't need authentication.
		status, headers, _ := ts.apiRequest(t, http.MethodGet, "/api/v1/snippets", "sb_wrong", nil)

		assert.Equal(t, status, http.StatusUnauthorized)
		assert.StringContains(t, headers.Get("WWW-Authenticate"), "invalid_token")
	})

	t.Run("Valid token", func(t *testing.T) {
		status, _, body := ts.apiRequest(t, http.MethodGet, "/api/v1/me", mocks.ValidToken, nil)

		assert.Equal(t, status, http.StatusOK)
		assert.Equal[any](t, body["user"].(map[string]any)["email"], "alice@example.com")
	})

	t.Run("Session cookies don't authenticate the API", func(t *testing.T) {
		browser := ts.newSession(t)
		browser.login(t)

		status, _, _ := browser.apiRequest(t, http.MethodGet, "/api/v1/me", "", nil)
		assert.Equal(t, status, http.StatusUnauthorized)
	})

	t.Run("Revoking the current token", func(t *testing.T) {
		status, _, _ := ts.apiRequest(t, http.MethodDelete, "/api/v1/tokens/current", mocks.ValidToken, nil)
		assert.Equal(t, status, http.StatusNoContent)

		status, _, _ = ts.apiRequest(t, http.MethodGet, "/api/v1/me", mocks.ValidToken, nil)
		assert.Equal(t, status, http.StatusUnauthorized)
	})
}

func TestAPIReadSnippets(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	t.Run("List", func(t *testing.T) {
		status, _, body := ts.apiRequest(t, http.MethodGet, "/api/v1/snippets?q=pond", "", nil)

		assert.Equal(t, status, http.StatusOK)
		snippets := body["snippets"].([]any)
		assert.Equal(t, len(snippets), 1)
		first := snippets[0].(map[string]any)
		assert.Equal[any](t, first["slug"], "a1b2c3d4e5f6")
		assert.Equal[any](t, first["url"], ts.URL+"/snippet/view/a1b2c3d4e5f6")
		// Lists leave the content out.
		assert.Equal(t, first["content"], nil)
		assert.Equal[any](t, body["metadata"].(map[string]any)["total"], float64(1))
	})

	t.Run("Invalid paging", func(t *testing.T) {
		status, _, body := ts.apiRequest(t, http.MethodGet, "/api/v1/snippets?page=0&page_size=1000", "", nil)

		assert.Equal(t, status, http.StatusUnprocessableEntity)
		fields := body["fields"].(map[string]any)
		assert.Equal(t, len(fields), 2)
	})

	t.Run("Show", func(t *testing.T) {
		status, _, body := ts.apiRequest(t, http.MethodGet, "/api/v1/snippets/a1b2c3d4e5f6", "", nil)

		assert.Equal(t, status, http.StatusOK)
		assert.Equal[any](t, body["snippet"].(map[string]any)["content"], "An old silent pond...")
	})

	t.Run("Someone else's private snippet", func(t *testing.T) {
		status, _, body := ts.apiRequest(t, http.MethodGet, "/api/v1/snippets/0f0f0f0f0f0f", mocks.ValidToken, nil)

		assert.Equal(t, status, http.StatusNotFound)
		assert.Equal[any](t, body["error"], "snippet not found")
	})

	t.Run("My snippets", func(t *testing.T) {
		status, _, body := ts.apiRequest(t, http.MethodGet, "/api/v1/me/snippets", mocks.ValidToken, nil)

		assert.Equal(t, status, http.StatusOK)
		assert.Equal(t, len(body["snippets"].([]any)), 1)
	})

	t.Run("Languages", func(t *testing.T) {
		status, _, body := ts.apiRequest(t, http.MethodGet, "/api/v1/languages", "", nil)

		assert.Equal(t, status, http.StatusOK)
		assert.Equal(t, len(body["languages"].([]any)) > 10, true)
	})
}

func TestAPIWriteSnippets(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	token := mocks.ValidToken

	t.Run("Create", func(t *testing.T) {
		status, headers, body := ts.apiRequest(t, http.MethodPost, "/api/v1/snippets", token, map[string]any{
			"title": "From the API", "content": "package main", "language": "go",
		})

		assert.Equal(t, status, http.StatusCreated)
		// The mock returns slug ffffffffffff for new snippets.
		assert.Equal(t, headers.Get("Location"), "/api/v1/snippets/ffffffffffff")
		assert.Equal[any](t, body["snippet"].(map[string]any)["slug"], "ffffffffffff")
	})

	t.Run("Create needs a token", func(t *testing.T) {
		status, _, _ := ts.apiRequest(t, http.MethodPost, "/api/v1/snippets", "", map[string]any{"title": "x", "content": "y"})
		assert.Equal(t, status, http.StatusUnauthorized)
	})

	t.Run("Create validation", func(t *testing.T) {
		status, _, body := ts.apiRequest(t, http.MethodPost, "/api/v1/snippets", token, map[string]any{
			"title": "", "content": strings.Repeat("x", maxContentBytes+1), "language": "klingon", "visibility": "secret", "expires_in_days": 3,
		})

		assert.Equal(t, status, http.StatusUnprocessableEntity)
		fields := body["fields"].(map[string]any)
		for _, f := range []string{"title", "content", "language", "visibility", "expires_in_days"} {
			if _, ok := fields[f]; !ok {
				t.Errorf("expected a validation error for %q, got %v", f, fields)
			}
		}
	})

	t.Run("Update own snippet", func(t *testing.T) {
		status, _, body := ts.apiRequest(t, http.MethodPatch, "/api/v1/snippets/a1b2c3d4e5f6", token, map[string]any{"visibility": "private"})

		assert.Equal(t, status, http.StatusOK)
		snippet := body["snippet"].(map[string]any)
		assert.Equal[any](t, snippet["visibility"], "private")
		// Fields not in the request keep their values.
		assert.Equal[any](t, snippet["title"], "An old silent pond")
	})

	t.Run("Update with invalid value", func(t *testing.T) {
		status, _, _ := ts.apiRequest(t, http.MethodPatch, "/api/v1/snippets/a1b2c3d4e5f6", token, map[string]any{"title": ""})
		assert.Equal(t, status, http.StatusUnprocessableEntity)
	})

	t.Run("Update someone else's snippet", func(t *testing.T) {
		status, _, _ := ts.apiRequest(t, http.MethodPatch, "/api/v1/snippets/0f0f0f0f0f0f", token, map[string]any{"title": "hacked"})
		assert.Equal(t, status, http.StatusNotFound)
	})

	t.Run("Delete someone else's snippet", func(t *testing.T) {
		status, _, _ := ts.apiRequest(t, http.MethodDelete, "/api/v1/snippets/0f0f0f0f0f0f", token, nil)
		assert.Equal(t, status, http.StatusNotFound)
	})

	t.Run("Delete own snippet", func(t *testing.T) {
		status, _, _ := ts.apiRequest(t, http.MethodDelete, "/api/v1/snippets/a1b2c3d4e5f6", token, nil)
		assert.Equal(t, status, http.StatusNoContent)
	})
}

func TestAPIRateLimitAndErrors(t *testing.T) {
	app := newTestApplication(t)
	app.authLimiter = newIPRateLimiter(rate.Every(time.Hour), 1)
	ts := newTestServer(t, app.routes())

	creds := map[string]string{"email": "alice@example.com", "password": "nope"}
	ts.apiRequest(t, http.MethodPost, "/api/v1/tokens", "", creds)

	status, headers, body := ts.apiRequest(t, http.MethodPost, "/api/v1/tokens", "", creds)
	assert.Equal(t, status, http.StatusTooManyRequests)
	assert.Equal(t, headers.Get("Content-Type"), "application/json")
	assert.StringContains(t, body["error"].(string), "too many attempts")
}

func TestPasswordChangeRevokesAPITokens(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	status, _, _ := ts.apiRequest(t, http.MethodGet, "/api/v1/me", mocks.ValidToken, nil)
	assert.Equal(t, status, http.StatusOK)

	browser := ts.newSession(t)
	browser.login(t)
	form := map[string][]string{
		"currentPassword":         {"pa$$word"},
		"newPassword":             {"newPa$$word"},
		"newPasswordConfirmation": {"newPa$$word"},
	}
	res := browser.postWithCSRF(t, "/account/password/update", "/account/password/update", form)
	assert.Equal(t, res.status, http.StatusSeeOther)

	status, _, _ = ts.apiRequest(t, http.MethodGet, "/api/v1/me", mocks.ValidToken, nil)
	assert.Equal(t, status, http.StatusUnauthorized)
}
