// Package seed creates a demo user and a fully populated kitchen remodel
// project so the app can be explored without manual data entry.
package seed

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func Run(app core.App, email, password string) error {
	// user
	users, err := app.FindCachedCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	u := core.NewRecord(users)
	u.Set("name", "Demo Homeowner")
	u.SetEmail(email)
	u.SetPassword(password)
	if err := app.Save(u); err != nil {
		return fmt.Errorf("creating user: %w", err)
	}

	// project
	projects, _ := app.FindCachedCollectionByNameOrId("projects")
	p := core.NewRecord(projects)
	p.Set("name", "Kitchen Remodel")
	p.Set("type", "kitchen")
	p.Set("status", "active")
	p.Set("property_address", "742 Maple Street")
	p.Set("budget_cents", 4500000)
	p.Set("description", "Full gut remodel: new layout, cabinets, quartz counters, appliances.")
	today := time.Now()
	p.Set("start_date", today.AddDate(0, 0, -14).Format("2006-01-02"))
	p.Set("target_date", today.AddDate(0, 0, 46).Format("2006-01-02"))
	if err := app.Save(p); err != nil {
		return err
	}

	members, _ := app.FindCachedCollectionByNameOrId("project_members")
	m := core.NewRecord(members)
	m.Set("project", p.Id)
	m.Set("user", u.Id)
	m.Set("role", "owner")
	if err := app.Save(m); err != nil {
		return err
	}

	mk := func(coll string) *core.Record {
		c, _ := app.FindCachedCollectionByNameOrId(coll)
		return core.NewRecord(c)
	}

	// rooms
	rooms := map[string]string{}
	for _, r := range []struct{ name, typ string }{{"Kitchen", "interior"}, {"Pantry", "interior"}, {"Exterior rear", "exterior"}} {
		rec := mk("rooms")
		rec.Set("project", p.Id)
		rec.Set("name", r.name)
		rec.Set("type", r.typ)
		app.Save(rec)
		rooms[r.name] = rec.Id
	}

	// vendors
	vendors := map[string]string{}
	for _, v := range []struct {
		company, contact, trade, phone, email string
	}{
		{"Maple Ridge GC", "Dale Cooper", "General contractor", "555-0101", "dale@mapleridge.example"},
		{"Bright Spark Electric", "Ada Volt", "Electrician", "555-0102", "ada@brightspark.example"},
		{"Acme Cabinets", "", "Cabinet supplier", "555-0103", "sales@acmecabinets.example"},
	} {
		rec := mk("vendors")
		rec.Set("project", p.Id)
		rec.Set("company", v.company)
		rec.Set("contact_name", v.contact)
		rec.Set("trade", v.trade)
		rec.Set("phone", v.phone)
		rec.Set("email", v.email)
		app.Save(rec)
		vendors[v.company] = rec.Id
	}

	// phases + tasks
	type taskSpec struct {
		title, status string
		vendor        string
		subtasks      []string
	}
	phaseSpecs := []struct {
		name, status          string
		offsetStart, offsetEnd int
		tasks                 []taskSpec
	}{
		{"Design & permits", "done", -60, -45, []taskSpec{
			{"Finalize layout with architect", "done", "", nil},
			{"Submit permit application", "done", "", []string{"Prepare drawings", "Pay filing fee"}},
			{"Receive building permit", "done", "", nil},
		}},
		{"Demolition", "done", -14, -7, []taskSpec{
			{"Remove cabinets", "done", "", nil},
			{"Remove flooring", "done", "", nil},
			{"Haul away debris", "done", vendors["Maple Ridge GC"], nil},
		}},
		{"Electrical", "in_progress", -3, 10, []taskSpec{
			{"Relocate outlets", "in_progress", vendors["Bright Spark Electric"], nil},
			{"Install recessed lights", "todo", vendors["Bright Spark Electric"], nil},
			{"Under-cabinet lighting rough-in", "todo", "", nil},
		}},
		{"Cabinets & counters", "not_started", 14, 35, []taskSpec{
			{"Order cabinets", "in_progress", vendors["Acme Cabinets"], []string{"Confirm door style", "Confirm finish"}},
			{"Cabinet delivery", "todo", "", nil},
			{"Install cabinets", "todo", "", nil},
			{"Template quartz counters", "todo", "", nil},
		}},
		{"Finishing", "not_started", 36, 45, []taskSpec{
			{"Install flooring", "todo", "", nil},
			{"Paint walls", "todo", "", nil},
			{"Final punch list", "todo", "", nil},
		}},
	}

	phases, _ := app.FindCachedCollectionByNameOrId("phases")
	taskColl, _ := app.FindCachedCollectionByNameOrId("tasks")
	for i, ps := range phaseSpecs {
		ph := core.NewRecord(phases)
		ph.Set("project", p.Id)
		ph.Set("name", ps.name)
		ph.Set("status", ps.status)
		ph.Set("start_date", today.AddDate(0, 0, ps.offsetStart).Format("2006-01-02"))
		ph.Set("end_date", today.AddDate(0, 0, ps.offsetEnd).Format("2006-01-02"))
		ph.Set("sort", i)
		if err := app.Save(ph); err != nil {
			return err
		}
		for j, ts := range ps.tasks {
			t := core.NewRecord(taskColl)
			t.Set("project", p.Id)
			t.Set("phase", ph.Id)
			t.Set("kind", "task")
			t.Set("title", ts.title)
			t.Set("status", ts.status)
			t.Set("priority", "medium")
			if ts.vendor != "" {
				t.Set("vendor", ts.vendor)
			}
			if ts.status == "done" {
				t.Set("completed_date", today.AddDate(0, 0, -8).Format("2006-01-02"))
			}
			t.Set("sort", j)
			if err := app.Save(t); err != nil {
				return err
			}
			for k, sub := range ts.subtasks {
				st := core.NewRecord(taskColl)
				st.Set("project", p.Id)
				st.Set("phase", ph.Id)
				st.Set("parent", t.Id)
				st.Set("kind", "subtask")
				st.Set("title", sub)
				st.Set("status", "todo")
				st.Set("sort", k)
				app.Save(st)
			}
		}
	}

	// materials
	materialSpecs := []struct {
		item, cat, mfr, unit string
		qty, unitCost        float64
		actual               float64
		vendor, status, room string
		expected             int
	}{
		{"White oak flooring", "Flooring", "Nordic Timber", "sq ft", 220, 12.5, 0, "", "ordered", "Kitchen", 12},
		{"Quartz countertop slab", "Countertop", "CaesarStone", "slab", 2, 3200, 3400, "", "delivered", "Kitchen", -2},
		{"Shaker cabinets set", "Cabinets", "Acme", "set", 1, 9800, 0, "Acme Cabinets", "ordered", "Kitchen", 28},
		{"Recessed LED lights", "Lighting", "Philips", "each", 10, 38, 0, "Bright Spark Electric", "to_order", "Kitchen", 0},
	}
	for _, ms := range materialSpecs {
		rec := mk("materials")
		rec.Set("project", p.Id)
		rec.Set("item", ms.item)
		rec.Set("category", ms.cat)
		rec.Set("manufacturer", ms.mfr)
		rec.Set("quantity", ms.qty)
		rec.Set("unit", ms.unit)
		rec.Set("unit_cost_cents", int(ms.unitCost*100))
		if ms.actual > 0 {
			rec.Set("actual_cost_cents", int(ms.actual*100))
		}
		if ms.vendor != "" {
			rec.Set("vendor", vendors[ms.vendor])
		}
		rec.Set("status", ms.status)
		rec.Set("room", rooms[ms.room])
		rec.Set("expected_delivery", today.AddDate(0, 0, ms.expected).Format("2006-01-02"))
		app.Save(rec)
	}

	// budget lines
	for _, b := range []struct {
		label, cat   string
		est, comm, paid float64
	}{
		{"Cabinets & counters", "materials", 16000, 13000, 3400},
		{"Electrical work", "electrical", 3500, 0, 0},
		{"Flooring install", "flooring", 2800, 0, 0},
		{"Building permit", "permits", 600, 600, 600},
		{"GC labor", "labor", 12000, 12000, 5000},
	} {
		rec := mk("budget_items")
		rec.Set("project", p.Id)
		rec.Set("label", b.label)
		rec.Set("category", b.cat)
		rec.Set("estimated_cents", int(b.est*100))
		rec.Set("committed_cents", int(b.comm*100))
		rec.Set("paid_cents", int(b.paid*100))
		app.Save(rec)
	}

	// notes
	for _, n := range []struct {
		body string
		pin  bool
	}{{"Cabinet lead time is 6 weeks — order by Friday or the schedule slips two weeks.", true},
		{"Water shutoff is in the garage, behind the storage shelf.", false}} {
		rec := mk("notes")
		rec.Set("project", p.Id)
		rec.Set("body", n.body)
		rec.Set("pinned", n.pin)
		app.Save(rec)
	}

	// decisions
	for _, d := range []struct {
		title, selected string
		options         []string
		dateOffset      int
	}{{"Countertop material", "Quartz", []string{"Quartz", "Granite", "Porcelain"}, -20},
		{"Flooring direction", "Run long ways", []string{"Run long ways", "Run crossways"}, -18}} {
		rec := mk("decisions")
		rec.Set("project", p.Id)
		rec.Set("title", d.title)
		rec.Set("options", d.options)
		rec.Set("selected", d.selected)
		rec.Set("decided_date", today.AddDate(0, 0, d.dateOffset).Format("2006-01-02"))
		app.Save(rec)
	}

	return nil
}
