package handlers

import (
	"errors"
	"net/http"
	"strings"

	"reno/internal/ui"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func (h *H) registerVendors(r *router.Router[*core.RequestEvent]) {
	r.GET("/projects/{pid}/vendors", h.wrapAuth(h.vendorsPage))
	r.POST("/projects/{pid}/vendors", h.wrapAuth(h.vendorCreate))
	r.GET("/vendors/{vid}", h.wrapAuth(h.vendorEditPage))
	r.POST("/vendors/{vid}/update", h.wrapAuth(h.vendorUpdate))
	r.POST("/vendors/{vid}/delete", h.wrapAuth(h.vendorDelete))
}

func (h *H) vendorWithProject(e *core.RequestEvent) (*core.Record, *core.Record, *core.Record, error) {
	v, err := h.app.FindRecordById("vendors", e.Request.PathValue("vid"))
	if err != nil {
		return nil, nil, nil, errors.New("vendor not found")
	}
	p, m, err := h.loadProjectByID(v.GetString("project"), user(e).ID)
	if err != nil || m == nil {
		return nil, nil, nil, errors.New("vendor not found")
	}
	return v, p, m, nil
}

func vendorView(v *core.Record) ui.Vendor {
	return ui.Vendor{
		ID:      v.Id,
		Company: v.GetString("company"),
		Contact: v.GetString("contact_name"),
		Trade:   v.GetString("trade"),
		Phone:   v.GetString("phone"),
		Email:   v.GetString("email"),
		Website: v.GetString("website"),
		Address: v.GetString("address"),
	}
}

func (h *H) vendorsPage(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	records, err := h.app.FindRecordsByFilter(
		"vendors", "project = {:p}", "company", 0, 0, map[string]any{"p": p.Id})
	if err != nil {
		return err
	}
	vendors := make([]ui.Vendor, 0, len(records))
	for _, v := range records {
		vendors = append(vendors, vendorView(v))
	}
	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.VendorsPage(user(e), pv, vendors, ""))
}

func (h *H) applyVendorForm(v *core.Record, e *core.RequestEvent) {
	f := e.Request.FormValue
	v.Set("company", strings.TrimSpace(f("company")))
	v.Set("contact_name", strings.TrimSpace(f("contact_name")))
	v.Set("trade", strings.TrimSpace(f("trade")))
	v.Set("phone", strings.TrimSpace(f("phone")))
	v.Set("email", strings.TrimSpace(f("email")))
	v.Set("website", strings.TrimSpace(f("website")))
	v.Set("address", strings.TrimSpace(f("address")))
	v.Set("notes", f("notes"))
}

func (h *H) vendorCreate(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	collection, err := h.app.FindCachedCollectionByNameOrId("vendors")
	if err != nil {
		return err
	}
	v := core.NewRecord(collection)
	v.Set("project", p.Id)
	h.applyVendorForm(v, e)
	if err := h.app.Save(v); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/vendors")
}

func (h *H) vendorEditPage(e *core.RequestEvent) error {
	v, p, m, err := h.vendorWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.VendorEdit(user(e), pv, vendorView(v)))
}

func (h *H) vendorUpdate(e *core.RequestEvent) error {
	v, p, m, err := h.vendorWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	h.applyVendorForm(v, e)
	if err := h.app.Save(v); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/vendors")
}

func (h *H) vendorDelete(e *core.RequestEvent) error {
	v, p, m, err := h.vendorWithProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	if err := h.app.Delete(v); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/vendors")
}
