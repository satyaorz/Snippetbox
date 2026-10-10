package main

import (
	"net/http"
	"net/url"
	"testing"

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

	res := ts.get(t, "/")

	assert.Equal(t, res.status, http.StatusOK)
	assert.StringContains(t, res.body, "An old silent pond")
	assert.StringContains(t, res.body, "<a href='/user/login'>Login</a>")
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
		{name: "Valid ID", urlPath: "/snippet/view/1", wantCode: http.StatusOK, wantBody: "An old silent pond..."},
		{name: "Non-existent ID", urlPath: "/snippet/view/2", wantCode: http.StatusNotFound},
		{name: "Negative ID", urlPath: "/snippet/view/-1", wantCode: http.StatusNotFound},
		{name: "Decimal ID", urlPath: "/snippet/view/1.23", wantCode: http.StatusNotFound},
		{name: "String ID", urlPath: "/snippet/view/foo", wantCode: http.StatusNotFound},
		{name: "Empty ID", urlPath: "/snippet/view/", wantCode: http.StatusNotFound},
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
		form.Add("expires", "7")
		form.Add("csrf_token", extractCSRFToken(t, res.body))

		res = ts.postForm(t, "/snippet/create", form)

		assert.Equal(t, res.status, http.StatusSeeOther)
		assert.Equal(t, res.headers.Get("Location"), "/snippet/view/2")
	})

	t.Run("Invalid snippet", func(t *testing.T) {
		res := ts.get(t, "/snippet/create")

		form := url.Values{}
		form.Add("title", "")
		form.Add("content", "")
		form.Add("expires", "42")
		form.Add("csrf_token", extractCSRFToken(t, res.body))

		res = ts.postForm(t, "/snippet/create", form)

		assert.Equal(t, res.status, http.StatusUnprocessableEntity)
		assert.StringContains(t, res.body, "This field cannot be blank")
		assert.StringContains(t, res.body, "This field must equal 1, 7 or 365")
	})
}
