package handlers

import (
	"errors"
	"net/http"
	"strings"

	"reno/internal/ui"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func (h *H) registerMaterials(r *router.Router[*core.RequestEvent]) {
	r.GET("/projects/{pid}/materials", h.wrapAuth(h.materialsPage))
	r.POST("/projects/{pid}/materials", h.wrapAuth(h.materialCreate))
	r.GET("/materials/{mid}", h.wrapAuth(h.materialEditPage))
	r.POST("/materials/{mid}/update", h.wrapAuth(h.materialUpdate))
	r.POST("/materials/{mid}/delete", h.wrapAuth(h.materialDelete))
}

func (h *H) materialWithProject(e *core.RequestEvent) (*core.Record, *core.Record, *core.Record, error) {
	mr, err := h.app.FindRecordById("materials", e.Request.PathValue("mid"))
	if err != nil {
		return nil, nil, nil, errors.New("material not found")
	}
	p, m, err := h.loadProjectByID(mr.GetString("project"), user(e).ID)
	if err != nil || m == nil {
		return nil, nil, nil, errors.New("material not found")
	}
	return mr, p, m, nil
}

// materialLineTotal: actual cost if set, else qty × unit cost (cents).
func materialLineTotal(m *core.Record) int64 {
	if actual := m.GetInt("actual_cost_cents"); actual > 0 {
		return int64(actual)
	}
	qty := m.GetFloat("quantity")
	unit := m.GetInt("unit_cost_cents")
	return int64(qty * float64(unit))
}

func (h *H) materialRows(pid string) ([]ui.MaterialRow, int64, error) {
	records, err := h.app.FindRecordsByFilter(
		"materials", "project = {:p}", "item", 0, 0, map[string]any{"p": pid})
	if err != nil {
		return nil, 0, err
	}
	rows := make([]ui.MaterialRow, 0, len(records))
	total := int64(0)
	for _, m := range records {
		vendorName := ""
		if vid := m.GetString("vendor"); vid != "" {
			if v, err := h.app.FindRecordById("vendors", vid); err == nil {
				vendorName = v.GetString("company")
			}
		}
		total += materialLineTotal(m)
		rows = append(rows, ui.MaterialRow{
			ID:           m.Id,
			Item:         m.GetString("item"),
			Category:     m.GetString("category"),
			Manufacturer: m.GetString("manufacturer"),
			SKU:          m.GetString("sku"),
			Quantity:     trimFloat(m.GetFloat("quantity")),
			Unit:         m.GetString("unit"),
			UnitCost:     int64(m.GetInt("unit_cost_cents")),
			LineTotal:    materialLineTotal(m),
			Actual:       int64(m.GetInt("actual_cost_cents")),
			Vendor:       vendorName,
			Status:       m.GetString("status"),
			Expected:     m.GetString("expected_delivery"),
			ProductURL:   m.GetString("product_url"),
		})
	}
	return rows, total, nil
}

func trimFloat(f float64) string {
	s := strconvFormat(f)
	return s
}

func (h *H) vendorsFor(pid string) []ui.Vendor {
	records, _ := h.app.FindRecordsByFilter(
		"vendors", "project = {:p}", "company", 0, 0, map[string]any{"p": pid})
	out := make([]ui.Vendor, 0, len(records))
	for _, v := range records {
		out = append(out, vendorView(v))
	}
	return out
}

var materialSortFields = map[string]bool{
	"item": true, "category": true, "quantity": true,
	"unit_cost_cents": true, "status": true, "expected_delivery": true,
}

func (h *H) materialsPage(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	q := strings.TrimSpace(e.Request.URL.Query().Get("q"))
	sortField := e.Request.URL.Query().Get("sort")
	sortDir := e.Request.URL.Query().Get("dir")
	if !materialSortFields[sortField] {
		sortField = "item"
	}
	if sortDir != "desc" {
		sortDir = "asc"
	}

	rows, total, err := h.materialRows(p.Id)
	if err != nil {
		return err
	}
	if q != "" {
		lq := strings.ToLower(q)
		filtered := rows[:0]
		for _, r := range rows {
			hay := strings.ToLower(r.Item + " " + r.Category + " " + r.Manufacturer + " " + r.SKU + " " + r.Vendor)
			if strings.Contains(hay, lq) {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}
	materialsSort(rows, sortField, sortDir == "desc")

	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.MaterialsPage(user(e), pv, rows, h.vendorsFor(p.Id), ui.MaterialSort{Field: sortField, Dir: sortDir, Q: q}, total, ""))
}

func (h *H) materialFormValues(m *core.Record) map[string]string {
	return map[string]string{
		"item":              m.GetString("item"),
		"category":          m.GetString("category"),
		"manufacturer":      m.GetString("manufacturer"),
		"sku":               m.GetString("sku"),
		"quantity":          trimFloat(m.GetFloat("quantity")),
		"unit":              m.GetString("unit"),
		"unit_cost":         centsToDollars(m.GetInt("unit_cost_cents")),
		"actual_cost":       centsToDollars(m.GetInt("actual_cost_cents")),
		"vendor":            m.GetString("vendor"),
		"status":            m.GetString("status"),
		"date_ordered":      m.GetString("date_ordered"),
		"expected_delivery": m.GetString("expected_delivery"),
		"date_received":     m.GetString("date_received"),
		"product_url":       m.GetString("product_url"),
		"notes":             m.GetString("notes"),
	}
}

func (h *H) applyMaterialForm(m *core.Record, e *core.RequestEvent) {
	f := e.Request.FormValue
	m.Set("item", strings.TrimSpace(f("item")))
	m.Set("category", strings.TrimSpace(f("category")))
	m.Set("manufacturer", strings.TrimSpace(f("manufacturer")))
	m.Set("sku", strings.TrimSpace(f("sku")))
	m.Set("quantity", parseFloatOrZero(f("quantity")))
	m.Set("unit", strings.TrimSpace(f("unit")))
	m.Set("unit_cost_cents", int(dollarsToCents(f("unit_cost"))))
	m.Set("actual_cost_cents", int(dollarsToCents(f("actual_cost"))))
	m.Set("vendor", f("vendor"))
	m.Set("status", f("status"))
	m.Set("date_ordered", f("date_ordered"))
	m.Set("expected_delivery", f("expected_delivery"))
	m.Set("date_received", f("date_received"))
	m.Set("product_url", strings.TrimSpace(f("product_url")))
	m.Set("notes", f("notes"))
}

func (h *H) materialCreate(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	collection, err := h.app.FindCachedCollectionByNameOrId("materials")
	if err != nil {
		return err
	}
	rec := core.NewRecord(collection)
	rec.Set("project", p.Id)
	rec.Set("status", "idea")
	h.applyMaterialForm(rec, e)
	if err := h.app.Save(rec); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/materials")
}

func (h *H) materialEditPage(e *core.RequestEvent) error {
	rec, p, m, err := h.materialWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	pv, _ := h.projectView(p, m)
	row := ui.MaterialRow{ID: rec.Id, Item: rec.GetString("item")}
	return renderPage(e, ui.MaterialEdit(user(e), pv, row, h.materialFormValues(rec), h.vendorsFor(p.Id), ""))
}

func (h *H) materialUpdate(e *core.RequestEvent) error {
	rec, p, m, err := h.materialWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	h.applyMaterialForm(rec, e)
	if err := h.app.Save(rec); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/materials")
}

func (h *H) materialDelete(e *core.RequestEvent) error {
	rec, p, m, err := h.materialWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	if err := h.app.Delete(rec); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/materials")
}
