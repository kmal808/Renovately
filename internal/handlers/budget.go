package handlers

import (
	"net/http"
	"sort"
	"strings"

	"reno/internal/ui"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func (h *H) registerBudget(r *router.Router[*core.RequestEvent]) {
	r.GET("/projects/{pid}/budget", h.wrapAuth(h.budgetPage))
	r.POST("/projects/{pid}/budget", h.wrapAuth(h.budgetLineCreate))
	r.POST("/budget-items/{bid}/delete", h.wrapAuth(h.budgetLineDelete))
}

// budgetSummary computes per-category + total rollups.
// Assumption: "actual" = paid budget lines + material line totals + task actual costs.
func (h *H) budgetSummary(pid string, budgetCents int64) ([]ui.BudgetCategory, ui.BudgetTotals, error) {
	cats := map[string]*ui.BudgetCategory{}

	get := func(name string) *ui.BudgetCategory {
		c, ok := cats[name]
		if !ok {
			c = &ui.BudgetCategory{Name: name}
			cats[name] = c
		}
		return c
	}

	lines, err := h.app.FindRecordsByFilter(
		"budget_items", "project = {:p}", "label", 0, 0, map[string]any{"p": pid})
	if err != nil {
		return nil, ui.BudgetTotals{}, err
	}
	for _, l := range lines {
		c := get(l.GetString("category"))
		c.Estimated += int64(l.GetInt("estimated_cents"))
		c.Committed += int64(l.GetInt("committed_cents"))
		c.Paid += int64(l.GetInt("paid_cents"))
	}

	materials, err := h.app.FindRecordsByFilter(
		"materials", "project = {:p}", "", 0, 0, map[string]any{"p": pid})
	if err != nil {
		return nil, ui.BudgetTotals{}, err
	}
	for _, m := range materials {
		c := get(materialCategory(m.GetString("category")))
		c.Materials += materialLineTotal(m)
	}

	tasks, err := h.app.FindRecordsByFilter(
		"tasks", "project = {:p}", "", 0, 0, map[string]any{"p": pid})
	if err != nil {
		return nil, ui.BudgetTotals{}, err
	}
	for _, t := range tasks {
		if cost := int64(t.GetInt("actual_cost_cents")); cost > 0 {
			c := get("labor")
			c.Tasks += cost
		}
	}

	out := make([]ui.BudgetCategory, 0, len(cats))
	actual := int64(0)
	for _, c := range cats {
		c.Actual = c.Paid + c.Materials + c.Tasks
		actual += c.Actual
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, ui.BudgetTotals{
		OriginalBudget: budgetCents,
		Actual:         actual,
		Remaining:      budgetCents - actual,
		Over:           budgetCents > 0 && actual > budgetCents,
	}, nil
}

// materialCategory loosely maps a free-text material category to a budget category.
func materialCategory(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case s == "":
		return "materials"
	case strings.Contains(s, "floor"):
		return "flooring"
	case strings.Contains(s, "window"), strings.Contains(s, "door"):
		return "windows_doors"
	case strings.Contains(s, "electric"), strings.Contains(s, "light"):
		return "electrical"
	case strings.Contains(s, "plumb"), strings.Contains(s, "pipe"), strings.Contains(s, "fixture"):
		return "plumbing"
	case strings.Contains(s, "paint"):
		return "paint"
	case strings.Contains(s, "appliance"):
		return "appliances"
	default:
		return "materials"
	}
}

func (h *H) budgetPage(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	cats, totals, err := h.budgetSummary(p.Id, int64(p.GetInt("budget_cents")))
	if err != nil {
		return err
	}
	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.BudgetPage(user(e), pv, cats, totals, ""))
}

func (h *H) budgetLineCreate(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	label := strings.TrimSpace(e.Request.FormValue("label"))
	if label == "" {
		return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/budget")
	}
	collection, err := h.app.FindCachedCollectionByNameOrId("budget_items")
	if err != nil {
		return err
	}
	f := e.Request.FormValue
	l := core.NewRecord(collection)
	l.Set("project", p.Id)
	l.Set("label", label)
	l.Set("category", f("category"))
	l.Set("estimated_cents", int(dollarsToCents(f("estimated"))))
	l.Set("committed_cents", int(dollarsToCents(f("committed"))))
	l.Set("paid_cents", int(dollarsToCents(f("paid"))))
	if err := h.app.Save(l); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/budget")
}

func (h *H) budgetLineDelete(e *core.RequestEvent) error {
	l, err := h.app.FindRecordById("budget_items", e.Request.PathValue("bid"))
	if err != nil {
		return err
	}
	p, m, err := h.loadProjectByID(l.GetString("project"), user(e).ID)
	if err != nil || m == nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	if err := h.app.Delete(l); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/budget")
}
