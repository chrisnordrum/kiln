package runner

import (
	"strings"
	"testing"

	"kiln/internal/ast"
	"kiln/internal/check"
	"kiln/internal/diag"
	"kiln/internal/eval"
	"kiln/internal/parse"
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
  rank int

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
    new Task owner=session.user title=title rank=0
  after
    refresh

route list
  path /tasks
  guard session.user else redirect /login
  data
    tasks many Task where rank >= 0 order rank asc
  view
    page title="Tasks"
      when count(tasks) == 0
        empty "Nothing here."
      else
        each tasks as t
          row
            check value=t.done do=toggle id=t.id done=$value
            text t.title

route login
  path /login
  view
    page title="Sign in"
      head 1 "Sign in"
`

// load parses and checks the fixture, so these tests never run a program the
// checker would have rejected.
func load(t *testing.T, extra string) *ast.Program {
	t.Helper()
	var d diag.List
	p := parse.Program([]parse.File{{Path: "a.kiln", Src: app + extra}}, &d)
	if d.Empty() {
		check.Program(p, &d)
	}
	if !d.Empty() {
		shown, _ := d.Resolved()
		t.Fatalf("fixture is not a valid program: %+v", shown)
	}
	return p
}

func setup(t *testing.T) *Runner {
	t.Helper()
	r := New(load(t, ""))
	r.Seed("User", map[string]eval.Value{"id": int64(1), "name": "Ada"})
	r.Seed("User", map[string]eval.Value{"id": int64(2), "name": "Bob"})
	r.Seed("Task", map[string]eval.Value{"id": int64(1), "owner": int64(1), "title": "Ship it", "rank": int64(0)})
	return r
}

func action(t *testing.T, r *Runner, name string) *ast.Action {
	t.Helper()
	a, ok := r.P.Action(name)
	if !ok {
		t.Fatalf("no action %s", name)
	}
	return a
}

// The allow rule is the boundary: a denied call must change nothing.
func TestAllowRuleIsEnforcedBeforeAnyWrite(t *testing.T) {
	r := setup(t)
	r.SignIn("user", int64(2)) // not the owner

	err := r.Call(action(t, r, "toggle"), map[string]eval.Value{"id": int64(1), "done": true})
	if err != Denied {
		t.Fatalf("want Denied, got %v", err)
	}
	row, _ := r.E.S.Get("Task", int64(1))
	if row["done"] != false {
		t.Error("a denied call still wrote to the store")
	}

	r.SignIn("user", int64(1)) // the owner
	if err := r.Call(action(t, r, "toggle"), map[string]eval.Value{"id": int64(1), "done": true}); err != nil {
		t.Fatalf("the owner should be allowed: %v", err)
	}
	row, _ = r.E.S.Get("Task", int64(1))
	if row["done"] != true {
		t.Error("the allowed call did not write")
	}
}

func TestAnonymousCallerIsDenied(t *testing.T) {
	r := setup(t)
	if err := r.Call(action(t, r, "add"), map[string]eval.Value{"title": "x"}); err != Denied {
		t.Errorf("want Denied with no session, got %v", err)
	}
}

func TestNewRecordUsesSession(t *testing.T) {
	r := setup(t)
	r.SignIn("user", int64(2))
	if err := r.Call(action(t, r, "add"), map[string]eval.Value{"title": "Fresh"}); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, row := range r.E.S.All("Task") {
		if row["title"] == "Fresh" {
			found = true
			if row["owner"] != int64(2) {
				t.Errorf("owner = %v, want the calling session's user", row["owner"])
			}
			if row["done"] != false {
				t.Errorf("done = %v, want the schema default", row["done"])
			}
		}
	}
	if !found {
		t.Error("the new record was not inserted")
	}
}

// A guard that fails redirects instead of rendering.
func TestGuardRedirects(t *testing.T) {
	r := setup(t)
	page, err := r.Visit("/tasks")
	if err != nil {
		t.Fatal(err)
	}
	if page.Redirect != "/login" {
		t.Errorf("want a redirect to /login for an anonymous visitor, got %q", page.Redirect)
	}
	if page.Text != "" {
		t.Error("a redirected visit should render nothing")
	}

	r.SignIn("user", int64(1))
	page, err = r.Visit("/tasks")
	if err != nil {
		t.Fatal(err)
	}
	if page.Redirect != "" {
		t.Errorf("a signed-in visitor should not be redirected, got %q", page.Redirect)
	}
	if !strings.Contains(page.Text, "Ship it") {
		t.Errorf("the page did not render its data:\n%s", page.Text)
	}
}

func TestPathParametersAreExtracted(t *testing.T) {
	p := load(t, `
route detail
  path /tasks/:id
  data
    task one Task where id == params.id
  view
    page
      text task.title
`)
	r := New(p)
	r.Seed("Task", map[string]eval.Value{"id": int64(7), "title": "Seven", "rank": int64(0)})
	page, err := r.Visit("/tasks/7")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Text, "Seven") {
		t.Errorf("the URL parameter did not reach the query:\n%s", page.Text)
	}
}

func TestUnknownPath(t *testing.T) {
	r := setup(t)
	if _, err := r.Visit("/nope"); err == nil {
		t.Error("want an error for a path no route serves")
	}
}

// when/else picks exactly one branch, and each expands to the rows that exist.
func TestRenderingReflectsTheData(t *testing.T) {
	p := load(t, "")
	r := New(p)
	r.Seed("User", map[string]eval.Value{"id": int64(1), "name": "Ada"})
	r.SignIn("user", int64(1))

	page, _ := r.Visit("/tasks")
	if !strings.Contains(page.Text, "Nothing here.") {
		t.Errorf("an empty list should take the when branch:\n%s", page.Text)
	}

	r.Seed("Task", map[string]eval.Value{"id": int64(1), "owner": int64(1), "title": "One", "rank": int64(0)})
	r.Seed("Task", map[string]eval.Value{"id": int64(2), "owner": int64(1), "title": "Two", "rank": int64(1)})
	page, _ = r.Visit("/tasks")
	if strings.Contains(page.Text, "Nothing here.") {
		t.Errorf("a non-empty list should take the else branch:\n%s", page.Text)
	}
	if strings.Count(page.Text, "check ") != 2 {
		t.Errorf("each should expand to one row per record:\n%s", page.Text)
	}
	if !strings.Contains(page.Text, `do=toggle(done=$value, id=1)`) {
		t.Errorf("a control should show its action and arguments:\n%s", page.Text)
	}
	// Ordering is by rank, so One precedes Two.
	if strings.Index(page.Text, "One") > strings.Index(page.Text, "Two") {
		t.Errorf("rows are not in rank order:\n%s", page.Text)
	}
}

// Rendering the same data twice must give the same bytes, or snapshots are
// worthless as a verification channel.
func TestRenderingIsDeterministic(t *testing.T) {
	r := setup(t)
	r.SignIn("user", int64(1))
	first, _ := r.Visit("/tasks")
	for range 5 {
		again, _ := r.Visit("/tasks")
		if again.Text != first.Text {
			t.Fatalf("rendering is not stable:\n--- a ---\n%s\n--- b ---\n%s", first.Text, again.Text)
		}
	}
}

func TestTestRunnerReportsFailures(t *testing.T) {
	p := load(t, `
test "passes"
  seed User id=1 name="Ada"
  seed Task id=1 owner=1 title="Ship it" rank=0
  as user=1
  call toggle id=1 done=true
  expect Task[1].done == true
  expect denied when as user=2

test "fails on a false expectation"
  seed User id=1 name="Ada"
  seed Task id=1 owner=1 title="Ship it" rank=0
  as user=1
  expect Task[1].done == true
`)
	results := RunTests(p)
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	if !results[0].OK() {
		t.Errorf("the passing test failed: %v", results[0].Failures)
	}
	if results[1].OK() {
		t.Error("the failing test passed")
	}
	if len(results[1].Failures) == 0 || !strings.Contains(results[1].Failures[0], "expected") {
		t.Errorf("want a readable failure, got %v", results[1].Failures)
	}
}

// Tests must not leak state into each other.
func TestEachTestGetsAFreshStore(t *testing.T) {
	p := load(t, `
test "first inserts a task"
  seed User id=1 name="Ada"
  as user=1
  call add title="Only in the first test"
  expect Task[1].title == "Only in the first test"

test "second starts empty"
  expect Task[1].title == null
`)
	results := RunTests(p)
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	if !results[0].OK() {
		t.Errorf("the inserting test failed: %v", results[0].Failures)
	}
	if !results[1].OK() {
		t.Errorf("state leaked between tests: %v", results[1].Failures)
	}
}

// A guard is proven statically by K040 and, until now, covered by no test at
// all: a redirect made visit itself fail, so there was nothing to assert on.
func TestGuardRedirectIsTestable(t *testing.T) {
	p := load(t, `
test "anonymous visitors are sent to sign in"
  visit /tasks
  expect redirect /login

test "the wrong target is caught"
  visit /tasks
  expect redirect /tasks

test "a rendered page is not a redirect"
  seed User id=1 name="Ada"
  as user=1
  visit /tasks
  expect redirect /login
`)
	res := RunTests(p)
	if !res[0].OK() {
		t.Errorf("a real redirect should satisfy the expectation: %v", res[0].Failures)
	}
	if res[1].OK() {
		t.Error("the wrong redirect target should fail")
	} else if !strings.Contains(res[1].Failures[0], "not /tasks") {
		t.Errorf("want both targets named, got %q", res[1].Failures[0])
	}
	if res[2].OK() {
		t.Error("a page that renders should not satisfy a redirect expectation")
	}
}

// Absence needs a first-class assertion. Without one the only way to prove a
// row is hidden was to read a snapshot.
func TestNegativeTextAssertion(t *testing.T) {
	p := load(t, `
test "absent text passes"
  seed User id=1 name="Ada"
  seed Task id=1 owner=1 title="Visible" rank=0
  as user=1
  visit /tasks
  expect text "Visible"
  expect no text "Hidden"

test "present text fails"
  seed User id=1 name="Ada"
  seed Task id=1 owner=1 title="Visible" rank=0
  as user=1
  visit /tasks
  expect no text "Visible"
`)
	res := RunTests(p)
	if !res[0].OK() {
		t.Errorf("absent text should pass: %v", res[0].Failures)
	}
	if res[1].OK() {
		t.Error("text that is present should fail a negative assertion")
	} else if !strings.Contains(res[1].Failures[0], "should not") {
		t.Errorf("want a clear message, got %q", res[1].Failures[0])
	}
}

// Asserting on a page that was redirected away should say so, rather than
// reporting that the text is merely missing.
func TestTextAssertionAfterARedirectExplainsItself(t *testing.T) {
	p := load(t, `
test "guarded away"
  visit /tasks
  expect text "Ship it"
`)
	res := RunTests(p)
	if res[0].OK() {
		t.Fatal("want a failure")
	}
	if !strings.Contains(res[0].Failures[0], "redirected") {
		t.Errorf("want the redirect named as the cause, got %q", res[0].Failures[0])
	}
}
