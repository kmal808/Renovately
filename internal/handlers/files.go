package handlers

import (
	"errors"
	"net/http"
	"strings"

	"reno/internal/ui"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/pocketbase/pocketbase/tools/router"
)

func (h *H) registerFiles(r *router.Router[*core.RequestEvent]) {
	// rooms
	r.GET("/projects/{pid}/rooms", h.wrapAuth(h.roomsPage))
	r.POST("/projects/{pid}/rooms", h.wrapAuth(h.roomCreate))
	r.POST("/rooms/{rid}/delete", h.wrapAuth(h.roomDelete))

	// photos
	r.GET("/projects/{pid}/photos", h.wrapAuth(h.photosPage))
	r.POST("/projects/{pid}/photos", h.wrapAuth(h.photoUpload))
	r.POST("/photos/{photoid}/delete", h.wrapAuth(h.photoDelete))
	r.GET("/files/photos/{photoid}/{filename}", h.wrapAuth(h.servePhoto))

	// documents
	r.GET("/projects/{pid}/documents", h.wrapAuth(h.documentsPage))
	r.POST("/projects/{pid}/documents", h.wrapAuth(h.documentUpload))
	r.POST("/documents/{doid}/delete", h.wrapAuth(h.documentDelete))
	r.GET("/files/documents/{doid}/{filename}", h.wrapAuth(h.serveDocument))

	// notes & decisions
	r.GET("/projects/{pid}/notes", h.wrapAuth(h.notesPage))
	r.POST("/projects/{pid}/notes", h.wrapAuth(h.noteCreate))
	r.POST("/notes/{nid}/pin", h.wrapAuth(h.notePin))
	r.POST("/notes/{nid}/delete", h.wrapAuth(h.noteDelete))
	r.POST("/projects/{pid}/decisions", h.wrapAuth(h.decisionCreate))
	r.POST("/decisions/{did}/delete", h.wrapAuth(h.decisionDelete))
}

// ---------- rooms ----------

func (h *H) roomsFor(pid string) []ui.RoomRow {
	records, _ := h.app.FindRecordsByFilter(
		"rooms", "project = {:p}", "sort", 0, 0, map[string]any{"p": pid})
	out := make([]ui.RoomRow, 0, len(records))
	for _, r := range records {
		out = append(out, ui.RoomRow{ID: r.Id, Name: r.GetString("name"), Type: r.GetString("type")})
	}
	return out
}

func roomName(h *H, rid string) string {
	if rid == "" {
		return ""
	}
	r, err := h.app.FindRecordById("rooms", rid)
	if err != nil {
		return ""
	}
	return r.GetString("name")
}

func (h *H) roomsPage(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.RoomsPage(user(e), pv, h.roomsFor(p.Id), ""))
}

func (h *H) roomCreate(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	name := strings.TrimSpace(e.Request.FormValue("name"))
	if name == "" {
		return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/rooms")
	}
	last, _ := h.app.FindRecordsByFilter(
		"rooms", "project = {:p}", "-sort", 1, 0, map[string]any{"p": p.Id})
	sort := 0.0
	if len(last) > 0 {
		sort = last[0].GetFloat("sort") + 1
	}
	collection, err := h.app.FindCachedCollectionByNameOrId("rooms")
	if err != nil {
		return err
	}
	r := core.NewRecord(collection)
	r.Set("project", p.Id)
	r.Set("name", name)
	r.Set("type", e.Request.FormValue("type"))
	r.Set("sort", sort)
	if err := h.app.Save(r); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/rooms")
}

func (h *H) roomDelete(e *core.RequestEvent) error {
	r, err := h.app.FindRecordById("rooms", e.Request.PathValue("rid"))
	if err != nil {
		return err
	}
	p, m, err := h.loadProjectByID(r.GetString("project"), user(e).ID)
	if err != nil || m == nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	if err := h.app.Delete(r); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/rooms")
}

// ---------- photos ----------

func (h *H) photoURL(recordID, filename string) string {
	return "/files/photos/" + recordID + "/" + filename
}

func (h *H) photosPage(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	q := e.Request.URL.Query()
	stage, roomID := q.Get("stage"), q.Get("room")

	filter := "project = {:p}"
	params := map[string]any{"p": p.Id}
	if stage != "" {
		filter += " && stage = {:stage}"
		params["stage"] = stage
	}
	if roomID != "" {
		filter += " && room = {:room}"
		params["room"] = roomID
	}
	records, err := h.app.FindRecordsByFilter("photos", filter, "-taken_date", 0, 0, params)
	if err != nil {
		return err
	}
	photos := make([]ui.PhotoCard, 0, len(records))
	for _, ph := range records {
		photos = append(photos, ui.PhotoCard{
			ID:      ph.Id,
			URL:     h.photoURL(ph.Id, ph.GetString("image")),
			Caption: ph.GetString("caption"),
			Stage:   ph.GetString("stage"),
			Room:    roomName(h, ph.GetString("room")),
			Taken:   ph.GetString("taken_date"),
		})
	}
	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.PhotosPage(user(e), pv, photos, h.roomsFor(p.Id), stage, roomID, ""))
}

// uploadedFile pulls a single uploaded file from the multipart form.
func uploadedFile(e *core.RequestEvent, key string) (*filesystem.File, error) {
	files, err := e.FindUploadedFiles(key)
	if err != nil || len(files) == 0 {
		return nil, errors.New("no file uploaded")
	}
	return files[0], nil
}

func (h *H) photoUpload(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	file, err := uploadedFile(e, "image")
	if err != nil {
		return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/photos")
	}
	collection, err := h.app.FindCachedCollectionByNameOrId("photos")
	if err != nil {
		return err
	}
	f := e.Request.FormValue
	ph := core.NewRecord(collection)
	ph.Set("project", p.Id)
	ph.Set("image", file)
	ph.Set("caption", strings.TrimSpace(f("caption")))
	ph.Set("stage", f("stage"))
	if f("room") != "" {
		ph.Set("room", f("room"))
	}
	ph.Set("taken_date", f("taken_date"))
	if err := h.app.Save(ph); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/photos")
}

func (h *H) photoDelete(e *core.RequestEvent) error {
	ph, err := h.app.FindRecordById("photos", e.Request.PathValue("photoid"))
	if err != nil {
		return err
	}
	p, m, err := h.loadProjectByID(ph.GetString("project"), user(e).ID)
	if err != nil || m == nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	if err := h.app.Delete(ph); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/photos")
}

// servePhoto streams a photo file after checking project membership.
func (h *H) servePhoto(e *core.RequestEvent) error {
	ph, err := h.app.FindRecordById("photos", e.Request.PathValue("photoid"))
	if err != nil {
		return e.NotFoundError("", err)
	}
	if _, m, err := h.loadProjectByID(ph.GetString("project"), user(e).ID); err != nil || m == nil {
		return e.NotFoundError("", errors.New("not found"))
	}
	return h.serveRecordFile(e, ph, e.Request.PathValue("filename"))
}

// serveDocument streams a document file after checking project membership.
func (h *H) serveDocument(e *core.RequestEvent) error {
	doc, err := h.app.FindRecordById("documents", e.Request.PathValue("doid"))
	if err != nil {
		return e.NotFoundError("", err)
	}
	if _, m, err := h.loadProjectByID(doc.GetString("project"), user(e).ID); err != nil || m == nil {
		return e.NotFoundError("", errors.New("not found"))
	}
	return h.serveRecordFile(e, doc, e.Request.PathValue("filename"))
}

// serveRecordFile serves a stored file from the record's base files path,
// generating an image thumbnail when a supported ?thumb= is requested.
func (h *H) serveRecordFile(e *core.RequestEvent, record *core.Record, filename string) error {
	field := record.FindFileFieldByFile(filename)
	if field == nil {
		return e.NotFoundError("", nil)
	}
	fsys, err := h.app.NewFilesystem()
	if err != nil {
		return e.InternalServerError("filesystem failure", err)
	}
	defer fsys.Close()

	servePath := record.BaseFilesPath() + "/" + filename
	serveName := filename

	// thumbnails for gallery rendering
	if thumb := e.Request.URL.Query().Get("thumb"); thumb != "" && strings.Contains(field.Name, "image") {
		thumbPath := record.BaseFilesPath() + "/thumbs_" + filename + "/" + thumb + "_" + filename
		if ok, _ := fsys.Exists(thumbPath); ok {
			servePath, serveName = thumbPath, thumb+"_"+filename
		}
	}

	e.Response.Header().Del("X-Frame-Options")
	if err := fsys.Serve(e.Response, e.Request, servePath, serveName); err != nil {
		return e.NotFoundError("", err)
	}
	return nil
}

// ---------- documents ----------

func (h *H) documentsPage(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	records, err := h.app.FindRecordsByFilter(
		"documents", "project = {:p}", "-created", 0, 0, map[string]any{"p": p.Id})
	if err != nil {
		return err
	}
	docs := make([]ui.DocRow, 0, len(records))
	for _, d := range records {
		docs = append(docs, ui.DocRow{
			ID:       d.Id,
			Title:    d.GetString("title"),
			Filename: d.GetString("file"),
			URL:      "/files/documents/" + d.Id + "/" + d.GetString("file"),
			Category: d.GetString("category"),
			Date:     d.GetString("doc_date"),
		})
	}
	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.DocumentsPage(user(e), pv, docs, ""))
}

func (h *H) documentUpload(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	file, err := uploadedFile(e, "file")
	if err != nil {
		return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/documents")
	}
	collection, err := h.app.FindCachedCollectionByNameOrId("documents")
	if err != nil {
		return err
	}
	f := e.Request.FormValue
	d := core.NewRecord(collection)
	d.Set("project", p.Id)
	d.Set("file", file)
	d.Set("title", strings.TrimSpace(f("title")))
	d.Set("category", f("category"))
	d.Set("doc_date", f("doc_date"))
	if err := h.app.Save(d); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/documents")
}

func (h *H) documentDelete(e *core.RequestEvent) error {
	d, err := h.app.FindRecordById("documents", e.Request.PathValue("doid"))
	if err != nil {
		return err
	}
	p, m, err := h.loadProjectByID(d.GetString("project"), user(e).ID)
	if err != nil || m == nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	if err := h.app.Delete(d); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/documents")
}

// ---------- notes & decisions ----------

func (h *H) notesPage(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	noteRecords, err := h.app.FindRecordsByFilter(
		"notes", "project = {:p}", "-pinned,-created", 0, 0, map[string]any{"p": p.Id})
	if err != nil {
		return err
	}
	notes := make([]ui.NoteRow, 0, len(noteRecords))
	for _, n := range noteRecords {
		notes = append(notes, ui.NoteRow{ID: n.Id, Body: n.GetString("body"), Pinned: n.GetBool("pinned")})
	}

	decisionRecords, err := h.app.FindRecordsByFilter(
		"decisions", "project = {:p}", "-created", 0, 0, map[string]any{"p": p.Id})
	if err != nil {
		return err
	}
	decisions := make([]ui.DecisionRow, 0, len(decisionRecords))
	for _, d := range decisionRecords {
		decisions = append(decisions, ui.DecisionRow{
			ID:      d.Id,
			Title:   d.GetString("title"),
			Options: joinTags(d.Get("options")),
			Selected: d.GetString("selected"),
			Date:    d.GetString("decided_date"),
			Notes:   d.GetString("notes"),
		})
	}
	pv, _ := h.projectView(p, m)
	return renderPage(e, ui.NotesPage(user(e), pv, notes, decisions, ""))
}

func joinTags(v any) string {
	switch xs := v.(type) {
	case []string:
		return strings.Join(xs, ", ")
	case []any:
		parts := make([]string, 0, len(xs))
		for _, x := range xs {
			if s, ok := x.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

func (h *H) noteCreate(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	body := strings.TrimSpace(e.Request.FormValue("body"))
	if body == "" {
		return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/notes")
	}
	collection, err := h.app.FindCachedCollectionByNameOrId("notes")
	if err != nil {
		return err
	}
	n := core.NewRecord(collection)
	n.Set("project", p.Id)
	n.Set("body", body)
	if err := h.app.Save(n); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/notes")
}

func (h *H) noteWithProject(e *core.RequestEvent) (*core.Record, *core.Record, *core.Record, error) {
	n, err := h.app.FindRecordById("notes", e.Request.PathValue("nid"))
	if err != nil {
		return nil, nil, nil, err
	}
	p, m, err := h.loadProjectByID(n.GetString("project"), user(e).ID)
	if err != nil || m == nil {
		return nil, nil, nil, errors.New("note not found")
	}
	return n, p, m, nil
}

func (h *H) notePin(e *core.RequestEvent) error {
	n, p, m, err := h.noteWithProject(e)
	if err != nil || m == nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	n.Set("pinned", !n.GetBool("pinned"))
	if err := h.app.Save(n); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/notes")
}

func (h *H) noteDelete(e *core.RequestEvent) error {
	n, p, m, err := h.noteWithProject(e)
	if err != nil || m == nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	if err := h.app.Delete(n); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/notes")
}

func (h *H) decisionCreate(e *core.RequestEvent) error {
	p, m, err := h.loadProject(e)
	if err != nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	title := strings.TrimSpace(e.Request.FormValue("title"))
	if title == "" {
		return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/notes")
	}
	options := []string{}
	for _, o := range strings.Split(e.Request.FormValue("options"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			options = append(options, o)
		}
	}
	collection, err := h.app.FindCachedCollectionByNameOrId("decisions")
	if err != nil {
		return err
	}
	f := e.Request.FormValue
	d := core.NewRecord(collection)
	d.Set("project", p.Id)
	d.Set("title", title)
	d.Set("options", options)
	d.Set("selected", strings.TrimSpace(f("selected")))
	d.Set("decided_date", f("decided_date"))
	d.Set("notes", f("notes"))
	if err := h.app.Save(d); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/notes")
}

func (h *H) decisionDelete(e *core.RequestEvent) error {
	d, err := h.app.FindRecordById("decisions", e.Request.PathValue("did"))
	if err != nil {
		return err
	}
	p, m, err := h.loadProjectByID(d.GetString("project"), user(e).ID)
	if err != nil || m == nil {
		return err
	}
	if err := requireEditor(m); err != nil {
		return err
	}
	if err := h.app.Delete(d); err != nil {
		return err
	}
	return redirect(e, http.StatusSeeOther, "/projects/"+p.Id+"/notes")
}
