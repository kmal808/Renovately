package handlers_test

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"reno/internal/handlers"
	_ "reno/migrations"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// newTestApp boots an app with our migrations applied.
func newTestApp(t *testing.T) *core.BaseApp {
	t.Helper()
	app := core.NewBaseApp(core.BaseAppConfig{
		DataDir:       t.TempDir(),
		EncryptionEnv: "RENO_TEST_SECRET",
	})
	t.Setenv("RENO_TEST_SECRET", "0123456789abcdef0123456789abcdef")
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	if err := app.RunAllMigrations(); err != nil {
		t.Fatal(err)
	}
	return app
}

type testEnv struct {
	app *core.BaseApp
	h   http.Handler
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	app := newTestApp(t)
	r, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	handlers.New(app).Register(r)
	mux, err := r.BuildMux()
	if err != nil {
		t.Fatal(err)
	}
	return &testEnv{app: app, h: mux}
}

// client drives the mux directly through httptest (no TCP, sandbox-safe),
// carrying the session cookie manually.
type client struct {
	t      *testing.T
	h      http.Handler
	cookie string
}

func loginClient(t *testing.T, env *testEnv, email, password string) *client {
	t.Helper()
	c := &client{t: t, h: env.h}
	c.postForm("/login", url.Values{"email": {email}, "password": {password}}, http.StatusSeeOther)
	return c
}

func anonClient(t *testing.T, env *testEnv) *client {
	t.Helper()
	return &client{t: t, h: env.h}
}

func (c *client) do(method, path, body, contentType string) (*httptest.ResponseRecorder, string) {
	c.t.Helper()
	req := httptest.NewRequest(method, "http://test.local"+path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	// track session cookie
	for _, sc := range rec.Header().Values("Set-Cookie") {
		if strings.HasPrefix(sc, "reno_session=") {
			c.cookie = strings.Split(sc, ";")[0]
			if strings.Contains(sc, "Max-Age=0") {
				c.cookie = ""
			}
		}
	}
	b, _ := io.ReadAll(rec.Body)
	return rec, string(b)
}

func (c *client) get(path string, want ...int) string {
	c.t.Helper()
	rec, body := c.do("GET", path, "", "")
	if len(want) > 0 && rec.Code != want[0] {
		c.t.Errorf("GET %s: got %d, want %d", path, rec.Code, want[0])
	}
	return body
}

func (c *client) postForm(path string, form url.Values, want int) {
	c.t.Helper()
	rec, _ := c.do("POST", path, form.Encode(), "application/x-www-form-urlencoded")
	if rec.Code != want {
		c.t.Errorf("POST %s: got %d, want %d", path, rec.Code, want)
	}
}

// postMultipart sends a multipart body built by fn.
func (c *client) postMultipart(path string, build func(*multipart.Writer)) int {
	c.t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	build(w)
	w.Close()
	rec, _ := c.do("POST", path, buf.String(), w.FormDataContentType())
	return rec.Code
}

// ---------- data helpers ----------

func createUser(t *testing.T, app *core.BaseApp, name, email, password string) string {
	t.Helper()
	collection, err := app.FindCachedCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	u := core.NewRecord(collection)
	u.Set("name", name)
	u.SetEmail(email)
	u.SetPassword(password)
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	return u.Id
}

func seedProject(t *testing.T, app *core.BaseApp, ownerUserID string) string {
	t.Helper()
	pc, _ := app.FindCachedCollectionByNameOrId("projects")
	p := core.NewRecord(pc)
	p.Set("name", "Test Kitchen")
	p.Set("type", "kitchen")
	p.Set("status", "active")
	p.Set("budget_cents", 5000000)
	if err := app.Save(p); err != nil {
		t.Fatal(err)
	}
	mc, _ := app.FindCachedCollectionByNameOrId("project_members")
	m := core.NewRecord(mc)
	m.Set("project", p.Id)
	m.Set("user", ownerUserID)
	m.Set("role", "owner")
	if err := app.Save(m); err != nil {
		t.Fatal(err)
	}
	return p.Id
}

func addMember(t *testing.T, app *core.BaseApp, pid, userID, role string) {
	t.Helper()
	mc, _ := app.FindCachedCollectionByNameOrId("project_members")
	m := core.NewRecord(mc)
	m.Set("project", pid)
	m.Set("user", userID)
	m.Set("role", role)
	if err := app.Save(m); err != nil {
		t.Fatal(err)
	}
}

func findRecordID(t *testing.T, app *core.BaseApp, coll string) string {
	t.Helper()
	records, err := app.FindRecordsByFilter(coll, "", "created", 1, 0)
	if err != nil || len(records) == 0 {
		t.Fatalf("no %s record: %v", coll, err)
	}
	return records[0].Id
}

func findRecord(t *testing.T, app *core.BaseApp, coll, id string) *core.Record {
	t.Helper()
	r, err := app.FindRecordById(coll, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func recordIDsByTitle(t *testing.T, app *core.BaseApp) map[string]string {
	t.Helper()
	out := map[string]string{}
	records, err := app.FindRecordsByFilter("tasks", "", "title", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		out[r.GetString("title")] = r.Id
	}
	return out
}

func recordsSorted(t *testing.T, app *core.BaseApp, coll string) []*core.Record {
	t.Helper()
	records, err := app.FindRecordsByFilter(coll, "", "sort", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return records
}

// minimal 1x1 PNG
var testPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49,
	0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02,
	0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44,
	0x41, 0x54, 0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00, 0x00, 0x03, 0x01, 0x01,
	0x00, 0x18, 0xDD, 0x8D, 0xB0, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
	0xAE, 0x42, 0x60, 0x82,
}

// ---------- tests ----------

func TestRegisterLoginLogout(t *testing.T) {
	env := newTestEnv(t)
	c := anonClient(t, env)

	c.postForm("/register", url.Values{"name": {"A"}, "email": {"a@x.com"}, "password": {"password123"}}, http.StatusSeeOther)

	if body := c.get("/", 200); !strings.Contains(body, "Your projects") {
		t.Fatal("home after register should render project list")
	}

	c.postForm("/logout", nil, http.StatusSeeOther)
	c.get("/", http.StatusSeeOther) // anon redirected to login
}

func TestProjectCRUD(t *testing.T) {
	env := newTestEnv(t)
	createUser(t, env.app, "Owner", "o@x.com", "password123")
	owner := loginClient(t, env, "o@x.com", "password123")

	owner.postForm("/projects", url.Values{
		"name": {"Bathroom Remodel"}, "type": {"bathroom"}, "status": {"planning"}, "budget": {"15000.50"},
	}, http.StatusSeeOther)

	if body := owner.get("/", 200); !strings.Contains(body, "Bathroom Remodel") {
		t.Fatal("project missing from list")
	}

	pid := findRecordID(t, env.app, "projects")
	if body := owner.get("/projects/"+pid, 200); !strings.Contains(body, "$15000.50") {
		t.Fatal("budget not rendered on dashboard")
	}

	owner.postForm("/projects/"+pid+"/edit", url.Values{
		"name": {"Bathroom Remodel"}, "type": {"bathroom"}, "status": {"active"}, "budget": {"15000.50"},
	}, http.StatusSeeOther)

	owner.postForm("/projects/"+pid+"/delete", nil, http.StatusSeeOther)
	if records, _ := env.app.FindRecordsByFilter("projects", "", "", 0, 0); len(records) != 0 {
		t.Fatal("project not deleted")
	}
}

func TestRoleEnforcement(t *testing.T) {
	env := newTestEnv(t)
	ownerID := createUser(t, env.app, "Owen", "o@x.com", "password123")
	editorID := createUser(t, env.app, "Ed", "e@x.com", "password123")
	viewerID := createUser(t, env.app, "Vera", "v@x.com", "password123")
	createUser(t, env.app, "Out", "z@x.com", "password123")

	pid := seedProject(t, env.app, ownerID)
	addMember(t, env.app, pid, editorID, "editor")
	addMember(t, env.app, pid, viewerID, "viewer")

	owner := loginClient(t, env, "o@x.com", "password123")
	editor := loginClient(t, env, "e@x.com", "password123")
	viewer := loginClient(t, env, "v@x.com", "password123")
	outsider := loginClient(t, env, "z@x.com", "password123")
	anon := anonClient(t, env)

	t.Run("anon redirected to login", func(t *testing.T) {
		anon.get("/projects/"+pid, http.StatusSeeOther)
	})

	t.Run("outsider blocked", func(t *testing.T) {
		outsider.get("/projects/"+pid, 400)
	})

	t.Run("viewer reads but cannot mutate", func(t *testing.T) {
		viewer.get("/projects/"+pid, 200)
		viewer.get("/projects/"+pid+"/tasks", 200)
		viewer.postForm("/projects/"+pid+"/phases", url.Values{"name": {"Nope"}}, 400)
	})

	t.Run("editor mutates", func(t *testing.T) {
		editor.postForm("/projects/"+pid+"/phases", url.Values{"name": {"Electrical"}}, 200)
	})

	t.Run("editor cannot delete project", func(t *testing.T) {
		editor.postForm("/projects/"+pid+"/delete", nil, 400)
	})

	t.Run("owner deletes project", func(t *testing.T) {
		owner.postForm("/projects/"+pid+"/delete", nil, http.StatusSeeOther)
	})
}

func TestTaskTreeAndReorder(t *testing.T) {
	env := newTestEnv(t)
	ownerID := createUser(t, env.app, "Owen", "o@x.com", "password123")
	pid := seedProject(t, env.app, ownerID)
	owner := loginClient(t, env, "o@x.com", "password123")

	owner.postForm("/projects/"+pid+"/phases", url.Values{"name": {"Demolition"}}, 200)
	ph := findRecordID(t, env.app, "phases")

	for _, title := range []string{"Task A", "Task B", "Task C"} {
		owner.postForm(fmt.Sprintf("/phases/%s/tasks", ph), url.Values{"title": {title}}, 200)
	}
	ids := recordIDsByTitle(t, env.app)

	// subtask under A
	owner.postForm(fmt.Sprintf("/tasks/%s/children", ids["Task A"]), url.Values{"title": {"Sub 1"}}, 200)

	// move C to top (before A)
	owner.postForm(fmt.Sprintf("/tasks/%s/move", ids["Task C"]), url.Values{
		"parent": {""}, "phase": {ph}, "next": {ids["Task A"]},
	}, 200)

	tasks := recordsSorted(t, env.app, "tasks")
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}
	if tasks[0].GetString("title") != "Task C" {
		t.Fatalf("Task C should be first after move, got %s", tasks[0].GetString("title"))
	}

	// toggle done sets status + completed date
	owner.postForm(fmt.Sprintf("/tasks/%s/toggle", ids["Task A"]), nil, 200)
	a := findRecord(t, env.app, "tasks", ids["Task A"])
	if a.GetString("status") != "done" || a.GetString("completed_date") == "" {
		t.Fatal("toggle did not complete the task")
	}
}

func TestMaterialBudgetMath(t *testing.T) {
	env := newTestEnv(t)
	ownerID := createUser(t, env.app, "Owen", "o@x.com", "password123")
	pid := seedProject(t, env.app, ownerID)
	owner := loginClient(t, env, "o@x.com", "password123")

	owner.postForm("/projects/"+pid+"/materials", url.Values{
		"item": {"Oak flooring"}, "quantity": {"200"}, "unit_cost": {"12.50"}, "status": {"ordered"},
	}, http.StatusSeeOther)
	owner.postForm("/projects/"+pid+"/materials", url.Values{
		"item": {"Quartz slab"}, "quantity": {"1"}, "unit_cost": {"3200"}, "actual_cost": {"3400"}, "status": {"idea"},
	}, http.StatusSeeOther)
	owner.postForm("/projects/"+pid+"/budget", url.Values{
		"label": {"Labor"}, "category": {"labor"}, "paid": {"2000"},
	}, http.StatusSeeOther)

	// budget page aggregates by category: $2500 + $3400 = $5900 materials, $2000 paid labor
	body := owner.get("/projects/"+pid+"/budget", 200)
	for _, want := range []string{"$5900.00", "$2000.00", "$7900.00"} {
		if !strings.Contains(body, want) {
			t.Errorf("budget page missing %s", want)
		}
	}
	// materials page shows per-item line totals
	matBody := owner.get("/projects/"+pid+"/materials", 200)
	for _, want := range []string{"$2500.00", "$3400.00", "$5900.00"} {
		if !strings.Contains(matBody, want) {
			t.Errorf("materials page missing %s", want)
		}
	}
	if dash := owner.get("/projects/"+pid, 200); !strings.Contains(dash, "$42100.00 remaining") {
		t.Error("dashboard remaining should be $42100 (50000 - 7900)")
	}
}

func TestPhotoUploadAndGuardedServing(t *testing.T) {
	env := newTestEnv(t)
	ownerID := createUser(t, env.app, "Owen", "o@x.com", "password123")
	createUser(t, env.app, "Out", "z@x.com", "password123")
	pid := seedProject(t, env.app, ownerID)
	owner := loginClient(t, env, "o@x.com", "password123")
	outsider := loginClient(t, env, "z@x.com", "password123")

	if code := owner.postMultipart("/projects/"+pid+"/photos", func(w *multipart.Writer) {
		fw, _ := w.CreateFormFile("image", "test.png")
		fw.Write(testPNG)
		w.WriteField("caption", "Wall before")
		w.WriteField("stage", "before")
	}); code != http.StatusSeeOther {
		t.Fatalf("upload: got %d", code)
	}

	ph := findRecord(t, env.app, "photos", findRecordID(t, env.app, "photos"))
	fileURL := "/files/photos/" + ph.Id + "/" + ph.GetString("image")

	owner.get(fileURL, 200)
	outsider.get(fileURL, 404)
	anonClient(t, env).get(fileURL, http.StatusSeeOther)
}

func TestSearchFindsAcrossEntities(t *testing.T) {
	env := newTestEnv(t)
	ownerID := createUser(t, env.app, "Owen", "o@x.com", "password123")
	pid := seedProject(t, env.app, ownerID)
	owner := loginClient(t, env, "o@x.com", "password123")

	owner.postForm("/projects/"+pid+"/phases", url.Values{"name": {"Demo"}}, 200)
	ph := findRecordID(t, env.app, "phases")
	owner.postForm(fmt.Sprintf("/phases/%s/tasks", ph), url.Values{"title": {"Install quartz counters"}}, 200)
	owner.postForm("/projects/"+pid+"/materials", url.Values{"item": {"QUARTZ slab"}, "quantity": {"1"}}, http.StatusSeeOther)

	body := owner.get("/projects/"+pid+"/search?q=quartz", 200)
	if !strings.Contains(body, "Install quartz counters") || !strings.Contains(body, "QUARTZ slab") {
		t.Error("search should match task and material case-insensitively")
	}
}
