package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/satyaorz/snippetbox/internal/assert"
)

func TestPing(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	res := ts.get(t, "/ping")

	assert.Equal(t, res.status, http.StatusOK)
	assert.Equal(t, res.body, "OK")
}

func TestHome(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	t.Run("Latest", func(t *testing.T) {
		res := ts.get(t, "/")

		assert.Equal(t, res.status, http.StatusOK)
		assert.StringContains(t, res.body, "<a href='/snippet/view/a1b2c3d4e5f6'>An old silent pond</a>")
		assert.StringContains(t, res.body, "<td>Alice</td>")
		assert.StringContains(t, res.body, "<a href='/user/login'>Login</a>")
	})

	t.Run("Search with results", func(t *testing.T) {
		res := ts.get(t, "/?q=pond")

		assert.Equal(t, res.status, http.StatusOK)
		assert.StringContains(t, res.body, "Results for “pond”")
		assert.StringContains(t, res.body, "An old silent pond")
		assert.StringContains(t, res.body, "<input type='search' name='q' value='pond'")
	})

	t.Run("Search without results", func(t *testing.T) {
		res := ts.get(t, "/?q=nothing")

		assert.Equal(t, res.status, http.StatusOK)
		assert.StringContains(t, res.body, "No snippets matched your search.")
	})

	t.Run("Search term is escaped", func(t *testing.T) {
		res := ts.get(t, "/?q="+url.QueryEscape("<script>"))

		assert.Equal(t, res.status, http.StatusOK)
		assert.Equal(t, strings.Contains(res.body, "<script>"), false)
	})

	t.Run("Page past the end", func(t *testing.T) {
		res := ts.get(t, "/?page=5")

		assert.Equal(t, res.status, http.StatusOK)
		assert.StringContains(t, res.body, "There's nothing to see here")
		assert.StringContains(t, res.body, "<a href='/'>← Newer</a>")
	})

	t.Run("Invalid page number", func(t *testing.T) {
		res := ts.get(t, "/?page=abc")

		assert.Equal(t, res.status, http.StatusOK)
		assert.StringContains(t, res.body, "An old silent pond")
	})
}

func TestAbout(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	res := ts.get(t, "/about")

	assert.Equal(t, res.status, http.StatusOK)
	assert.StringContains(t, res.body, "<h2>About</h2>")
	assert.StringContains(t, res.body, "<a href='/about'>About</a>")
}

func TestSnippetView(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	tests := []struct {
		name     string
		urlPath  string
		wantCode int
		wantBody string
	}{
		{name: "Valid slug", urlPath: "/snippet/view/a1b2c3d4e5f6", wantCode: http.StatusOK, wantBody: "An old silent pond..."},
		{name: "Non-existent slug", urlPath: "/snippet/view/ffffffffffff", wantCode: http.StatusNotFound},
		{name: "Old numeric ID", urlPath: "/snippet/view/1", wantCode: http.StatusNotFound},
		{name: "Uppercase slug", urlPath: "/snippet/view/A1B2C3D4E5F6", wantCode: http.StatusNotFound},
		{name: "Too long", urlPath: "/snippet/view/a1b2c3d4e5f6a", wantCode: http.StatusNotFound},
		{name: "Empty slug", urlPath: "/snippet/view/", wantCode: http.StatusNotFound},
		{name: "Someone else's private snippet", urlPath: "/snippet/view/0f0f0f0f0f0f", wantCode: http.StatusNotFound},
		{name: "Raw", urlPath: "/snippet/raw/a1b2c3d4e5f6", wantCode: http.StatusOK, wantBody: "An old silent pond..."},
		{name: "Raw private", urlPath: "/snippet/raw/0f0f0f0f0f0f", wantCode: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := ts.get(t, tt.urlPath)

			assert.Equal(t, res.status, tt.wantCode)

			if tt.wantBody != "" {
				assert.StringContains(t, res.body, tt.wantBody)
			}
		})
	}

	t.Run("Raw is plain text", func(t *testing.T) {
		res := ts.get(t, "/snippet/raw/a1b2c3d4e5f6")

		assert.Equal(t, res.headers.Get("Content-Type"), "text/plain; charset=utf-8")
		assert.Equal(t, res.body, "An old silent pond...")
	})

	t.Run("Owner controls", func(t *testing.T) {
		// Anonymous visitors don't see the edit/delete controls...
		res := ts.get(t, "/snippet/view/a1b2c3d4e5f6")
		assert.Equal(t, strings.Contains(res.body, "/snippet/edit/a1b2c3d4e5f6"), false)
		assert.StringContains(t, res.body, "by Alice")

		// ...but the owner does.
		owner := ts.newSession(t)
		owner.login(t)
		res = owner.get(t, "/snippet/view/a1b2c3d4e5f6")
		assert.StringContains(t, res.body, "<a href='/snippet/edit/a1b2c3d4e5f6'>Edit</a>")
		assert.StringContains(t, res.body, "<form action='/snippet/delete/a1b2c3d4e5f6' method='POST'")
	})
}

func TestUserSignup(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	// Make a GET request to the signup page to get a CSRF cookie and token.
	res := ts.get(t, "/user/signup")
	validCSRFToken := extractCSRFToken(t, res.body)

	const (
		validName     = "Bob"
		validPassword = "validPa$$word"
		validEmail    = "bob@example.com"
		formTag       = "<form action='/user/signup' method='POST' novalidate>"
	)

	tests := []struct {
		name         string
		userName     string
		userEmail    string
		userPassword string
		csrfToken    string
		wantCode     int
		wantFormTag  string
	}{
		{"Valid submission", validName, validEmail, validPassword, validCSRFToken, http.StatusSeeOther, ""},
		{"Invalid CSRF Token", validName, validEmail, validPassword, "wrongToken", http.StatusBadRequest, ""},
		{"Empty name", "", validEmail, validPassword, validCSRFToken, http.StatusUnprocessableEntity, formTag},
		{"Empty email", validName, "", validPassword, validCSRFToken, http.StatusUnprocessableEntity, formTag},
		{"Empty password", validName, validEmail, "", validCSRFToken, http.StatusUnprocessableEntity, formTag},
		{"Invalid email", validName, "bob@example.", validPassword, validCSRFToken, http.StatusUnprocessableEntity, formTag},
		{"Short password", validName, validEmail, "pa$$", validCSRFToken, http.StatusUnprocessableEntity, formTag},
		{"Duplicate email", validName, "dupe@example.com", validPassword, validCSRFToken, http.StatusUnprocessableEntity, formTag},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Add("name", tt.userName)
			form.Add("email", tt.userEmail)
			form.Add("password", tt.userPassword)
			form.Add("csrf_token", tt.csrfToken)

			res := ts.postForm(t, "/user/signup", form)

			assert.Equal(t, res.status, tt.wantCode)

			if tt.wantFormTag != "" {
				assert.StringContains(t, res.body, tt.wantFormTag)
			}
		})
	}
}

func TestUserLogin(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	res := ts.get(t, "/user/login")
	csrfToken := extractCSRFToken(t, res.body)

	t.Run("Wrong password", func(t *testing.T) {
		form := url.Values{}
		form.Add("email", "alice@example.com")
		form.Add("password", "wrong")
		form.Add("csrf_token", csrfToken)

		res := ts.postForm(t, "/user/login", form)

		assert.Equal(t, res.status, http.StatusUnprocessableEntity)
		assert.StringContains(t, res.body, "Email or password is incorrect")
	})

	t.Run("Valid credentials", func(t *testing.T) {
		form := url.Values{}
		form.Add("email", "alice@example.com")
		form.Add("password", "pa$$word")
		form.Add("csrf_token", csrfToken)

		res := ts.postForm(t, "/user/login", form)

		assert.Equal(t, res.status, http.StatusSeeOther)
		assert.Equal(t, res.headers.Get("Location"), "/snippet/create")
	})
}

func TestSnippetCreate(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	t.Run("Unauthenticated", func(t *testing.T) {
		res := ts.get(t, "/snippet/create")

		assert.Equal(t, res.status, http.StatusSeeOther)
		assert.Equal(t, res.headers.Get("Location"), "/user/login")
	})

	t.Run("Authenticated", func(t *testing.T) {
		ts.login(t)

		res := ts.get(t, "/snippet/create")

		assert.Equal(t, res.status, http.StatusOK)
		assert.StringContains(t, res.body, "<form action='/snippet/create' method='POST'>")
		assert.Equal(t, res.headers.Get("Cache-Control"), "no-store")
	})

	t.Run("Create snippet", func(t *testing.T) {
		res := ts.get(t, "/snippet/create")

		form := url.Values{}
		form.Add("title", "A new snippet")
		form.Add("content", "Some content")
		form.Add("language", "go")
		form.Add("visibility", "unlisted")
		form.Add("expires", "7")
		form.Add("csrf_token", extractCSRFToken(t, res.body))

		res = ts.postForm(t, "/snippet/create", form)

		assert.Equal(t, res.status, http.StatusSeeOther)
		assert.Equal(t, res.headers.Get("Location"), "/snippet/view/ffffffffffff")
	})

	t.Run("Invalid snippet", func(t *testing.T) {
		res := ts.get(t, "/snippet/create")

		form := url.Values{}
		form.Add("title", "")
		form.Add("content", "")
		form.Add("language", "klingon")
		form.Add("visibility", "secret")
		form.Add("expires", "42")
		form.Add("csrf_token", extractCSRFToken(t, res.body))

		res = ts.postForm(t, "/snippet/create", form)

		assert.Equal(t, res.status, http.StatusUnprocessableEntity)
		assert.StringContains(t, res.body, "This field cannot be blank")
		assert.StringContains(t, res.body, "Please choose a language from the list")
		assert.StringContains(t, res.body, "Please choose a visibility")
		assert.StringContains(t, res.body, "This field must equal 1, 7 or 365")
	})
}

func TestAccountView(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	t.Run("Unauthenticated", func(t *testing.T) {
		res := ts.get(t, "/account/view")

		assert.Equal(t, res.status, http.StatusSeeOther)
		assert.Equal(t, res.headers.Get("Location"), "/user/login")
	})

	t.Run("Authenticated", func(t *testing.T) {
		ts.login(t)

		res := ts.get(t, "/account/view")

		assert.Equal(t, res.status, http.StatusOK)
		assert.StringContains(t, res.body, "<td>Alice</td>")
		assert.StringContains(t, res.body, "<td>alice@example.com</td>")
		assert.StringContains(t, res.body, "<td>01 Jan 2022 at 09:18</td>")
		assert.StringContains(t, res.body, "<a href='/account/view'>Account</a>")
		assert.StringContains(t, res.body, "<a href='/snippet/view/a1b2c3d4e5f6'>An old silent pond</a>")
	})
}

func TestRedirectAfterLogin(t *testing.T) {
	t.Run("Back to the requested page", func(t *testing.T) {
		app := newTestApplication(t)
		ts := newTestServer(t, app.routes())

		res := ts.get(t, "/account/view")
		assert.Equal(t, res.headers.Get("Location"), "/user/login")

		res = ts.login(t)
		assert.Equal(t, res.headers.Get("Location"), "/account/view")
	})

	t.Run("Only redirects once", func(t *testing.T) {
		app := newTestApplication(t)
		ts := newTestServer(t, app.routes())

		ts.get(t, "/account/view")
		ts.login(t)

		// Log out and back in again: the stored path was popped on the first
		// login, so we fall back to the default page.
		res := ts.get(t, "/")
		form := url.Values{}
		form.Add("csrf_token", extractCSRFToken(t, res.body))
		res = ts.postForm(t, "/user/logout", form)
		assert.Equal(t, res.status, http.StatusSeeOther)

		res = ts.login(t)
		assert.Equal(t, res.headers.Get("Location"), "/snippet/create")
	})

	t.Run("Default without a stored page", func(t *testing.T) {
		app := newTestApplication(t)
		ts := newTestServer(t, app.routes())

		res := ts.login(t)
		assert.Equal(t, res.headers.Get("Location"), "/snippet/create")
	})
}

func TestAccountPasswordUpdate(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	t.Run("Unauthenticated", func(t *testing.T) {
		res := ts.get(t, "/account/password/update")

		assert.Equal(t, res.status, http.StatusSeeOther)
		assert.Equal(t, res.headers.Get("Location"), "/user/login")
	})

	ts.login(t)

	res := ts.get(t, "/account/password/update")
	assert.Equal(t, res.status, http.StatusOK)
	csrfToken := extractCSRFToken(t, res.body)

	const formTag = "<form action='/account/password/update' method='POST' novalidate>"

	tests := []struct {
		name            string
		currentPassword string
		newPassword     string
		confirmation    string
		wantCode        int
		wantBody        string
	}{
		{"Valid", "pa$$word", "newPa$$word", "newPa$$word", http.StatusSeeOther, ""},
		{"Wrong current password", "wrong", "newPa$$word", "newPa$$word", http.StatusUnprocessableEntity, "Current password is incorrect"},
		{"Empty current password", "", "newPa$$word", "newPa$$word", http.StatusUnprocessableEntity, formTag},
		{"Short new password", "pa$$word", "short", "short", http.StatusUnprocessableEntity, "at least 8 characters"},
		{"Mismatched confirmation", "pa$$word", "newPa$$word", "different1", http.StatusUnprocessableEntity, "Passwords do not match"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Add("currentPassword", tt.currentPassword)
			form.Add("newPassword", tt.newPassword)
			form.Add("newPasswordConfirmation", tt.confirmation)
			form.Add("csrf_token", csrfToken)

			res := ts.postForm(t, "/account/password/update", form)

			assert.Equal(t, res.status, tt.wantCode)

			if tt.wantCode == http.StatusSeeOther {
				assert.Equal(t, res.headers.Get("Location"), "/account/view")
			}
			if tt.wantBody != "" {
				assert.StringContains(t, res.body, tt.wantBody)
			}
		})
	}
}

func TestSnippetEdit(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	t.Run("Unauthenticated", func(t *testing.T) {
		res := ts.get(t, "/snippet/edit/a1b2c3d4e5f6")

		assert.Equal(t, res.status, http.StatusSeeOther)
		assert.Equal(t, res.headers.Get("Location"), "/user/login")
	})

	ts.login(t) // as Alice, user 1

	t.Run("Owner sees pre-filled form", func(t *testing.T) {
		res := ts.get(t, "/snippet/edit/a1b2c3d4e5f6")

		assert.Equal(t, res.status, http.StatusOK)
		assert.StringContains(t, res.body, "<form action='/snippet/edit/a1b2c3d4e5f6' method='POST'>")
		assert.StringContains(t, res.body, "value='An old silent pond'")
	})

	t.Run("Someone else's snippet", func(t *testing.T) {
		res := ts.get(t, "/snippet/edit/0f0f0f0f0f0f")
		assert.Equal(t, res.status, http.StatusNotFound)
	})

	editForm := func(title string) url.Values {
		form := url.Values{}
		form.Add("title", title)
		form.Add("content", "Updated content")
		form.Add("language", "python")
		form.Add("visibility", "private")
		return form
	}

	t.Run("Valid edit", func(t *testing.T) {
		res := ts.postWithCSRF(t, "/snippet/edit/a1b2c3d4e5f6", "/snippet/edit/a1b2c3d4e5f6", editForm("Updated"))

		assert.Equal(t, res.status, http.StatusSeeOther)
		assert.Equal(t, res.headers.Get("Location"), "/snippet/view/a1b2c3d4e5f6")
	})

	t.Run("Invalid edit", func(t *testing.T) {
		res := ts.postWithCSRF(t, "/snippet/edit/a1b2c3d4e5f6", "/snippet/edit/a1b2c3d4e5f6", editForm(""))

		assert.Equal(t, res.status, http.StatusUnprocessableEntity)
		assert.StringContains(t, res.body, "This field cannot be blank")
	})

	t.Run("Posting to someone else's snippet", func(t *testing.T) {
		res := ts.postWithCSRF(t, "/snippet/edit/a1b2c3d4e5f6", "/snippet/edit/0f0f0f0f0f0f", editForm("Hacked"))

		assert.Equal(t, res.status, http.StatusNotFound)
	})
}

func TestSnippetDelete(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	ts.login(t)

	t.Run("Someone else's snippet", func(t *testing.T) {
		res := ts.postWithCSRF(t, "/", "/snippet/delete/0f0f0f0f0f0f", url.Values{})
		assert.Equal(t, res.status, http.StatusNotFound)
	})

	t.Run("Own snippet", func(t *testing.T) {
		res := ts.postWithCSRF(t, "/", "/snippet/delete/a1b2c3d4e5f6", url.Values{})

		assert.Equal(t, res.status, http.StatusSeeOther)
		assert.Equal(t, res.headers.Get("Location"), "/account/view")
	})

	t.Run("Without CSRF token", func(t *testing.T) {
		res := ts.postForm(t, "/snippet/delete/a1b2c3d4e5f6", url.Values{})
		assert.Equal(t, res.status, http.StatusBadRequest)
	})
}

func TestPasswordChangeLogsOutOtherSessions(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	laptop := ts.newSession(t)
	phone := ts.newSession(t)
	laptop.login(t)
	phone.login(t)

	assert.Equal(t, phone.get(t, "/account/view").status, http.StatusOK)

	form := url.Values{}
	form.Add("currentPassword", "pa$$word")
	form.Add("newPassword", "newPa$$word")
	form.Add("newPasswordConfirmation", "newPa$$word")
	res := laptop.postWithCSRF(t, "/account/password/update", "/account/password/update", form)
	assert.Equal(t, res.status, http.StatusSeeOther)

	// The session that changed the password stays logged in...
	assert.Equal(t, laptop.get(t, "/account/view").status, http.StatusOK)

	// ...but every other session is logged out.
	res = phone.get(t, "/account/view")
	assert.Equal(t, res.status, http.StatusSeeOther)
	assert.Equal(t, res.headers.Get("Location"), "/user/login")
}

func TestRateLimit(t *testing.T) {
	app := newTestApplication(t)
	// Two attempts, then one per hour: effectively a hard limit of two.
	app.authLimiter = newIPRateLimiter(rate.Every(time.Hour), 2)
	ts := newTestServer(t, app.routes())

	badLogin := func() testResponse {
		form := url.Values{}
		form.Add("email", "alice@example.com")
		form.Add("password", "wrong")
		return ts.postWithCSRF(t, "/user/login", "/user/login", form)
	}

	assert.Equal(t, badLogin().status, http.StatusUnprocessableEntity)
	assert.Equal(t, badLogin().status, http.StatusUnprocessableEntity)

	res := badLogin()
	assert.Equal(t, res.status, http.StatusTooManyRequests)
	assert.Equal(t, res.headers.Get("Retry-After"), "3600")

	// Signup shares the same allowance; ordinary pages aren't limited.
	res = ts.postWithCSRF(t, "/user/signup", "/user/signup", url.Values{})
	assert.Equal(t, res.status, http.StatusTooManyRequests)
	assert.Equal(t, ts.get(t, "/user/login").status, http.StatusOK)
}

func TestSyntaxCSS(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	res := ts.get(t, "/static/css/syntax.css")

	assert.Equal(t, res.status, http.StatusOK)
	assert.Equal(t, res.headers.Get("Content-Type"), "text/css; charset=utf-8")
	assert.StringContains(t, res.body, ".chroma")

	// Other static files are still served from the embedded filesystem.
	assert.Equal(t, ts.get(t, "/static/css/main.css").status, http.StatusOK)
}
