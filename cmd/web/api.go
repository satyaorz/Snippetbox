package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/satyaorz/snippetbox/internal/lang"
	"github.com/satyaorz/snippetbox/internal/models"
	"github.com/satyaorz/snippetbox/internal/validator"
)

// API tokens stay valid for 30 days, or until revoked.
const apiTokenTTL = 30 * 24 * time.Hour

// apiSnippet is the JSON representation of a snippet.
type apiSnippet struct {
	Slug       string    `json:"slug"`
	Title      string    `json:"title"`
	Content    string    `json:"content,omitempty"`
	Language   string    `json:"language"`
	Visibility string    `json:"visibility"`
	Author     string    `json:"author,omitempty"`
	Created    time.Time `json:"created"`
	Expires    time.Time `json:"expires"`
	URL        string    `json:"url"`
	RawURL     string    `json:"raw_url"`
}

// toAPISnippet converts a snippet for a response. Lists leave out the content
// to keep responses small; fetch a single snippet to get it.
func (app *application) toAPISnippet(r *http.Request, s models.Snippet, withContent bool) apiSnippet {
	base := app.baseURL(r)

	out := apiSnippet{
		Slug:       s.Slug,
		Title:      s.Title,
		Language:   s.Language,
		Visibility: s.Visibility,
		Author:     s.AuthorName,
		Created:    s.Created,
		Expires:    s.Expires,
		URL:        base + "/snippet/view/" + s.Slug,
		RawURL:     base + "/snippet/raw/" + s.Slug,
	}
	if withContent {
		out.Content = s.Content
	}
	return out
}

func (app *application) toAPISnippets(r *http.Request, snippets []models.Snippet) []apiSnippet {
	out := make([]apiSnippet, len(snippets))
	for i, s := range snippets {
		out[i] = app.toAPISnippet(r, s, false)
	}
	return out
}

// apiTokenContextKey holds the bearer token used for the request, so the
// "revoke current token" endpoint knows which one to delete.
const apiTokenContextKey = contextKey("apiToken")

// authenticateToken checks for an "Authorization: Bearer <token>" header.
// With no header the request carries on anonymously; an invalid or expired
// token is rejected outright so clients notice straight away. A valid token
// sets the same authenticated user ID as a logged-in browser session, so
// ownership checks work identically for both.
func (app *application) authenticateToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Responses differ depending on this header, so caches must too.
		w.Header().Add("Vary", "Authorization")

		header := r.Header.Get("Authorization")
		if header == "" {
			next.ServeHTTP(w, r)
			return
		}

		scheme, token, ok := strings.Cut(header, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			app.invalidTokenResponse(w, r)
			return
		}

		userID, err := app.tokens.UserID(strings.TrimSpace(token))
		if err != nil {
			if errors.Is(err, models.ErrInvalidCredentials) {
				app.invalidTokenResponse(w, r)
			} else {
				app.serverError(w, r, err)
			}
			return
		}

		ctx := context.WithValue(r.Context(), authenticatedUserIDContextKey, userID)
		ctx = context.WithValue(ctx, apiTokenContextKey, strings.TrimSpace(token))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (app *application) invalidTokenResponse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
	app.apiError(w, r, http.StatusUnauthorized, "invalid or expired API token")
}

// requireToken rejects anonymous API requests.
func (app *application) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.isAuthenticated(r) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			app.apiError(w, r, http.StatusUnauthorized, "this endpoint needs an API token: POST /api/v1/tokens to get one")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// apiSnippetError sends the right response for an error from the snippet
// lookups and model.
func (app *application) apiSnippetError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, models.ErrNoRecord) {
		app.apiError(w, r, http.StatusNotFound, "snippet not found")
		return
	}
	app.serverError(w, r, err)
}

func (app *application) apiLanguages(w http.ResponseWriter, r *http.Request) {
	type language struct {
		Value string `json:"value"`
		Label string `json:"label"`
	}

	languages := make([]language, len(lang.All))
	for i, l := range lang.All {
		languages[i] = language{l.Value, l.Label}
	}

	app.writeJSON(w, http.StatusOK, envelope{"languages": languages}, nil)
}

// apiCreateToken exchanges an email address and password for an API token.
func (app *application) apiCreateToken(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.apiError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	var v validator.Validator
	v.CheckField(validator.NotBlank(input.Email), "email", "must be provided")
	v.CheckField(validator.NotBlank(input.Password), "password", "must be provided")
	v.CheckField(validator.MaxChars(input.Name, 100), "name", "must not be more than 100 characters")
	if !v.Valid() {
		app.apiValidationError(w, r, v.FieldErrors)
		return
	}

	userID, err := app.users.Authenticate(input.Email, input.Password)
	if err != nil {
		if errors.Is(err, models.ErrInvalidCredentials) {
			app.apiError(w, r, http.StatusUnauthorized, "invalid email or password")
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	token, expires, err := app.tokens.New(userID, input.Name, apiTokenTTL)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.writeJSON(w, http.StatusCreated, envelope{"token": token, "expires": expires}, nil)
}

// apiRevokeToken deletes the token used to make the request.
func (app *application) apiRevokeToken(w http.ResponseWriter, r *http.Request) {
	token, _ := r.Context().Value(apiTokenContextKey).(string)

	err := app.tokens.Delete(token)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) apiMe(w http.ResponseWriter, r *http.Request) {
	user, err := app.users.Get(app.authenticatedUserID(r))
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.writeJSON(w, http.StatusOK, envelope{"user": map[string]any{
		"id":      user.ID,
		"name":    user.Name,
		"email":   user.Email,
		"created": user.Created,
	}}, nil)
}

func (app *application) apiMySnippets(w http.ResponseWriter, r *http.Request) {
	snippets, err := app.snippets.ListByUser(app.authenticatedUserID(r))
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.writeJSON(w, http.StatusOK, envelope{"snippets": app.toAPISnippets(r, snippets)}, nil)
}

// apiListSnippets lists public snippets: ?q= searches, ?page= and
// ?page_size= (1-100, default 20) paginate.
func (app *application) apiListSnippets(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	var v validator.Validator

	page, pageSize := 1, 20
	if s := qs.Get("page"); s != "" {
		n, err := strconv.Atoi(s)
		v.CheckField(err == nil && n >= 1 && n <= 10_000, "page", "must be a number between 1 and 10000")
		page = n
	}
	if s := qs.Get("page_size"); s != "" {
		n, err := strconv.Atoi(s)
		v.CheckField(err == nil && n >= 1 && n <= 100, "page_size", "must be a number between 1 and 100")
		pageSize = n
	}
	query := strings.TrimSpace(qs.Get("q"))
	v.CheckField(validator.MaxChars(query, 100), "q", "must not be more than 100 characters")

	if !v.Valid() {
		app.apiValidationError(w, r, v.FieldErrors)
		return
	}

	snippets, total, err := app.snippets.List(query, page, pageSize)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.writeJSON(w, http.StatusOK, envelope{
		"snippets": app.toAPISnippets(r, snippets),
		"metadata": map[string]int{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
			"last_page": max(1, (total+pageSize-1)/pageSize),
		},
	}, nil)
}

func (app *application) apiShowSnippet(w http.ResponseWriter, r *http.Request) {
	snippet, err := app.findViewableSnippet(r, r.PathValue("slug"))
	if err != nil {
		app.apiSnippetError(w, r, err)
		return
	}

	app.writeJSON(w, http.StatusOK, envelope{"snippet": app.toAPISnippet(r, snippet, true)}, nil)
}

// apiCreateSnippet creates a snippet. Only title and content are required:
// language defaults to plain text, visibility to public and expiry to 365
// days, matching the web form.
func (app *application) apiCreateSnippet(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title         string `json:"title"`
		Content       string `json:"content"`
		Language      string `json:"language"`
		Visibility    string `json:"visibility"`
		ExpiresInDays int    `json:"expires_in_days"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.apiError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	form := snippetForm{
		Title:      input.Title,
		Content:    input.Content,
		Language:   input.Language,
		Visibility: cmp.Or(input.Visibility, models.VisibilityPublic),
		Expires:    cmp.Or(input.ExpiresInDays, 365),
	}
	form.validate()
	form.CheckField(validator.PermittedValue(form.Expires, 1, 7, 365), "expires_in_days", "must be 1, 7 or 365")
	if !form.Valid() {
		app.apiValidationError(w, r, form.FieldErrors)
		return
	}

	slug, err := app.snippets.Insert(app.authenticatedUserID(r), form.Title, form.Content, form.Language, form.Visibility, form.Expires)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	snippet, err := app.snippets.Get(slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	headers := http.Header{"Location": {fmt.Sprintf("/api/v1/snippets/%s", slug)}}
	app.writeJSON(w, http.StatusCreated, envelope{"snippet": app.toAPISnippet(r, snippet, true)}, headers)
}

// apiUpdateSnippet changes any of title, content, language and visibility.
// Fields left out of the JSON body keep their current values.
func (app *application) apiUpdateSnippet(w http.ResponseWriter, r *http.Request) {
	snippet, err := app.findOwnedSnippet(r, r.PathValue("slug"))
	if err != nil {
		app.apiSnippetError(w, r, err)
		return
	}

	// Pointers let us tell "not provided" (nil) apart from "set to empty".
	var input struct {
		Title      *string `json:"title"`
		Content    *string `json:"content"`
		Language   *string `json:"language"`
		Visibility *string `json:"visibility"`
	}

	err = app.readJSON(w, r, &input)
	if err != nil {
		app.apiError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	form := snippetForm{
		Title:      snippet.Title,
		Content:    snippet.Content,
		Language:   snippet.Language,
		Visibility: snippet.Visibility,
	}
	if input.Title != nil {
		form.Title = *input.Title
	}
	if input.Content != nil {
		form.Content = *input.Content
	}
	if input.Language != nil {
		form.Language = *input.Language
	}
	if input.Visibility != nil {
		form.Visibility = *input.Visibility
	}

	form.validate()
	if !form.Valid() {
		app.apiValidationError(w, r, form.FieldErrors)
		return
	}

	err = app.snippets.Update(snippet.Slug, app.authenticatedUserID(r), form.Title, form.Content, form.Language, form.Visibility)
	if err != nil {
		app.apiSnippetError(w, r, err)
		return
	}

	snippet.Title, snippet.Content, snippet.Language, snippet.Visibility = form.Title, form.Content, form.Language, form.Visibility
	app.writeJSON(w, http.StatusOK, envelope{"snippet": app.toAPISnippet(r, snippet, true)}, nil)
}

func (app *application) apiDeleteSnippet(w http.ResponseWriter, r *http.Request) {
	snippet, err := app.findOwnedSnippet(r, r.PathValue("slug"))
	if err != nil {
		app.apiSnippetError(w, r, err)
		return
	}

	err = app.snippets.Delete(snippet.Slug, app.authenticatedUserID(r))
	if err != nil {
		app.apiSnippetError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
