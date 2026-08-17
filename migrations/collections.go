package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// All collections are superuser-only at the API level; every read/write goes
// through this app's handlers, which enforce project membership roles.

func text(name string, opts ...func(*core.TextField)) *core.TextField {
	f := &core.TextField{Name: name}
	for _, o := range opts {
		o(f)
	}
	return f
}

func editor(name string) *core.EditorField { return &core.EditorField{Name: name} }
func num(name string) *core.NumberField     { return &core.NumberField{Name: name} }
func date(name string) *core.DateField     { return &core.DateField{Name: name} }
func jsonf(name string) *core.JSONField    { return &core.JSONField{Name: name} }

func sel(name string, values ...string) *core.SelectField {
	return &core.SelectField{Name: name, MaxSelect: 1, Values: values}
}

func multisel(name string, values ...string) *core.SelectField {
	return &core.SelectField{Name: name, MaxSelect: len(values), Values: values}
}

func rel(name, collectionId string) *core.RelationField {
	return &core.RelationField{Name: name, CollectionId: collectionId, MaxSelect: 1}
}

func file(name string) *core.FileField {
	return &core.FileField{Name: name, MaxSelect: 1, MaxSize: 50 << 20}
}

func image(name string) *core.FileField {
	return &core.FileField{
		Name:      name,
		MaxSelect: 1,
		MaxSize:   25 << 20,
		MimeTypes: []string{"image/jpeg", "image/png", "image/webp", "image/heic", "image/gif"},
	}
}

func init() {
	m.Register(func(app core.App) error {
		ids := map[string]string{}
		save := func(c *core.Collection) error {
			// system timestamp fields (NewBaseCollection only adds id)
			c.Fields.Add(
				&core.AutodateField{Name: "created", OnCreate: true},
				&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
			)
			// superuser-only API rules; app handlers enforce roles
			c.ListRule, c.ViewRule = nil, nil
			c.CreateRule, c.UpdateRule, c.DeleteRule = nil, nil, nil
			if err := app.Save(c); err != nil {
				return err
			}
			ids[c.Name] = c.Id
			return nil
		}

		projects := core.NewBaseCollection("projects")
		projects.Fields.Add(
			text("name", func(f *core.TextField) { f.Required = true; f.Max = 120 }),
			text("property_address", func(f *core.TextField) { f.Max = 250 }),
			multisel("type", "kitchen", "bathroom", "whole_home", "windows", "flooring", "exterior", "addition", "painting", "roofing", "custom"),
			editor("description"),
			sel("status", "planning", "active", "on_hold", "completed", "cancelled"),
			date("start_date"),
			date("target_date"),
			num("budget_cents"),
			editor("notes"),
			image("cover"),
		)
		if err := save(projects); err != nil {
			return err
		}

		members := core.NewBaseCollection("project_members")
		members.Fields.Add(
			rel("project", ids["projects"]),
			rel("user", "_pb_users_auth_"),
			sel("role", "owner", "editor", "viewer"),
		)
		if err := save(members); err != nil {
			return err
		}
		members.AddIndex("idx_member_unique", true, "project, user", "")
		if err := app.Save(members); err != nil {
			return err
		}

		rooms := core.NewBaseCollection("rooms")
		rooms.Fields.Add(
			rel("project", ids["projects"]),
			text("name", func(f *core.TextField) { f.Required = true; f.Max = 80 }),
			sel("type", "interior", "exterior", "other"),
			num("sort"),
		)
		if err := save(rooms); err != nil {
			return err
		}

		phases := core.NewBaseCollection("phases")
		phases.Fields.Add(
			rel("project", ids["projects"]),
			text("name", func(f *core.TextField) { f.Required = true; f.Max = 120 }),
			sel("status", "not_started", "in_progress", "done"),
			date("start_date"),
			date("end_date"),
			num("sort"),
		)
		if err := save(phases); err != nil {
			return err
		}

		vendors := core.NewBaseCollection("vendors")
		vendors.Fields.Add(
			rel("project", ids["projects"]),
			text("company", func(f *core.TextField) { f.Required = true; f.Max = 120 }),
			text("contact_name", func(f *core.TextField) { f.Max = 120 }),
			text("trade", func(f *core.TextField) { f.Max = 80 }),
			text("phone", func(f *core.TextField) { f.Max = 40 }),
			text("email", func(f *core.TextField) { f.Max = 120 }),
			text("website", func(f *core.TextField) { f.Max = 250 }),
			text("address", func(f *core.TextField) { f.Max = 250 }),
			editor("notes"),
		)
		if err := save(vendors); err != nil {
			return err
		}

		tasks := core.NewBaseCollection("tasks")
		tasks.Fields.Add(
			rel("project", ids["projects"]),
			rel("phase", ids["phases"]),
			// "parent" self-reference is added after the first save
			sel("kind", "task", "subtask", "checklist_item"),
			text("title", func(f *core.TextField) { f.Required = true; f.Max = 250 }),
			editor("description"),
			sel("status", "todo", "in_progress", "blocked", "done"),
			sel("priority", "low", "medium", "high", "urgent"),
			date("start_date"),
			date("due_date"),
			date("completed_date"),
			text("assignee", func(f *core.TextField) { f.Max = 120 }),
			jsonf("depends_on"), // list of task ids
			jsonf("tags"),       // list of strings
			editor("notes"),
			num("estimated_cost_cents"),
			num("actual_cost_cents"),
			rel("vendor", ids["vendors"]),
			rel("room", ids["rooms"]),
			file("attachment"),
			num("sort"), // fractional ordering among siblings
		)
		if err := save(tasks); err != nil {
			return err
		}
		// now add the tasks self-reference that needed the saved task id
		tasks.Fields.Add(rel("parent", ids["tasks"]))
		if err := app.Save(tasks); err != nil {
			return err
		}

		materials := core.NewBaseCollection("materials")
		materials.Fields.Add(
			rel("project", ids["projects"]),
			text("item", func(f *core.TextField) { f.Required = true; f.Max = 150 }),
			text("category", func(f *core.TextField) { f.Max = 60 }),
			editor("description"),
			text("manufacturer", func(f *core.TextField) { f.Max = 120 }),
			text("sku", func(f *core.TextField) { f.Max = 80 }),
			num("quantity"),
			text("unit", func(f *core.TextField) { f.Max = 20 }),
			num("unit_cost_cents"),
			num("actual_cost_cents"),
			rel("vendor", ids["vendors"]),
			rel("room", ids["rooms"]),
			rel("phase", ids["phases"]),
			rel("task", ids["tasks"]),
			multisel("status", "idea", "to_order", "ordered", "shipped", "delivered", "installed", "returned"),
			date("date_ordered"),
			date("expected_delivery"),
			date("date_received"),
			text("product_url", func(f *core.TextField) { f.Max = 500 }),
			editor("notes"),
		)
		if err := save(materials); err != nil {
			return err
		}

		budgetItems := core.NewBaseCollection("budget_items")
		budgetItems.Fields.Add(
			rel("project", ids["projects"]),
			text("label", func(f *core.TextField) { f.Required = true; f.Max = 150 }),
			sel("category", "materials", "labor", "permits", "design", "appliances", "fixtures", "electrical", "plumbing", "flooring", "windows_doors", "paint", "misc"),
			num("estimated_cents"),
			num("committed_cents"),
			num("paid_cents"),
			rel("vendor", ids["vendors"]),
			editor("notes"),
		)
		if err := save(budgetItems); err != nil {
			return err
		}

		photos := core.NewBaseCollection("photos")
		photos.Fields.Add(
			rel("project", ids["projects"]),
			image("image"),
			text("caption", func(f *core.TextField) { f.Max = 250 }),
			sel("stage", "before", "during", "after", "damage", "measurement", "inspiration", "product", "progress"),
			text("location", func(f *core.TextField) { f.Max = 80 }),
			jsonf("tags"),
			rel("room", ids["rooms"]),
			rel("phase", ids["phases"]),
			rel("task", ids["tasks"]),
			date("taken_date"),
		)
		if err := save(photos); err != nil {
			return err
		}

		documents := core.NewBaseCollection("documents")
		documents.Fields.Add(
			rel("project", ids["projects"]),
			file("file"),
			text("title", func(f *core.TextField) { f.Max = 150 }),
			sel("category", "contract", "estimate", "receipt", "invoice", "permit", "drawing", "brochure", "spec", "warranty", "installation", "other"),
			editor("description"),
			rel("vendor", ids["vendors"]),
			rel("phase", ids["phases"]),
			rel("task", ids["tasks"]),
			jsonf("tags"),
			date("doc_date"),
		)
		if err := save(documents); err != nil {
			return err
		}

		notes := core.NewBaseCollection("notes")
		notes.Fields.Add(
			rel("project", ids["projects"]),
			editor("body"),
			rel("task", ids["tasks"]),
			rel("room", ids["rooms"]),
			&core.BoolField{Name: "pinned"},
		)
		if err := save(notes); err != nil {
			return err
		}

		decisions := core.NewBaseCollection("decisions")
		decisions.Fields.Add(
			rel("project", ids["projects"]),
			text("title", func(f *core.TextField) { f.Required = true; f.Max = 200 }),
			jsonf("options"), // list of strings
			text("selected", func(f *core.TextField) { f.Max = 120 }),
			date("decided_date"),
			editor("notes"),
			rel("room", ids["rooms"]),
		)
		if err := save(decisions); err != nil {
			return err
		}

		return nil
	}, func(app core.App) error {
		names := []string{"decisions", "notes", "documents", "photos", "budget_items", "tasks", "materials", "vendors", "phases", "rooms", "project_members", "projects"}
		for _, n := range names {
			if c, err := app.FindCollectionByNameOrId(n); err == nil {
				if err := app.Delete(c); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
