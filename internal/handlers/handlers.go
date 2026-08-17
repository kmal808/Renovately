package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"reno/internal/session"
	"reno/internal/ui"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

type H struct {
	app core.App
}

func New(app core.App) *H { return &H{app: app} }

func (h *H) Register(r *router.Router[*core.RequestEvent]) {
	r.GET("/", h.wrap(h.home))
	r.GET("/login", h.wrap(h.loginPage))
	r.POST("/login", h.wrap(h.loginSubmit))
	r.GET("/register", h.wrap(h.registerPage))
	r.POST("/register", h.wrap(h.registerSubmit))
	r.POST("/logout", h.wrap(h.logout))

	r.GET("/projects/new", h.wrapAuth(h.projectNewPage))
	r.POST("/projects", h.wrapAuth(h.projectCreate))
	r.GET("/projects/{pid}", h.wrapAuth(h.projectDashboard))
	r.GET("/projects/{pid}/edit", h.wrapAuth(h.projectEditPage))
	r.POST("/projects/{pid}/edit", h.wrapAuth(h.projectUpdate))
	r.POST("/projects/{pid}/delete", h.wrapAuth(h.projectDelete))

	h.registerTasks(r)
	h.registerVendors(r)
	h.registerMaterials(r)
	h.registerBudget(r)
	h.registerFiles(r)
	h.registerM5(r)
}

type handlerFunc func(e *core.RequestEvent) error

// wrap resolves the session user (may be nil) and stashes it on the event.
func (h *H) wrap(next handlerFunc) handlerFunc {
	return func(e *core.RequestEvent) error {
		u := session.FromRequest(h.app, e.Request)
		if u != nil {
			e.Set("user", u)
		}
		return next(e)
	}
}

// wrapAuth requires a logged-in user.
func (h *H) wrapAuth(next handlerFunc) handlerFunc {
	return h.wrap(func(e *core.RequestEvent) error {
		if e.Get("user") == nil {
			return redirect(e, http.StatusSeeOther, "/login")
		}
		return next(e)
	})
}

func user(e *core.RequestEvent) *session.User {
	u, _ := e.Get("user").(*session.User)
	return u
}

func pathID(e *core.RequestEvent) string { return e.Request.PathValue("pid") }

// --- auth ---

func (h *H) home(e *core.RequestEvent) error {
	u := user(e)
	if u == nil {
		return redirect(e, http.StatusSeeOther, "/login")
	}
	projects, err := h.projectsFor(u.ID)
	if err != nil {
		return err
	}
	return renderPage(e, ui.ProjectsList(u, projects))
}

func (h *H) loginPage(e *core.RequestEvent) error {
	return renderPage(e, ui.AuthPage("Log in", "/login", ""))
}

func (h *H) loginSubmit(e *core.RequestEvent) error {
	email := strings.TrimSpace(e.Request.FormValue("email"))
	password := e.Request.FormValue("password")
	token, err := session.Login(h.app, email, password)
	if err != nil {
		return renderPage(e, ui.AuthPage("Log in", "/login", "Invalid email or password."), http.StatusUnauthorized)
	}
	e.SetCookie(session.Cookie(token))
	return redirect(e, http.StatusSeeOther, "/")
}

func (h *H) registerPage(e *core.RequestEvent) error {
	return renderPage(e, ui.AuthPage("Sign up", "/register", ""))
}

func (h *H) registerSubmit(e *core.RequestEvent) error {
	name := strings.TrimSpace(e.Request.FormValue("name"))
	email := strings.TrimSpace(e.Request.FormValue("email"))
	password := e.Request.FormValue("password")
	token, err := session.Register(h.app, name, email, password)
	if err != nil {
		return renderPage(e, ui.AuthPage("Sign up", "/register", "Could not create the account (email may already be in use, or password too short)."), http.StatusBadRequest)
	}
	e.SetCookie(session.Cookie(token))
	return redirect(e, http.StatusSeeOther, "/")
}

func (h *H) logout(e *core.RequestEvent) error {
	e.SetCookie(session.ClearedCookie())
	return redirect(e, http.StatusSeeOther, "/login")
}

// --- projects ---

func (h *H) projectsFor(userID string) ([]ui.Project, error) {
	memberships, err := h.app.FindRecordsByFilter(
		"project_members", "user = {:user}", "-created", 0, 0,
		map[string]any{"user": userID},
	)
	if err != nil {
		return nil, err
	}
	out := make([]ui.Project, 0, len(memberships))
	for _, m := range memberships {
		p, err := h.app.FindRecordById("projects", m.GetString("project"))
		if err != nil {
			continue
		}
		progress, _ := h.projectProgress(p.Id)
		out = append(out, ui.Project{
			ID:       p.Id,
			Name:     p.GetString("name"),
			Type:     p.GetString("type"),
			Status:   p.GetString("status"),
			Address:  p.GetString("property_address"),
			Start:    p.GetString("start_date"),
			Target:   p.GetString("target_date"),
			Budget:   int64(p.GetInt("budget_cents")),
			Role:     m.GetString("role"),
			Progress: progress,
		})
	}
	return out, nil
}

func (h *H) projectProgress(pid string) (int, error) {
	tasks, err := h.app.FindRecordsByFilter("tasks", "project = {:p}", "sort", 0, 0, map[string]any{"p": pid})
	if err != nil {
		return 0, err
	}
	if len(tasks) == 0 {
		return 0, nil
	}
	done := 0
	for _, t := range tasks {
		if t.GetString("status") == "done" {
			done++
		}
	}
	return done * 100 / len(tasks), nil
}

// memberOf returns the user's membership record for the project, or nil.
func (h *H) memberOf(pid, userID string) (*core.Record, error) {
	m, err := h.app.FindFirstRecordByFilter(
		"project_members",
		"project = {:p} && user = {:u}",
		map[string]any{"p": pid, "u": userID},
	)
	if err != nil {
		return nil, nil
	}
	return m, nil
}

func (h *H) loadProject(e *core.RequestEvent) (*core.Record, *core.Record, error) {
	pid := pathID(e)
	u := user(e)
	m, err := h.memberOf(pid, u.ID)
	if err != nil || m == nil {
		return nil, nil, errors.New("project not found")
	}
	p, err := h.app.FindRecordById("projects", pid)
	if err != nil {
		return nil, nil, err
	}
	return p, m, nil
}

func (h *H) projectNewPage(e *core.RequestEvent) error {
	return renderPage(e, ui.ProjectForm(user(e), "", "/projects", map[string]string{"status": "planning", "type": "kitchen"}, ""))
}

func dollarsToCents(s string) int64 {
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f * 100)
}

func (h *H) projectCreate(e *core.RequestEvent) error {
	u := user(e)
	collection, err := h.app.FindCachedCollectionByNameOrId("projects")
	if err != nil {
		return err
	}
	p := core.NewRecord(collection)
	h.applyProjectForm(p, e)

	if err := h.app.Save(p); err != nil {
		return renderPage(e, ui.ProjectForm(u, "", "/projects", formValues(e), "Could not save the project."), http.StatusBadRequest)
	}

	// creator becomes owner
	mc, err := h.app.FindCachedCollectionByNameOrId("project_members")
	if err != nil {
		return err
	}
	m := core.NewRecord(mc)
	m.Set("project", p.Id)
	m.Set("user", u.ID)
	m.Set("role", "owner")
	if err := h.app.Save(m); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id)
}

func (h *H) applyProjectForm(p *core.Record, e *core.RequestEvent) {
	f := e.Request.FormValue
	p.Set("name", strings.TrimSpace(f("name")))
	p.Set("type", f("type"))
	p.Set("status", f("status"))
	p.Set("property_address", strings.TrimSpace(f("property_address")))
	p.Set("start_date", f("start_date"))
	p.Set("target_date", f("target_date"))
	p.Set("budget_cents", dollarsToCents(f("budget")))
	p.Set("description", f("description"))
}

func formValues(e *core.RequestEvent) map[string]string {
	f := e.Request.FormValue
	return map[string]string{
		"name":             f("name"),
		"type":             f("type"),
		"status":           f("status"),
		"property_address": f("property_address"),
		"start_date":       f("start_date"),
		"target_date":      f("target_date"),
		"budget":           f("budget"),
		"description":      f("description"),
	}
}

func (h *H) projectEditPage(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	if m.GetString("role") == "viewer" {
		return errors.New("viewers cannot edit")
	}
	values := map[string]string{
		"name":             p.GetString("name"),
		"type":             p.GetString("type"),
		"status":           p.GetString("status"),
		"property_address": p.GetString("property_address"),
		"start_date":       p.GetString("start_date"),
		"target_date":      p.GetString("target_date"),
		"budget":           fmt.Sprintf("%.2f", float64(p.GetInt("budget_cents"))/100),
		"description":      p.GetString("description"),
	}
	return renderPage(e, ui.ProjectForm(user(e), p.Id, "/projects/"+p.Id+"/edit", values, ""))
}

func (h *H) projectUpdate(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	if m.GetString("role") == "viewer" {
		return errors.New("viewers cannot edit")
	}
	h.applyProjectForm(p, e)
	if err := h.app.Save(p); err != nil {
		return renderPage(e, ui.ProjectForm(user(e), p.Id, "/projects/"+p.Id+"/edit", formValues(e), "Could not save the project."), http.StatusBadRequest)
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id)
}

func (h *H) projectDelete(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	if m.GetString("role") != "owner" {
		return errors.New("only owners can delete")
	}
	if err := h.app.Delete(p); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/")
}

func (h *H) projectDashboard(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	progress, _ := h.projectProgress(p.Id)

	// upcoming / overdue tasks (top 8 by due date)
	tasks, err := h.app.FindRecordsByFilter(
		"tasks", "project = {:p} && status != {:done}", "due_date", 8, 0,
		map[string]any{"p": p.Id, "done": "done"},
	)
	if err != nil {
		return err
	}
	upcoming := make([]ui.TaskRow, 0, len(tasks))
	for _, t := range tasks {
		due := t.GetString("due_date")
		upcoming = append(upcoming, ui.TaskRow{
			ID:      t.Id,
			Title:   t.GetString("title"),
			Due:     due,
			Overdue: isOverdue(due),
			Status:  t.GetString("status"),
		})
	}

	_, totals, _ := h.budgetSummary(p.Id, int64(p.GetInt("budget_cents")))

	view := ui.Project{
		ID:       p.Id,
		Name:     p.GetString("name"),
		Type:     p.GetString("type"),
		Status:   p.GetString("status"),
		Address:  p.GetString("property_address"),
		Start:    p.GetString("start_date"),
		Target:   p.GetString("target_date"),
		Budget:   int64(p.GetInt("budget_cents")),
		Role:     m.GetString("role"),
		Progress: progress,
	}
	return renderPage(e, ui.ProjectDashboard(user(e), view, upcoming, totals))
}
