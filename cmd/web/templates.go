package main

import (
	"html/template"
	"io/fs"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/satyaorz/snippetbox/internal/lang"
	"github.com/satyaorz/snippetbox/internal/models"
	"github.com/satyaorz/snippetbox/ui"
)

type templateData struct {
	CurrentYear         int
	Snippet             models.Snippet
	Snippets            []models.Snippet
	User                models.User
	Form                any
	Flash               string
	IsAuthenticated     bool
	AuthenticatedUserID int
	CSRFToken           string
	Search              string
	Pagination          pagination
}

// pagination holds the links for moving between pages of a listing.
type pagination struct {
	Page     int
	LastPage int
	PrevURL  string
	NextURL  string
}

// newPagination works out the previous/next links for a listing, keeping any
// search term in the URLs. A link is left empty when there's no such page.
func newPagination(query string, page, pageSize, total int) pagination {
	p := pagination{
		Page:     page,
		LastPage: max(1, (total+pageSize-1)/pageSize),
	}

	pageURL := func(n int) string {
		v := url.Values{}
		if query != "" {
			v.Set("q", query)
		}
		if n > 1 {
			v.Set("page", strconv.Itoa(n))
		}
		if len(v) == 0 {
			return "/"
		}
		return "/?" + v.Encode()
	}

	if page > 1 {
		// If someone asks for a page past the end, "previous" takes them back
		// to the last page that actually has results.
		p.PrevURL = pageURL(min(page-1, p.LastPage))
	}
	if page < p.LastPage {
		p.NextURL = pageURL(page + 1)
	}

	return p
}

func humanDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("02 Jan 2006 at 15:04")
}

var functions = template.FuncMap{
	"humanDate":    humanDate,
	"highlight":    highlight,
	"languages":    func() []lang.Language { return lang.All },
	"languageName": lang.Name,
}

func newTemplateCache() (map[string]*template.Template, error) {
	cache := map[string]*template.Template{}

	// Use fs.Glob() to get a slice of all filepaths in the ui.Files embedded
	// filesystem which match the pattern 'html/pages/*.tmpl.html'.
	pages, err := fs.Glob(ui.Files, "html/pages/*.tmpl.html")
	if err != nil {
		return nil, err
	}

	for _, page := range pages {
		name := filepath.Base(page)

		// Create a slice containing the filepath patterns for the templates we
		// want to parse: the base layout, all partials, then the page itself.
		patterns := []string{
			"html/base.tmpl.html",
			"html/partials/*.tmpl.html",
			page,
		}

		// Use ParseFS() instead of ParseFiles() to parse the template files
		// from the ui.Files embedded filesystem.
		ts, err := template.New(name).Funcs(functions).ParseFS(ui.Files, patterns...)
		if err != nil {
			return nil, err
		}

		cache[name] = ts
	}

	return cache, nil
}
