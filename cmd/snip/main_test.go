package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/satyaorz/snippetbox/internal/assert"
)

const testToken = "sb_testtokentesttokentesttokentest"

// fakeAPI is a minimal stand-in for the Snippetbox API that records what the
// CLI sent.
type fakeAPI struct {
	*httptest.Server
	lastAuth    string
	lastCreated map[string]any
	lastLogin   map[string]string
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	api := &fakeAPI{}
	mux := http.NewServeMux()

	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(v)
	}
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		api.lastAuth = r.Header.Get("Authorization")
		if api.lastAuth != "Bearer "+testToken {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired API token"})
			return false
		}
		return true
	}

	mux.HandleFunc("POST /api/v1/tokens", func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&api.lastLogin)
		if api.lastLogin["password"] != "pa$$word" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"token": testToken, "expires": time.Now().Add(time.Hour)})
	})
	mux.HandleFunc("DELETE /api/v1/tokens/current", func(w http.ResponseWriter, r *http.Request) {
		if authed(w, r) {
			w.WriteHeader(http.StatusNoContent)
		}
	})
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, r *http.Request) {
		if authed(w, r) {
			writeJSON(w, http.StatusOK, map[string]any{"user": map[string]any{"name": "Alice", "email": "alice@example.com"}})
		}
	})
	mux.HandleFunc("POST /api/v1/snippets", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		json.NewDecoder(r.Body).Decode(&api.lastCreated)
		if api.lastCreated["title"] == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation failed", "fields": map[string]string{"title": "This field cannot be blank"}})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"snippet": map[string]any{"slug": "abcdef123456", "url": api.URL + "/snippet/view/abcdef123456"}})
	})
	mux.HandleFunc("GET /api/v1/snippets/{slug}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("slug") != "abcdef123456" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "snippet not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"snippet": map[string]any{"content": "package main"}})
	})
	mux.HandleFunc("GET /api/v1/me/snippets", func(w http.ResponseWriter, r *http.Request) {
		if authed(w, r) {
			writeJSON(w, http.StatusOK, map[string]any{"snippets": []map[string]any{
				{"slug": "abcdef123456", "title": "main.go", "visibility": "unlisted", "expires": "2030-01-02T00:00:00Z"},
			}})
		}
	})
	mux.HandleFunc("DELETE /api/v1/snippets/{slug}", func(w http.ResponseWriter, r *http.Request) {
		if authed(w, r) {
			w.WriteHeader(http.StatusNoContent)
		}
	})

	api.Server = httptest.NewServer(mux)
	t.Cleanup(api.Close)
	return api
}

type testCLI struct {
	t          *testing.T
	api        *fakeAPI
	configPath string
	vars       map[string]string
}

func newTestCLI(t *testing.T) *testCLI {
	api := newFakeAPI(t)
	return &testCLI{
		t:          t,
		api:        api,
		configPath: filepath.Join(t.TempDir(), "snip", "config.json"),
		vars:       map[string]string{"SNIP_SERVER": api.URL},
	}
}

// run executes the CLI with the given stdin and returns stdout and the error.
func (c *testCLI) run(stdin string, args ...string) (string, error) {
	c.t.Helper()
	var stdout, stderr bytes.Buffer

	err := run(args, env{
		stdin:  strings.NewReader(stdin),
		stdout: &stdout,
		stderr: &stderr,
		getenv: func(key string) string {
			if key == "SNIP_CONFIG" {
				return c.configPath
			}
			return c.vars[key]
		},
		hostname: "testhost",
	})
	return stdout.String(), err
}

func (c *testCLI) login() {
	c.t.Helper()
	if _, err := c.run("alice@example.com\npa$$word\n", "login"); err != nil {
		c.t.Fatal(err)
	}
}

func TestLogin(t *testing.T) {
	cli := newTestCLI(t)

	out, err := cli.run("alice@example.com\npa$$word\n", "login")
	assert.NilError(t, err)
	assert.StringContains(t, out, "Logged in to "+cli.api.URL)
	assert.Equal(t, cli.api.lastLogin["email"], "alice@example.com")
	assert.Equal(t, cli.api.lastLogin["name"], "snip on testhost")

	// The token is saved, readable only by the user.
	info, err := os.Stat(cli.configPath)
	assert.NilError(t, err)
	assert.Equal(t, info.Mode().Perm(), os.FileMode(0o600))
	cfg, err := loadConfig(cli.configPath)
	assert.NilError(t, err)
	assert.Equal(t, cfg.Token, testToken)
	assert.Equal(t, cfg.Server, cli.api.URL)

	out, err = cli.run("", "whoami")
	assert.NilError(t, err)
	assert.StringContains(t, out, "Alice <alice@example.com>")
}

func TestLoginWrongPassword(t *testing.T) {
	cli := newTestCLI(t)

	_, err := cli.run("alice@example.com\nwrong\n", "login")
	assert.StringContains(t, err.Error(), "invalid email or password")

	_, err = os.Stat(cli.configPath)
	assert.Equal(t, os.IsNotExist(err), true)
}

func TestCreate(t *testing.T) {
	cli := newTestCLI(t)

	t.Run("Not logged in", func(t *testing.T) {
		_, err := cli.run("hello", "create")
		assert.StringContains(t, err.Error(), "run `snip login` first")
	})

	cli.login()

	t.Run("From a file", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "main.go")
		os.WriteFile(file, []byte("package main\n"), 0o644)

		// "create" is the default command, so it can be left out.
		out, err := cli.run("", file)
		assert.NilError(t, err)
		assert.Equal(t, strings.TrimSpace(out), cli.api.URL+"/snippet/view/abcdef123456")

		assert.Equal(t, cli.api.lastAuth, "Bearer "+testToken)
		assert.Equal[any](t, cli.api.lastCreated["title"], "main.go")
		assert.Equal[any](t, cli.api.lastCreated["language"], "go")
		assert.Equal[any](t, cli.api.lastCreated["content"], "package main\n")
		assert.Equal[any](t, cli.api.lastCreated["visibility"], "unlisted")
		assert.Equal[any](t, cli.api.lastCreated["expires_in_days"], float64(7))
	})

	t.Run("From stdin with flags", func(t *testing.T) {
		_, err := cli.run("SELECT 1;", "-t", "Query", "-l", "sql", "-v", "public", "-e", "365")
		assert.NilError(t, err)

		assert.Equal[any](t, cli.api.lastCreated["title"], "Query")
		assert.Equal[any](t, cli.api.lastCreated["language"], "sql")
		assert.Equal[any](t, cli.api.lastCreated["visibility"], "public")
		assert.Equal[any](t, cli.api.lastCreated["expires_in_days"], float64(365))
	})

	t.Run("Flags after the file name", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "query.sql")
		os.WriteFile(file, []byte("SELECT 1;"), 0o644)

		_, err := cli.run("", file, "-v", "public", "-e", "1")
		assert.NilError(t, err)
		assert.Equal[any](t, cli.api.lastCreated["title"], "query.sql")
		assert.Equal[any](t, cli.api.lastCreated["visibility"], "public")
		assert.Equal[any](t, cli.api.lastCreated["expires_in_days"], float64(1))
	})

	t.Run("Stdin without a title", func(t *testing.T) {
		_, err := cli.run("x", "create")
		assert.NilError(t, err)
		assert.Equal[any](t, cli.api.lastCreated["title"], "Untitled")
		assert.Equal[any](t, cli.api.lastCreated["language"], "")
	})

	t.Run("Validation errors are readable", func(t *testing.T) {
		apiErr := &apiError{Status: 422, Message: "validation failed", Fields: map[string]string{"title": "blank", "content": "too long"}}
		assert.Equal(t, apiErr.Error(), "validation failed (content: too long; title: blank)")
	})
}

func TestGetListRemove(t *testing.T) {
	cli := newTestCLI(t)
	cli.login()

	out, err := cli.run("", "get", cli.api.URL+"/snippet/view/abcdef123456")
	assert.NilError(t, err)
	assert.Equal(t, out, "package main\n")

	_, err = cli.run("", "get", "000000000000")
	assert.StringContains(t, err.Error(), "snippet not found")

	out, err = cli.run("", "ls")
	assert.NilError(t, err)
	assert.StringContains(t, out, "SLUG")
	assert.StringContains(t, out, "abcdef123456  unlisted")

	out, err = cli.run("", "rm", "abcdef123456")
	assert.NilError(t, err)
	assert.StringContains(t, out, "Deleted abcdef123456")
}

func TestExpiredToken(t *testing.T) {
	cli := newTestCLI(t)
	saveConfig(cli.configPath, savedConfig{Server: cli.api.URL, Token: "sb_revoked"})

	_, err := cli.run("", "whoami")
	assert.StringContains(t, err.Error(), "run `snip login`")
}

func TestTokenOnlySentToItsServer(t *testing.T) {
	cli := newTestCLI(t)
	cli.login()

	// Point the CLI at a different server: the saved token must not be sent.
	other := newFakeAPI(t)
	cli.vars["SNIP_SERVER"] = other.URL

	_, err := cli.run("", "whoami")
	assert.StringContains(t, err.Error(), "not logged in to "+other.URL)
}

func TestLogout(t *testing.T) {
	cli := newTestCLI(t)
	cli.login()

	out, err := cli.run("", "logout")
	assert.NilError(t, err)
	assert.StringContains(t, out, "Logged out")

	cfg, _ := loadConfig(cli.configPath)
	assert.Equal(t, cfg.Token, "")
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		args     []string
		wantV    string
		wantArgs string
	}{
		{[]string{"-v", "public", "a.go"}, "public", "a.go"},
		{[]string{"a.go", "-v", "public"}, "public", "a.go"},
		{[]string{"a.go", "-v", "public", "b.go"}, "public", "a.go b.go"},
		{[]string{"-", "-v", "private"}, "private", "-"},
		{[]string{"--", "-v", "x"}, "unlisted", "-v x"},
		{[]string{"a.go", "--", "-v"}, "unlisted", "a.go -v"},
		{nil, "unlisted", ""},
	}

	for _, tt := range tests {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		v := fs.String("v", "unlisted", "")

		assert.NilError(t, parseArgs(fs, tt.args))
		assert.Equal(t, *v, tt.wantV)
		assert.Equal(t, strings.Join(fs.Args(), " "), tt.wantArgs)
	}
}

func TestSlugFromArg(t *testing.T) {
	assert.Equal(t, slugFromArg("abcdef123456"), "abcdef123456")
	assert.Equal(t, slugFromArg("https://example.com/snippet/view/abcdef123456"), "abcdef123456")
	assert.Equal(t, slugFromArg("https://example.com/snippet/view/abcdef123456/"), "abcdef123456")
}
