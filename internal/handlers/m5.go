package handlers

import (
	"net/http"
	"strings"
	"time"

	"reno/internal/ui"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func (h *H) registerM5(r *router.Router[*core.RequestEvent]) {
	r.GET("/projects/{pid}/timeline", h.wrapAuth(h.timelinePage))
	r.GET("/projects/{pid}/search", h.wrapAuth(h.searchPage))
	r.GET("/projects/{pid}/settings", h.wrapAuth(h.settingsPage))
	r.POST("/projects/{pid}/members", h.wrapAuth(h.memberInvite))
	r.POST("/members/{mid}/role", h.wrapAuth(h.memberRole))
	r.POST("/members/{mid}/remove", h.wrapAuth(h.memberRemove))
}

// ---------- timeline ----------

var ganttEpoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// ganttEnd expands a bare date by one day so single-day bars are visible.
func ganttEnd(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

// normDate trims a PB datetime string to YYYY-MM-DD for Frappe Gantt.
func normDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func (h *H) timelinePage(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	tasks := []ui.GanttTask{}
	hasDates := false

	phases, _ := h.app.FindRecordsByFilter(
		"phases", "project = {:p}", "sort", 0, 0, map[string]any{"p": p.Id})
	for _, ph := range phases {
		start, end := ph.GetString("start_date"), ph.GetString("end_date")
		if start == "" || end == "" {
			continue
		}
		hasDates = true
		tasks = append(tasks, ui.GanttTask{
			ID:          "phase-" + ph.Id,
			Name:        ph.GetString("name"),
			Start:       normDate(start),
			End:         ganttEnd(normDate(end)),
			Progress:    phaseProgress(ph.GetString("status")),
			CustomClass: "gantt-phase",
		})
	}

	taskRecords, _ := h.app.FindRecordsByFilter(
		"tasks", "project = {:p} && parent = ''", "sort", 0, 0, map[string]any{"p": p.Id})
	for _, t := range taskRecords {
		start := t.GetString("start_date")
		end := t.GetString("due_date")
		if start == "" {
			start = end // allow due-only tasks to render as a milestone-ish bar
		}
		if start == "" || end == "" {
			continue
		}
		hasDates = true
		progress := 0.0
		if t.GetString("status") == "done" {
			progress = 100
		}
		deps := []string{}
		for _, d := range toStringSlice(t.Get("depends_on")) {
			deps = append(deps, d)
		}
		tasks = append(tasks, ui.GanttTask{
			ID:           t.Id,
			Name:         t.GetString("title"),
			Start:        normDate(start),
			End:          ganttEnd(normDate(end)),
			Progress:     progress,
			Dependencies: strings.Join(deps, ", "),
		})
	}

	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.TimelinePage(user(e), pv, tasks, hasDates))
}

func phaseProgress(status string) float64 {
	switch status {
	case "done":
		return 100
	case "in_progress":
		return 50
	}
	return 0
}

func toStringSlice(v any) []string {
	switch xs := v.(type) {
	case []string:
		return xs
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// ---------- search ----------

func (h *H) searchPage(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	q := strings.TrimSpace(e.Request.URL.Query().Get("q"))
	results := []ui.SearchResult{}
	if q != "" {
		like := "%" + q + "%"
		params := map[string]any{"p": p.Id, "q": like}

		type searcher struct {
			label string
			coll  string
			kind  string
			filter string
			title func(*core.Record) string
			desc  func(*core.Record) string
			url   func(*core.Record) string
		}
		searchers := []searcher{
			{"Tasks", "tasks", "task", "project = {:p} && title ~ {:q}",
				func(r *core.Record) string { return r.GetString("title") },
				func(r *core.Record) string { return r.GetString("description") },
				func(r *core.Record) string { return "/tasks/" + r.Id }},
			{"Materials", "materials", "material", "project = {:p} && (item ~ {:q} || category ~ {:q} || manufacturer ~ {:q} || sku ~ {:q})",
				func(r *core.Record) string { return r.GetString("item") },
				func(r *core.Record) string { return r.GetString("category") },
				func(r *core.Record) string { return "/materials/" + r.Id }},
			{"Vendors", "vendors", "vendor", "project = {:p} && (company ~ {:q} || contact_name ~ {:q} || trade ~ {:q})",
				func(r *core.Record) string { return r.GetString("company") },
				func(r *core.Record) string { return r.GetString("trade") },
				func(r *core.Record) string { return "/vendors/" + r.Id }},
			{"Notes", "notes", "note", "project = {:p} && body ~ {:q}",
				func(r *core.Record) string { return truncate(r.GetString("body"), 60) },
				func(r *core.Record) string { return "" },
				func(r *core.Record) string { return "/projects/" + p.Id + "/notes" }},
			{"Documents", "documents", "document", "project = {:p} && (title ~ {:q} || description ~ {:q})",
				func(r *core.Record) string { return r.GetString("title") },
				func(r *core.Record) string { return r.GetString("category") },
				func(r *core.Record) string { return "/files/documents/" + r.Id + "/" + r.GetString("file") }},
			{"Photos", "photos", "photo", "project = {:p} && caption ~ {:q}",
				func(r *core.Record) string { return r.GetString("caption") },
				func(r *core.Record) string { return r.GetString("stage") },
				func(r *core.Record) string { return "/files/photos/" + r.Id + "/" + r.GetString("image") }},
		}
		for _, s := range searchers {
			records, err := h.app.FindRecordsByFilter(s.coll, s.filter, "-created", 20, 0, params)
			if err != nil {
				continue
			}
			hits := make([]ui.SearchHit, 0, len(records))
			for _, r := range records {
				hits = append(hits, ui.SearchHit{
					Kind:        s.kind,
					ID:          r.Id,
					Title:       s.title(r),
					Description: s.desc(r),
					URL:         s.url(r),
				})
			}
			if len(hits) > 0 {
				results = append(results, ui.SearchResult{Kind: s.kind, Label: s.label, Hits: hits})
			}
		}
	}
	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.SearchPage(user(e), pv, q, results))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ---------- settings / members ----------

func (h *H) membersFor(pid string) []ui.MemberRow {
	records, _ := h.app.FindRecordsByFilter(
		"project_members", "project = {:p}", "created", 0, 0, map[string]any{"p": pid})
	out := make([]ui.MemberRow, 0, len(records))
	for _, m := range records {
		name, email := m.GetString("user"), ""
		if u, err := h.app.FindRecordById("users", m.GetString("user")); err == nil {
			name, email = u.GetString("name"), u.Email()
		}
		out = append(out, ui.MemberRow{
			ID: m.Id, UserID: m.GetString("user"), Name: name, Email: email, Role: m.GetString("role"),
		})
	}
	return out
}

func (h *H) settingsPage(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.SettingsPage(user(e), pv, h.membersFor(p.Id), user(e).ID, ""))
}

func (h *H) memberWithProject(e *core.RequestEvent) (*core.Record, *core.Record, *core.Record, error) {
	mr, err := h.app.FindRecordById("project_members", e.Request.PathValue("mid"))
	if err != nil {
		return nil, nil, nil, err
	}
	p, m, err := h.loadProjectByID(mr.GetString("project"), user(e).ID)
	if err != nil || m == nil {
		return nil, nil, nil, err
	}
	return mr, p, m, nil
}

func (h *H) memberInvite(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	if m.GetString("role") != "owner" {
		return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/settings")
	}
	email := strings.TrimSpace(e.Request.FormValue("email"))
	invitee, err := h.app.FindAuthRecordByEmail("users", email)
	if err != nil {
		pv, _ := h.projectView(p, m)
		return renderPage(e, ui.SettingsPage(user(e), pv, h.membersFor(p.Id), user(e).ID, "No registered user with that email."), http.StatusBadRequest)
	}
	// already a member?
	if existing, _ := h.memberOf(p.Id, invitee.Id); existing != nil {
		return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/settings")
	}
	collection, err := h.app.FindCachedCollectionByNameOrId("project_members")
	if err != nil {
		return err
	}
	nm := core.NewRecord(collection)
	nm.Set("project", p.Id)
	nm.Set("user", invitee.Id)
	role := e.Request.FormValue("role")
	if role != "editor" {
		role = "viewer"
	}
	nm.Set("role", role)
	if err := h.app.Save(nm); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/settings")
}

func (h *H) memberRole(e *core.RequestEvent) error {
	mr, p, m, err := h.memberWithProject(e)
	if err != nil {
		return err
	}
	if m.GetString("role") != "owner" {
		return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/settings")
	}
	role := e.Request.FormValue("role")
	if role != "owner" && role != "editor" && role != "viewer" {
		role = "viewer"
	}
	mr.Set("role", role)
	if err := h.app.Save(mr); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/settings")
}

func (h *H) memberRemove(e *core.RequestEvent) error {
	mr, p, m, err := h.memberWithProject(e)
	if err != nil {
		return err
	}
	if m.GetString("role") != "owner" {
		return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/settings")
	}
	// an owner cannot remove themselves if they're the only owner
	if mr.GetString("role") == "owner" {
		owners, _ := h.app.FindRecordsByFilter(
			"project_members", "project = {:p} && role = 'owner'", "", 0, 0, map[string]any{"p": p.Id})
		if len(owners) <= 1 {
			return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/settings")
		}
	}
	if err := h.app.Delete(mr); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/settings")
}
