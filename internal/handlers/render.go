package handlers

import (
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/pocketbase/pocketbase/core"
)

func redirect(e *core.RequestEvent, code int, url string) error {
	e.Response.Header().Set("Location", url)
	e.Response.WriteHeader(code)
	return nil
}

// renderPage renders a templ component as an HTML response.
func renderPage(e *core.RequestEvent, cmp templ.Component, status ...int) error {
	code := http.StatusOK
	if len(status) > 0 {
		code = status[0]
	}
	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	e.Response.WriteHeader(code)
	return cmp.Render(e.Request.Context(), e.Response)
}

// isOverdue reports whether a YYYY-MM-DD due date is in the past.
func isOverdue(due string) bool {
	if due == "" {
		return false
	}
	t, err := time.Parse("2006-01-02", due)
	if err != nil {
		return false
	}
	return t.Before(time.Now().Truncate(24 * time.Hour))
}

func timeNow() time.Time { return time.Now() }
