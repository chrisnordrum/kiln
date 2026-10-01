package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chrisnordrum/kiln/internal/ast"
	"github.com/chrisnordrum/kiln/internal/check"
	"github.com/chrisnordrum/kiln/internal/diag"
	"github.com/chrisnordrum/kiln/internal/eval"
	"github.com/chrisnordrum/kiln/internal/parse"
)

const app = `app t
  title "T"
  session user ref User

table User
  id id
  name text max 80

table Task
  id id
  owner ref User on delete cascade
  title text max 200
  done bool = false

action toggle
  in
    id ref Task
    done bool
  allow session.user == Task[id].owner
  do
    set Task[id].done = done
  after
    refresh

action add
  in
    title text max 200
  allow session.user != null
  do
    new Task owner=session.user title=title
  after
    refresh

route list
  path /tasks
  guard session.user else redirect /login
  data
    tasks many Task where done != null
  view
    page title="Tasks"
      each tasks as t
        row
          check value=t.done do=toggle id=t.id done=$value
          text t.title
      form do=add
        input title text required
        submit "Add"

route login
  path /login
  view
    page title="Sign in"
      head 1 "Sign in"
`

func program(t *testing.T) *ast.Program {
	t.Helper()
	var d diag.List
	p := parse.Program([]parse.File{{Path: "a.kiln", Src: app}}, &d)
	if d.Empty() {
		check.Program(p, &d)
	}
	if !d.Empty() {
		shown, _ := d.Resolved()
		t.Fatalf("fixture is not a valid program: %+v", shown)
	}
	return p
}

func newServer(t *testing.T, signedIn bool) *Server {
	t.Helper()
	s := New(program(t))
	s.Seed("User", map[string]eval.Value{"id": int64(1), "name": "Ada"})
	s.Seed("Task", map[string]eval.Value{"id": int64(1), "owner": int64(1), "title": "Ship it"})
	if signedIn {
		s.SignIn("user", int64(1))
	}
	return s
}

func get(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func post(t *testing.T, s *Server, body string) actionResponse {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/_k/action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(w, req)
	var out actionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, w.Body.String())
	}
	return out
}

func TestGuardRedirectsAnonymousVisitors(t *testing.T) {
	w := get(t, newServer(t, false), "/tasks")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("want a redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("want /login, got %q", loc)
	}
}

func TestPageRendersWithControls(t *testing.T) {
	body := get(t, newServer(t, true), "/tasks").Body.String()
	for _, want := range []string{
		`<title>Tasks</title>`,
		`data-k-do="toggle"`,
		`data-k-event="done"`,
		`<p>Ship it</p>`,
		`/_k/client.js`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page is missing %q:\n%s", want, body)
		}
	}
}

func TestUnknownPathIs404(t *testing.T) {
	if code := get(t, newServer(t, true), "/nope").Code; code != http.StatusNotFound {
		t.Errorf("want 404, got %d", code)
	}
}

// An action mutates and the response carries the re-rendered route, so what an
// interaction returns is what a reload would produce.
func TestActionMutatesAndReturnsFreshMarkup(t *testing.T) {
	s := newServer(t, true)
	out := post(t, s, `{"action":"toggle","args":{"id":1,"done":true},"path":"/tasks"}`)
	if out.Error != "" {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	if !strings.Contains(out.HTML, "checked") {
		t.Errorf("the checkbox should come back checked:\n%s", out.HTML)
	}
	if strings.Contains(out.HTML, "<html") {
		t.Error("an action returns the main region, not a whole document")
	}
}

func TestActionEnforcesPermissions(t *testing.T) {
	s := newServer(t, false) // nobody is signed in
	out := post(t, s, `{"action":"toggle","args":{"id":1,"done":true},"path":"/tasks"}`)
	if out.Error != "not allowed" {
		t.Errorf("want a refusal, got %+v", out)
	}
	row, _ := s.r.E.S.Get("Task", int64(1))
	if row["done"] != false {
		t.Error("a refused action still wrote to the store")
	}
}

func TestUnknownActionIsReported(t *testing.T) {
	out := post(t, newServer(t, true), `{"action":"nope","args":{},"path":"/tasks"}`)
	if !strings.Contains(out.Error, "no action named nope") {
		t.Errorf("want a clear error, got %+v", out)
	}
}

// Every form field arrives as text; the declared parameter type decides what it
// becomes.
func TestFormValuesAreCoercedToDeclaredTypes(t *testing.T) {
	s := newServer(t, true)
	out := post(t, s, `{"action":"toggle","args":{"id":"1","done":"on"},"path":"/tasks"}`)
	if out.Error != "" {
		t.Fatalf("text form values should coerce: %s", out.Error)
	}
	row, _ := s.r.E.S.Get("Task", int64(1))
	if row["done"] != true {
		t.Errorf(`"on" from a checkbox should become true, got %v`, row["done"])
	}

	bad := post(t, s, `{"action":"toggle","args":{"id":"not-a-number","done":true},"path":"/tasks"}`)
	if bad.Error == "" {
		t.Error("a value that cannot be coerced should be refused")
	}
}

func TestMissingArgumentIsRefused(t *testing.T) {
	out := post(t, newServer(t, true), `{"action":"toggle","args":{"id":1},"path":"/tasks"}`)
	if !strings.Contains(out.Error, "done") {
		t.Errorf("want the missing parameter named, got %+v", out)
	}
}

func TestNewRecordAppearsInTheResponse(t *testing.T) {
	out := post(t, newServer(t, true), `{"action":"add","args":{"title":"Fresh"},"path":"/tasks"}`)
	if out.Error != "" {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	if !strings.Contains(out.HTML, "Fresh") {
		t.Errorf("the new row should be in the re-rendered markup:\n%s", out.HTML)
	}
}

// Values from the store must not be able to inject markup.
func TestStoredTextIsEscaped(t *testing.T) {
	s := newServer(t, true)
	s.Seed("Task", map[string]eval.Value{
		"id": int64(2), "owner": int64(1), "title": `<script>alert(1)</script>`})
	body := get(t, s, "/tasks").Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("stored text reached the page unescaped")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("want the text escaped, got:\n%s", body)
	}
}

func TestAssetsAreServed(t *testing.T) {
	s := newServer(t, true)
	for path, want := range map[string]string{
		"/_k/client.js": "data-k-do",
		"/_k/style.css": ".k-row",
	} {
		w := get(t, s, path)
		if w.Code != http.StatusOK {
			t.Errorf("%s returned %d", path, w.Code)
		}
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("%s does not look right", path)
		}
	}
}
