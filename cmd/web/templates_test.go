package main

import (
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/satyaorz/snippetbox/internal/assert"
)

func TestHumanDate(t *testing.T) {
	tests := []struct {
		name string
		tm   time.Time
		want string
	}{
		{
			name: "UTC",
			tm:   time.Date(2024, 3, 17, 10, 15, 0, 0, time.UTC),
			want: "17 Mar 2024 at 10:15",
		},
		{
			name: "Empty",
			tm:   time.Time{},
			want: "",
		},
		{
			name: "CET",
			tm:   time.Date(2024, 3, 17, 10, 15, 0, 0, time.FixedZone("CET", 1*60*60)),
			want: "17 Mar 2024 at 10:15",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, humanDate(tt.tm), tt.want)
		})
	}
}

func TestNewPagination(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		page     int
		total    int
		wantPrev string
		wantNext string
		wantLast int
	}{
		{name: "No results", page: 1, total: 0, wantLast: 1},
		{name: "Single page", page: 1, total: 10, wantLast: 1},
		{name: "First of three", page: 1, total: 25, wantNext: "/?page=2", wantLast: 3},
		{name: "Middle", page: 2, total: 25, wantPrev: "/", wantNext: "/?page=3", wantLast: 3},
		{name: "Last", page: 3, total: 25, wantPrev: "/?page=2", wantLast: 3},
		{name: "Past the end", page: 9, total: 25, wantPrev: "/?page=3", wantLast: 3},
		{name: "Keeps search", query: "go & rust", page: 2, total: 25, wantPrev: "/?q=go+%26+rust", wantNext: "/?page=3&q=go+%26+rust", wantLast: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPagination(tt.query, tt.page, 10, tt.total)

			assert.Equal(t, p.PrevURL, tt.wantPrev)
			assert.Equal(t, p.NextURL, tt.wantNext)
			assert.Equal(t, p.LastPage, tt.wantLast)
		})
	}
}

func TestHighlight(t *testing.T) {
	t.Run("Escapes HTML", func(t *testing.T) {
		for _, lang := range []string{"", "go", "html", "not-a-language"} {
			out := string(highlight("<script>alert(1)</script>", lang))

			assert.Equal(t, strings.Contains(out, "<script>"), false)
			assert.StringContains(t, out, "&lt;")
		}
	})

	t.Run("Uses CSS classes, not inline styles", func(t *testing.T) {
		out := string(highlight("package main", "go"))

		assert.StringContains(t, out, `class="chroma"`)
		assert.Equal(t, strings.Contains(out, "style="), false)
	})

	t.Run("Every offered language has a lexer", func(t *testing.T) {
		for _, l := range languages {
			if l.Value == "" {
				continue
			}
			if lexers.Get(l.Value) == nil {
				t.Errorf("no chroma lexer for %q", l.Value)
			}
		}
	})
}
