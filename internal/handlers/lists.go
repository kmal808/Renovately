package handlers

import (
	"errors"
	"math"
	"strings"

	"reno/internal/ui"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func (h *H) registerLists(r *router.Router[*core.RequestEvent]) {
	r.GET("/projects/{pid}/lists", h.wrapAuth(h.listsPage))
	r.POST("/projects/{pid}/lists", h.wrapAuth(h.listCreate))
	r.POST("/lists/{lid}/rename", h.wrapAuth(h.listRename))
	r.POST("/lists/{lid}/pin", h.wrapAuth(h.listPin))
	r.POST("/lists/{lid}/duplicate", h.wrapAuth(h.listDuplicate))
	r.POST("/lists/{lid}/delete", h.wrapAuth(h.listDelete))
	r.POST("/lists/{lid}/items", h.wrapAuth(h.itemCreate))
	r.POST("/list-items/{iid}/toggle", h.wrapAuth(h.itemToggle))
	r.POST("/list-items/{iid}/delete", h.wrapAuth(h.itemDelete))
	r.POST("/list-items/{iid}/move", h.wrapAuth(h.itemMove))
}

// listWithProject loads a list and verifies the caller's project membership.
func (h *H) listWithProject(e *core.RequestEvent) (*core.Record, *core.Record, *core.Record, error) {
	l, err := h.app.FindRecordById("lists", e.Request.PathValue("lid"))
	if err != nil {
		return nil, nil, nil, errors.New("list not found")
	}
	p, m, err := h.loadProjectByID(l.GetString("project"), user(e).ID)
	if err != nil || m == nil {
		return nil, nil, nil, errors.New("list not found")
	}
	return l, p, m, nil
}

func (h *H) itemWithProject(e *core.RequestEvent) (*core.Record, *core.Record, *core.Record, error) {
	it, err := h.app.FindRecordById("list_items", e.Request.PathValue("iid"))
	if err != nil {
		return nil, nil, nil, errors.New("item not found")
	}
	l, err := h.app.FindRecordById("lists", it.GetString("list"))
	if err != nil {
		return nil, nil, nil, errors.New("list not found")
	}
	p, m, err := h.loadProjectByID(l.GetString("project"), user(e).ID)
	if err != nil || m == nil {
		return nil, nil, nil, errors.New("item not found")
	}
	return it, p, m, nil
}

func (h *H) listsFor(pid string) []ui.ListView {
	listRecords, _ := h.app.FindRecordsByFilter(
		"lists", "project = {:p}", "-pinned,sort", 0, 0, map[string]any{"p": pid})
	itemsByList := map[string][]ui.ListItem{}
	for _, l := range listRecords {
		itemRecords, _ := h.app.FindRecordsByFilter(
			"list_items", "list = {:l}", "sort", 0, 0, map[string]any{"l": l.Id})
		for _, it := range itemRecords {
			itemsByList[l.Id] = append(itemsByList[l.Id], ui.ListItem{
				ID:      it.Id,
				Content: it.GetString("content"),
				Done:    it.GetBool("done"),
			})
		}
	}

	out := make([]ui.ListView, 0, len(listRecords))
	for _, l := range listRecords {
		out = append(out, ui.ListView{
			ID:     l.Id,
			Title:  l.GetString("title"),
			Pinned: l.GetBool("pinned"),
			Items:  itemsByList[l.Id],
		})
	}
	return out
}

func (h *H) listsFragment(e *core.RequestEvent, p, m *core.Record) error {
	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.ListsContent(pv, h.listsFor(p.Id)))
}

func (h *H) listsPage(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.ListsPage(user(e), pv, h.listsFor(p.Id)))
}

func (h *H) listCreate(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	title := strings.TrimSpace(e.Request.FormValue("title"))
	if title == "" {
		return h.listsFragment(e, p, m)
	}
	last, _ := h.app.FindRecordsByFilter(
		"lists", "project = {:p}", "-sort", 1, 0, map[string]any{"p": p.Id})
	sort := 0.0
	if len(last) > 0 {
		sort = last[0].GetFloat("sort") + 1
	}
	collection, err := h.app.FindCachedCollectionByNameOrId("lists")
	if err != nil {
		return err
	}
	l := core.NewRecord(collection)
	l.Set("project", p.Id)
	l.Set("title", title)
	l.Set("sort", sort)
	if err := h.app.Save(l); err != nil {
		return err
	}
	return h.listsFragment(e, p, m)
}

func (h *H) listRename(e *core.RequestEvent) error {
	l, p, m, err := h.listWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	if title := strings.TrimSpace(e.Request.FormValue("title")); title != "" {
		l.Set("title", title)
		if err := h.app.Save(l); err != nil {
			return err
		}
	}
	return h.listsFragment(e, p, m)
}

func (h *H) listPin(e *core.RequestEvent) error {
	l, p, m, err := h.listWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	l.Set("pinned", !l.GetBool("pinned"))
	if err := h.app.Save(l); err != nil {
		return err
	}
	return h.listsFragment(e, p, m)
}

// listDuplicate copies a list and all its items — per the PRD: "Lists should
// be easy to create, reorder, nest, and duplicate."
func (h *H) listDuplicate(e *core.RequestEvent) error {
	l, p, m, err := h.listWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	last, _ := h.app.FindRecordsByFilter(
		"lists", "project = {:p}", "-sort", 1, 0, map[string]any{"p": p.Id})
	sort := 0.0
	if len(last) > 0 {
		sort = last[0].GetFloat("sort") + 1
	}
	lc, _ := h.app.FindCachedCollectionByNameOrId("lists")
	copyList := core.NewRecord(lc)
	copyList.Set("project", p.Id)
	copyList.Set("title", l.GetString("title")+" (copy)")
	copyList.Set("sort", sort)
	if err := h.app.Save(copyList); err != nil {
		return err
	}
	items, _ := h.app.FindRecordsByFilter(
		"list_items", "list = {:l}", "sort", 0, 0, map[string]any{"l": l.Id})
	ic, _ := h.app.FindCachedCollectionByNameOrId("list_items")
	for _, it := range items {
		c := core.NewRecord(ic)
		c.Set("list", copyList.Id)
		c.Set("content", it.GetString("content"))
		c.Set("done", false) // duplicates start unchecked
		c.Set("sort", it.GetFloat("sort"))
		if err := h.app.Save(c); err != nil {
			return err
		}
	}
	return h.listsFragment(e, p, m)
}

func (h *H) listDelete(e *core.RequestEvent) error {
	l, p, m, err := h.listWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	items, _ := h.app.FindRecordsByFilter(
		"list_items", "list = {:l}", "", 0, 0, map[string]any{"l": l.Id})
	for _, it := range items {
		if err := h.app.Delete(it); err != nil {
			return err
		}
	}
	if err := h.app.Delete(l); err != nil {
		return err
	}
	return h.listsFragment(e, p, m)
}

func (h *H) itemCreate(e *core.RequestEvent) error {
	l, p, m, err := h.listWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	content := strings.TrimSpace(e.Request.FormValue("content"))
	if content == "" {
		return h.listsFragment(e, p, m)
	}
	last, _ := h.app.FindRecordsByFilter(
		"list_items", "list = {:l}", "-sort", 1, 0, map[string]any{"l": l.Id})
	sort := 0.0
	if len(last) > 0 {
		sort = last[0].GetFloat("sort") + 1
	}
	collection, err := h.app.FindCachedCollectionByNameOrId("list_items")
	if err != nil {
		return err
	}
	it := core.NewRecord(collection)
	it.Set("list", l.Id)
	it.Set("content", content)
	it.Set("sort", sort)
	if err := h.app.Save(it); err != nil {
		return err
	}
	return h.listsFragment(e, p, m)
}

func (h *H) itemToggle(e *core.RequestEvent) error {
	it, p, m, err := h.itemWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	it.Set("done", !it.GetBool("done"))
	if err := h.app.Save(it); err != nil {
		return err
	}
	return h.listsFragment(e, p, m)
}

func (h *H) itemDelete(e *core.RequestEvent) error {
	it, p, m, err := h.itemWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	if err := h.app.Delete(it); err != nil {
		return err
	}
	return h.listsFragment(e, p, m)
}

func (h *H) itemMove(e *core.RequestEvent) error {
	it, p, m, err := h.itemWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	f := e.Request.FormValue
	prevID, nextID := f("prev"), f("next")
	var prevSort, nextSort = math.Inf(-1), math.Inf(1)
	if prevID != "" {
		if prev, err := h.app.FindRecordById("list_items", prevID); err == nil {
			prevSort = prev.GetFloat("sort")
		}
	}
	if nextID != "" {
		if next, err := h.app.FindRecordById("list_items", nextID); err == nil {
			nextSort = next.GetFloat("sort")
		}
	}
	switch {
	case math.IsInf(prevSort, -1) && math.IsInf(nextSort, 1):
		it.Set("sort", 0.0)
	case math.IsInf(prevSort, -1):
		it.Set("sort", nextSort-1)
	case math.IsInf(nextSort, 1):
		it.Set("sort", prevSort+1)
	default:
		it.Set("sort", (prevSort+nextSort)/2)
	}
	if err := h.app.Save(it); err != nil {
		return err
	}
	return h.listsFragment(e, p, m)
}
