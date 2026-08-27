package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Generic freeform lists (measurements, shopping, ideas…) — the PRD's
// "List / Workstream" level that isn't tied to scheduling or cost tracking.
func init() {
	m.Register(func(app core.App) error {
		lists := core.NewBaseCollection("lists")
		lists.Fields.Add(
			&core.TextField{Name: "title", Max: 150, Required: true},
			&core.BoolField{Name: "pinned"},
			&core.NumberField{Name: "sort"},
		)
		if err := app.Save(lockForApi(lists)); err != nil {
			return err
		}
		// relation needs the saved id, so it is added in a second pass
		lists.Fields.Add(&core.RelationField{Name: "project", CollectionId: projectsCollectionId(app), MaxSelect: 1})
		if err := app.Save(lists); err != nil {
			return err
		}

		items := core.NewBaseCollection("list_items")
		items.Fields.Add(
			&core.RelationField{Name: "list", CollectionId: lists.Id, MaxSelect: 1},
			&core.TextField{Name: "content", Max: 500, Required: true},
			&core.BoolField{Name: "done"},
			&core.NumberField{Name: "sort"},
		)
		return app.Save(lockForApi(items))
	}, func(app core.App) error {
		for _, n := range []string{"list_items", "lists"} {
			if c, err := app.FindCollectionByNameOrId(n); err == nil {
				if err := app.Delete(c); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func projectsCollectionId(app core.App) string {
	if c, err := app.FindCollectionByNameOrId("projects"); err == nil {
		return c.Id
	}
	return ""
}

// lockForApi ensures superuser-only API access and system timestamp fields,
// mirroring the main collections migration.
func lockForApi(c *core.Collection) *core.Collection {
	c.Fields.Add(
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	c.ListRule, c.ViewRule = nil, nil
	c.CreateRule, c.UpdateRule, c.DeleteRule = nil, nil, nil
	return c
}
