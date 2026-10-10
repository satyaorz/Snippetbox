package main

import (
	"bytes"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/satyaorz/snippetbox/internal/models/mocks"

	"github.com/alexedwards/scs/v2"
	"github.com/go-playground/form/v4"
)

// newTestApplication returns an application wired up with mock models, an
// in-memory session store and a logger that discards everything.
func newTestApplication(t *testing.T) *application {
	t.Helper()

	templateCache, err := newTemplateCache()
	if err != nil {
		t.Fatal(err)
	}

	sessionManager := scs.New()
	sessionManager.Lifetime = 12 * time.Hour
	sessionManager.Cookie.Secure = true

	return &application{
		logger:         slog.New(slog.DiscardHandler),
		snippets:       &mocks.SnippetModel{},
		users:          &mocks.UserModel{},
		templateCache:  templateCache,
		formDecoder:    form.NewDecoder(),
		sessionManager: sessionManager,
	}
}

// testServer embeds an httptest.Server so we can add helper methods to it.
type testServer struct {
	*httptest.Server
}

// newTestServer starts an HTTPS test server for the given handler. Its client
// keeps cookies between requests and does not follow redirects, so tests can
// assert on the redirect responses themselves.
func newTestServer(t *testing.T, h http.Handler) *testServer {
	t.Helper()

	ts := httptest.NewTLSServer(h)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	ts.Client().Jar = jar

	ts.Client().CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	t.Cleanup(ts.Close)

	return &testServer{ts}
}

type testResponse struct {
	status  int
	headers http.Header
	cookies []*http.Cookie
	body    string
}

// get makes a GET request to the given path on the test server.
func (ts *testServer) get(t *testing.T, urlPath string) testResponse {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, ts.URL+urlPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	return ts.do(t, req)
}

// postForm makes a POST request with URL-encoded form data. It sets the
// Origin header like a browser would, because nosurf rejects cross-origin (or
// origin-less) state-changing requests.
func (ts *testServer) postForm(t *testing.T, urlPath string, form url.Values) testResponse {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, ts.URL+urlPath, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", ts.URL)

	return ts.do(t, req)
}

func (ts *testServer) do(t *testing.T, req *http.Request) testResponse {
	t.Helper()

	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	return testResponse{
		status:  res.StatusCode,
		headers: res.Header,
		cookies: res.Cookies(),
		body:    string(bytes.TrimSpace(body)),
	}
}

var csrfTokenRX = regexp.MustCompile(`<input type='hidden' name='csrf_token' value='(.+)'>`)

// extractCSRFToken pulls the CSRF token out of a rendered HTML page. The
// token is HTML-escaped by html/template (e.g. "+" becomes "&#43;"), so we
// unescape it before returning.
func extractCSRFToken(t *testing.T, body string) string {
	t.Helper()

	matches := csrfTokenRX.FindStringSubmatch(body)
	if len(matches) < 2 {
		t.Fatal("no csrf token found in body")
	}

	return html.UnescapeString(matches[1])
}

// login signs in as the mock user "alice@example.com" (user ID 1).
func (ts *testServer) login(t *testing.T) {
	t.Helper()

	res := ts.get(t, "/user/login")
	form := url.Values{}
	form.Add("email", "alice@example.com")
	form.Add("password", "pa$$word")
	form.Add("csrf_token", extractCSRFToken(t, res.body))

	res = ts.postForm(t, "/user/login", form)
	if res.status != http.StatusSeeOther {
		t.Fatalf("login failed: got status %d", res.status)
	}
}
