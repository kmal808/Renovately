package handlers

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"reno/internal/ui"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func (h *H) registerTasks(r *router.Router[*core.RequestEvent]) {
	r.GET("/projects/{pid}/tasks", h.wrapAuth(h.tasksPage))
	r.GET("/tasks/{tid}", h.wrapAuth(h.taskDetail))
	r.POST("/tasks/{tid}/update", h.wrapAuth(h.taskUpdate))

	r.POST("/projects/{pid}/phases", h.wrapAuth(h.phaseCreate))
	r.POST("/phases/{phaseID}/status", h.wrapAuth(h.phaseStatus))
	r.POST("/phases/{phaseID}/delete", h.wrapAuth(h.phaseDelete))
	r.POST("/phases/{phaseID}/tasks", h.wrapAuth(h.taskCreate))

	r.POST("/tasks/{tid}/children", h.wrapAuth(h.taskCreateChild))
	r.POST("/tasks/{tid}/toggle", h.wrapAuth(h.taskToggle))
	r.POST("/tasks/{tid}/cycle", h.wrapAuth(h.taskCycle))
	r.POST("/tasks/{tid}/delete", h.wrapAuth(h.taskDelete))
	r.POST("/tasks/{tid}/move", h.wrapAuth(h.taskMove))
}

// --- loading helpers ---

func (h *H) taskWithProject(e *core.RequestEvent) (*core.Record, *core.Record, *core.Record, error) {
	t, err := h.app.FindRecordById("tasks", e.Request.PathValue("tid"))
	if err != nil {
		return nil, nil, nil, errors.New("task not found")
	}
	p, m, err := h.loadProjectByID(t.GetString("project"), user(e).ID)
	if err != nil || m == nil {
		return nil, nil, nil, errors.New("task not found")
	}
	return t, p, m, nil
}

// loadProjectByID is loadProject by explicit project id.
func (h *H) loadProjectByID(pid, userID string) (*core.Record, *core.Record, error) {
	m, err := h.memberOf(pid, userID)
	if err != nil || m == nil {
		return nil, nil, errors.New("project not found")
	}
	p, err := h.app.FindRecordById("projects", pid)
	if err != nil {
		return nil, nil, err
	}
	return p, m, nil
}

func (h *H) phaseWithProject(e *core.RequestEvent) (*core.Record, *core.Record, *core.Record, error) {
	ph, err := h.app.FindRecordById("phases", e.Request.PathValue("phaseID"))
	if err != nil {
		return nil, nil, nil, errors.New("phase not found")
	}
	p, m, err := h.loadProjectByID(ph.GetString("project"), user(e).ID)
	if err != nil || m == nil {
		return nil, nil, nil, errors.New("phase not found")
	}
	return ph, p, m, nil
}

func requireEditor(m *core.Record) error {
	if m.GetString("role") == "viewer" {
		return errors.New("viewers cannot make changes")
	}
	return nil
}

// taskTreeFor renders the phases + task tree for a project.
func (h *H) taskTreeFor(pid string) ([]ui.PhaseBlock, error) {
	phases, err := h.app.FindRecordsByFilter(
		"phases", "project = {:p}", "sort", 0, 0, map[string]any{"p": pid})
	if err != nil {
		return nil, err
	}
	tasks, err := h.app.FindRecordsByFilter(
		"tasks", "project = {:p}", "sort", 0, 0, map[string]any{"p": pid})
	if err != nil {
		return nil, err
	}

	childrenOf := map[string][]*core.Record{}
	rootsByPhase := map[string][]*core.Record{}
	noPhase := []*core.Record{}
	for i := range tasks {
		t := tasks[i]
		if parent := t.GetString("parent"); parent != "" {
			childrenOf[parent] = append(childrenOf[parent], t)
		} else if ph := t.GetString("phase"); ph != "" {
			rootsByPhase[ph] = append(rootsByPhase[ph], t)
		} else {
			noPhase = append(noPhase, t)
		}
	}

	var convert func(t *core.Record) ui.TaskTree
	convert = func(t *core.Record) ui.TaskTree {
		due := t.GetString("due_date")
		kids := childrenOf[t.Id]
		out := ui.TaskTree{
			ID:        t.Id,
			ProjectID: t.GetString("project"),
			Title:     t.GetString("title"),
			Status:    t.GetString("status"),
			Priority:  t.GetString("priority"),
			Due:       due,
			Assignee:  t.GetString("assignee"),
			Kind:      t.GetString("kind"),
			Overdue:   t.GetString("status") != "done" && isOverdue(due),
		}
		for _, c := range kids {
			out.Children = append(out.Children, convert(c))
		}
		out.HasChildren = len(out.Children) > 0
		return out
	}

	blocks := make([]ui.PhaseBlock, 0, len(phases)+1)
	for _, ph := range phases {
		block := ui.PhaseBlock{
			ID:     ph.Id,
			Name:   ph.GetString("name"),
			Status: ph.GetString("status"),
		}
		for _, t := range rootsByPhase[ph.Id] {
			block.Tasks = append(block.Tasks, convert(t))
		}
		blocks = append(blocks, block)
	}
	if len(noPhase) > 0 {
		block := ui.PhaseBlock{ID: "", Name: "General", Status: "not_started"}
		for _, t := range noPhase {
			block.Tasks = append(block.Tasks, convert(t))
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func (h *H) projectView(p, m *core.Record) (ui.Project, error) {
	progress, err := h.projectProgress(p.Id)
	if err != nil {
		progress = 0
	}
	return ui.Project{
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
	}, nil
}

// tasksFragment re-renders the #task-list fragment after a mutation.
func (h *H) tasksFragment(e *core.RequestEvent, p, m *core.Record, view string) error {
	blocks, err := h.taskTreeFor(p.Id)
	if err != nil {
		return err
	}
	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.TaskListContent(pv, blocks, view))
}

// --- pages ---

func (h *H) tasksPage(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	view := e.Request.URL.Query().Get("view")
	blocks, err := h.taskTreeFor(p.Id)
	if err != nil {
		return err
	}
	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.TasksPage(user(e), pv, blocks, view))
}

// --- phases ---

func (h *H) phaseCreate(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	name := strings.TrimSpace(e.Request.FormValue("name"))
	if name == "" {
		return h.tasksFragment(e, p, m, "")
	}
	last, _ := h.app.FindRecordsByFilter(
		"phases", "project = {:p}", "-sort", 1, 0, map[string]any{"p": p.Id})
	sort := 0.0
	if len(last) > 0 {
		sort = last[0].GetFloat("sort") + 1
	}
	collection, err := h.app.FindCachedCollectionByNameOrId("phases")
	if err != nil {
		return err
	}
	ph := core.NewRecord(collection)
	ph.Set("project", p.Id)
	ph.Set("name", name)
	ph.Set("status", "not_started")
	ph.Set("sort", sort)
	if err := h.app.Save(ph); err != nil {
		return err
	}
	return h.tasksFragment(e, p, m, "")
}

func (h *H) phaseStatus(e *core.RequestEvent) error {
	ph, p, m, err := h.phaseWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	next := map[string]string{"not_started": "in_progress", "in_progress": "done", "done": "not_started"}[ph.GetString("status")]
	ph.Set("status", next)
	if err := h.app.Save(ph); err != nil {
		return err
	}
	return h.tasksFragment(e, p, m, "")
}

func (h *H) phaseDelete(e *core.RequestEvent) error {
	ph, p, m, err := h.phaseWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	// delete the phase's tasks and their descendants
	tasks, _ := h.app.FindRecordsByFilter(
		"tasks", "project = {:p} && phase = {:ph}", "", 0, 0,
		map[string]any{"p": p.Id, "ph": ph.Id})
	for _, t := range tasks {
		if err := h.deleteTaskCascade(t); err != nil {
			return err
		}
	}
	if err := h.app.Delete(ph); err != nil {
		return err
	}
	return h.tasksFragment(e, p, m, "")
}

// --- tasks ---

func (h *H) taskCreate(e *core.RequestEvent) error {
	ph, p, m, err := h.phaseWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	title := strings.TrimSpace(e.Request.FormValue("title"))
	if title == "" {
		return h.tasksFragment(e, p, m, "")
	}
	if err := h.saveNewTask(p.Id, title, "task", ph.Id, ""); err != nil {
		return err
	}
	return h.tasksFragment(e, p, m, "")
}

func (h *H) taskCreateChild(e *core.RequestEvent) error {
	parent, p, m, err := h.taskWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	title := strings.TrimSpace(e.Request.FormValue("title"))
	if title == "" {
		return h.tasksFragment(e, p, m, "")
	}
	kind := "subtask"
	if parent.GetString("kind") == "subtask" {
		kind = "checklist_item"
	}
	if err := h.saveNewTask(p.Id, title, kind, parent.GetString("phase"), parent.Id); err != nil {
		return err
	}
	return h.tasksFragment(e, p, m, "")
}

// saveNewTask creates a task with fractional sort at the end of its siblings.
func (h *H) saveNewTask(pid, title, kind, phaseID, parentID string) error {
	collection, err := h.app.FindCachedCollectionByNameOrId("tasks")
	if err != nil {
		return err
	}
	filter := "project = {:p} && parent = ''"
	params := map[string]any{"p": pid}
	if parentID != "" {
		filter = "project = {:p} && parent = {:parent}"
		params["parent"] = parentID
	}
	sort := 0.0
	if last, _ := h.app.FindRecordsByFilter("tasks", filter, "-sort", 1, 0, params); len(last) > 0 {
		sort = last[0].GetFloat("sort") + 1
	}
	t := core.NewRecord(collection)
	t.Set("project", pid)
	t.Set("title", title)
	t.Set("kind", kind)
	t.Set("status", "todo")
	if phaseID != "" {
		t.Set("phase", phaseID)
	}
	if parentID != "" {
		t.Set("parent", parentID)
	}
	t.Set("sort", sort)
	return h.app.Save(t)
}

func (h *H) taskToggle(e *core.RequestEvent) error {
	t, p, m, err := h.taskWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	if t.GetString("status") == "done" {
		t.Set("status", "todo")
		t.Set("completed_date", "")
	} else {
		t.Set("status", "done")
		t.Set("completed_date", today())
	}
	if err := h.app.Save(t); err != nil {
		return err
	}
	return h.tasksFragment(e, p, m, "")
}

func (h *H) taskCycle(e *core.RequestEvent) error {
	t, p, m, err := h.taskWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	next := map[string]string{"todo": "in_progress", "in_progress": "blocked", "blocked": "done", "done": "todo"}[t.GetString("status")]
	t.Set("status", next)
	if next == "done" {
		t.Set("completed_date", today())
	} else {
		t.Set("completed_date", "")
	}
	if err := h.app.Save(t); err != nil {
		return err
	}
	return h.tasksFragment(e, p, m, "")
}

func (h *H) deleteTaskCascade(t *core.Record) error {
	children, _ := h.app.FindRecordsByFilter(
		"tasks", "parent = {:p}", "", 0, 0, map[string]any{"p": t.Id})
	for _, c := range children {
		if err := h.deleteTaskCascade(c); err != nil {
			return err
		}
	}
	return h.app.Delete(t)
}

func (h *H) taskDelete(e *core.RequestEvent) error {
	t, p, m, err := h.taskWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	if err := h.deleteTaskCascade(t); err != nil {
		return err
	}
	return h.tasksFragment(e, p, m, "")
}

// taskMove handles drag-drop reordering (list) and status changes (board).
func (h *H) taskMove(e *core.RequestEvent) error {
	t, p, m, err := h.taskWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	f := e.Request.FormValue

	if f("board") == "1" {
		// kanban column drop: change status only
		t.Set("status", f("status"))
		if f("status") == "done" {
			t.Set("completed_date", today())
		} else {
			t.Set("completed_date", "")
		}
		if err := h.app.Save(t); err != nil {
			return err
		}
		return h.tasksFragment(e, p, m, "board")
	}

	// re-parent (dropped into a task's child list or a phase's root list)
	parentID := f("parent")
	phaseID := f("phase")
	t.Set("parent", parentID)
	if phaseID != "" {
		t.Set("phase", phaseID)
	}

	// fractional sort between new neighbours
	prevID, nextID := f("prev"), f("next")
	var prevSort, nextSort = math.Inf(-1), math.Inf(1)
	if prevID != "" {
		if prev, err := h.app.FindRecordById("tasks", prevID); err == nil {
			prevSort = prev.GetFloat("sort")
		}
	}
	if nextID != "" {
		if next, err := h.app.FindRecordById("tasks", nextID); err == nil {
			nextSort = next.GetFloat("sort")
		}
	}
	switch {
	case math.IsInf(prevSort, -1) && math.IsInf(nextSort, 1):
		t.Set("sort", 0.0) // only task in the list
	case math.IsInf(prevSort, -1):
		t.Set("sort", nextSort-1) // moved to the top
	case math.IsInf(nextSort, 1):
		t.Set("sort", prevSort+1) // moved to the bottom
	default:
		t.Set("sort", (prevSort+nextSort)/2)
	}
	if err := h.app.Save(t); err != nil {
		return err
	}
	return h.tasksFragment(e, p, m, "")
}

func today() string {
	return timeNow().Format("2006-01-02")
}

// --- task detail page ---

func (h *H) phaseOptions(pid string) []ui.PhaseOption {
	phases, _ := h.app.FindRecordsByFilter(
		"phases", "project = {:p}", "sort", 0, 0, map[string]any{"p": pid})
	out := make([]ui.PhaseOption, 0, len(phases))
	for _, ph := range phases {
		out = append(out, ui.PhaseOption{ID: ph.Id, Name: ph.GetString("name")})
	}
	return out
}

func taskFormValues(t *core.Record) map[string]string {
	return map[string]string{
		"title":         t.GetString("title"),
		"status":        t.GetString("status"),
		"priority":      t.GetString("priority"),
		"phase":         t.GetString("phase"),
		"start_date":    t.GetString("start_date"),
		"due_date":      t.GetString("due_date"),
		"assignee":      t.GetString("assignee"),
		"estimated_cost": centsToDollars(t.GetInt("estimated_cost_cents")),
		"actual_cost":   centsToDollars(t.GetInt("actual_cost_cents")),
		"description":   t.GetString("description"),
	}
}

func centsToDollars(c int) string {
	return fmt.Sprintf("%.2f", float64(c)/100)
}

func (h *H) taskDetail(e *core.RequestEvent) error {
	t, p, m, err := h.taskWithProject(e)
	if err != nil {
		return err
	}
	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.TaskDetail(user(e), pv, t.Id, taskFormValues(t), h.phaseOptions(p.Id), ""))
}

func (h *H) taskUpdate(e *core.RequestEvent) error {
	t, p, m, err := h.taskWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	f := e.Request.FormValue
	status := f("status")
	t.Set("title", strings.TrimSpace(f("title")))
	t.Set("status", status)
	t.Set("priority", f("priority"))
	t.Set("phase", f("phase"))
	t.Set("start_date", f("start_date"))
	t.Set("due_date", f("due_date"))
	t.Set("assignee", strings.TrimSpace(f("assignee")))
	t.Set("estimated_cost_cents", int(dollarsToCents(f("estimated_cost"))))
	t.Set("actual_cost_cents", int(dollarsToCents(f("actual_cost"))))
	t.Set("description", f("description"))
	if status == "done" && t.GetString("completed_date") == "" {
		t.Set("completed_date", today())
	}
	if status != "done" {
		t.Set("completed_date", "")
	}
	if err := h.app.Save(t); err != nil {
		pv, _ := h.projectView(p, m)
		return renderPage(e, ui.TaskDetail(user(e), pv, t.Id, taskFormValues(t), h.phaseOptions(p.Id), "Could not save the task."), http.StatusBadRequest)
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/tasks")
}
