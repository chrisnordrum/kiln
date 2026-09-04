package check

import (
	"strings"
	"testing"

	"kiln/internal/diag"
	"kiln/internal/parse"
)

// base is the schema every case below is checked against.
const base = `app tasks
  title "Tasks"
  session user ref User
  effect email via smtp

table User
  id id
  email text max 200 unique
  name text max 80

table Project
  id id
  owner ref User on delete cascade
  title text max 200

table Task
  id id
  project ref Project on delete cascade
  title text max 200
  done bool = false
  rank int
  created at now
`

// run checks base plus src and returns the diagnostics.
func run(t *testing.T, src string) []diag.Diag {
	t.Helper()
	var d diag.List
	p := parse.Program([]parse.File{
		{Path: "base.kiln", Src: base},
		{Path: "case.kiln", Src: src},
	}, &d)
	if !d.Empty() {
		shown, _ := d.Resolved()
		t.Fatalf("the case did not parse: %+v", shown)
	}
	Program(p, &d)
	shown, _ := d.Resolved()
	return shown
}

// codes returns just the diagnostic codes.
func codes(t *testing.T, src string) []string {
	t.Helper()
	var out []string
	for _, s := range run(t, src) {
		out = append(out, s.Code)
	}
	return out
}

// hasCode reports whether a code appears.
func hasCode(list []string, want string) bool {
	for _, c := range list {
		if c == want {
			return true
		}
	}
	return false
}

// a guarded route, for cases that are not about reachability
const guarded = `
route r
  path /r
  guard session.user else redirect /r
  data
    tasks many Task where rank > 0
  view
    page
`

func TestCleanProgramChecks(t *testing.T) {
	src := `
action toggle
  in
    id ref Task
    done bool
  allow session.user == Task[id].project.owner
  do
    set Task[id].done = done
  after
    refresh

route detail
  path /p/:id
  guard session.user else redirect /p/1
  data
    project one Project where id == params.id
    tasks many Task where project == params.id order rank asc
  view
    page title=project.title
      head 2 "Tasks"
      each tasks as t
        row gap=2
          check value=t.done do=toggle id=t.id done=$value
          text t.title
`
	if got := codes(t, src); len(got) > 0 {
		t.Errorf("clean program produced %v:\n%+v", got, run(t, src))
	}
}

func TestUnknownNamesAreCaught(t *testing.T) {
	cases := []struct {
		name, src, want, suggest string
	}{
		{"unknown field on a record", `
route r
  path /r
  data
    task one Task where id == 1
  view
    page
      text task.titel`, "K021", "title"},

		{"unknown field through a ref walk", `
action a
  in
    id ref Task
  allow session.user == Task[id].project.ownr
  do
    set Task[id].done = true`, "K021", "owner"},

		{"unknown table", `
action a
  in
    id ref Tsak
  allow true
  do
    set Task[id].done = true`, "K020", "Task"},

		{"unknown action on do=", `
route r
  path /r
  guard session.user else redirect /r
  view
    page
      button "Go" do=togle id=1`, "K022", ""},

		{"unknown stdlib function", `
route r
  path /r
  data
    tasks many Task where rank > 0
  view
    page
      text coont(tasks)`, "K025", "count"},

		{"unknown style token", `
route r
  path /r
  view
    page
      text "hi" style=subtle`, "K024", ""},

		{"unknown view element", `
route r
  path /r
  view
    page
      paragraph "hi"`, "K050", ""},

		{"unknown attribute", `
route r
  path /r
  view
    page
      text "hi" wobble=2`, "K052", ""},

		{"unknown session field", `
action a
  allow session.admin == 1
  do
    set Task[1].done = true`, "K021", ""},

		{"dead internal link", `
route r
  path /r
  view
    page
      link "Home" to=/nowhere`, "K023", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := run(t, c.src)
			var found *diag.Diag
			for i := range got {
				if got[i].Code == c.want {
					found = &got[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("want %s, got %+v", c.want, got)
			}
			if c.suggest != "" && !strings.Contains(strings.Join(found.Near, " "), c.suggest) {
				t.Errorf("want %q suggested, got %v", c.suggest, found.Near)
			}
			if found.Fix == "" {
				t.Error("diagnostic carries no fix")
			}
		})
	}
}

// The headline claim: a control cannot be wired to an action the caller could
// never satisfy.
func TestUnreachableActionIsCaught(t *testing.T) {
	unguarded := `
action toggle
  in
    id ref Task
  allow session.user == Task[id].project.owner
  do
    set Task[id].done = true

route r
  path /r
  view
    page
      button "Toggle" do=toggle id=1`
	if got := codes(t, unguarded); !hasCode(got, "K040") {
		t.Errorf("an unguarded route binding an owner-only action should be K040, got %v", got)
	}

	// The same program with a guard establishing session.user is fine.
	withGuard := strings.Replace(unguarded, "  path /r\n", "  path /r\n  guard session.user else redirect /r\n", 1)
	if got := codes(t, withGuard); hasCode(got, "K040") {
		t.Errorf("a guarded route should not be K040, got %v", got)
	}
}

func TestArgumentsMustMatchTheAction(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"missing parameter", `
action toggle
  in
    id ref Task
    done bool
  allow true
  do
    set Task[id].done = done

route r
  path /r
  view
    page
      button "Go" do=toggle id=1`, "K031"},

		{"unknown parameter", `
action toggle
  in
    id ref Task
  allow true
  do
    set Task[id].done = true

route r
  path /r
  view
    page
      button "Go" do=toggle id=1 colour="red"`, "K032"},

		{"wrong parameter type", `
action toggle
  in
    id ref Task
    done bool
  allow true
  do
    set Task[id].done = done

route r
  path /r
  view
    page
      button "Go" do=toggle id=1 done="yes"`, "K030"},

		{"ref to the wrong table", `
action toggle
  in
    id ref Task
  allow true
  do
    set Task[id].done = true

route r
  path /r
  data
    project one Project where id == 1
  view
    page
      button "Go" do=toggle id=project.owner`, "K030"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := codes(t, c.src); !hasCode(got, c.want) {
				t.Errorf("want %s, got %v", c.want, got)
			}
		})
	}
}

// A form's inputs supply the action's parameters by name.
func TestFormInputsSupplyParameters(t *testing.T) {
	ok := `
action add
  in
    project ref Project
    title text max 200
  allow true
  do
    new Task project=project title=title rank=0

route r
  path /r
  view
    page
      form do=add project=1
        input title text required
        submit "Add"`
	if got := codes(t, ok); len(got) > 0 {
		t.Errorf("a form whose inputs cover the parameters should check clean, got %v", got)
	}

	missing := strings.Replace(ok, "        input title text required\n", "", 1)
	if got := codes(t, missing); !hasCode(got, "K031") {
		t.Errorf("a form missing an input for a parameter should be K031, got %v", got)
	}
}

func TestTypeErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"comparing text with bool", `
action a
  allow true
  do
    set Task[1].title = true`, "K030"},

		{"non-condition guard", guardSrc("guard Task[1].title else redirect /r"), "K030"},

		{"each over a record", `
route r
  path /r
  data
    project one Project where id == 1
  view
    page
      each project as p
        text p.title`, "K030"},

		{"reading a field from a list", `
route r
  path /r
  data
    tasks many Task where rank > 0
  view
    page
      text tasks.title`, "K030"},

		{"ordering by a missing field", `
route r
  path /r
  data
    tasks many Task where rank > 0 order priority asc
  view
    page`, "K021"},

		{"gap out of range", `
route r
  path /r
  view
    page
      row gap=99`, "K030"},

		{"heading level out of range", `
route r
  path /r
  view
    page
      head 9 "Too deep"`, "K030"},

		{"stdlib arity", `
route r
  path /r
  view
    page
      text truncate("abc")`, "K031"},

		{"stdlib argument type", `
route r
  path /r
  view
    page
      text upper(5)`, "K030"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := codes(t, c.src); !hasCode(got, c.want) {
				t.Errorf("want %s, got %v", c.want, got)
			}
		})
	}
}

func guardSrc(guard string) string {
	return "\nroute r\n  path /r\n  " + guard + "\n  view\n    page\n"
}

// A new record must supply every field that has no default.
func TestNewMustSupplyRequiredFields(t *testing.T) {
	src := `
action add
  in
    project ref Project
  allow true
  do
    new Task project=project`
	got := run(t, src)
	var msgs []string
	for _, g := range got {
		if g.Code == "K053" {
			msgs = append(msgs, g.Msg)
		}
	}
	if len(msgs) == 0 {
		t.Fatalf("want K053 for the unset fields, got %+v", got)
	}
	joined := strings.Join(msgs, " ")
	for _, field := range []string{"title", "rank"} {
		if !strings.Contains(joined, field) {
			t.Errorf("want %s reported as unset, got %q", field, joined)
		}
	}
	if strings.Contains(joined, "done") || strings.Contains(joined, "created") || strings.Contains(joined, " id") {
		t.Errorf("fields with defaults or generated ids should not be required: %q", joined)
	}
}

func TestSessionAssignmentIsChecked(t *testing.T) {
	ok := `
action sign_in
  allow true
  do
    set session.user = User[1]`
	if got := codes(t, ok); len(got) > 0 {
		t.Errorf("assigning a User to session.user should check clean, got %v", got)
	}

	wrong := strings.Replace(ok, "User[1]", "Task[1]", 1)
	if got := codes(t, wrong); !hasCode(got, "K030") {
		t.Errorf("assigning a Task to session.user should be K030, got %v", got)
	}
}

func TestEventValueIsTyped(t *testing.T) {
	src := `
action toggle
  in
    id ref Task
    done bool
  allow true
  do
    set Task[id].done = done

route r
  path /r
  data
    tasks many Task where rank > 0
  view
    page
      each tasks as t
        check value=t.done do=toggle id=t.id done=$value`
	if got := codes(t, src); len(got) > 0 {
		t.Errorf("a check's $value is a bool and should satisfy a bool parameter, got %v", got)
	}

	// $value outside an element that fires one is meaningless.
	stray := `
action a
  allow true
  do
    set Task[1].done = $value`
	if got := codes(t, stray); !hasCode(got, "K021") {
		t.Errorf("want K021 for a stray $value, got %v", got)
	}
}

func TestDuplicateNames(t *testing.T) {
	for _, src := range []string{
		"\ntable Task\n  id id\n",
		"\nroute r\n  path /r\n  view\n    page\n\nroute r2\n  path /r\n  view\n    page\n",
	} {
		if got := codes(t, src); !hasCode(got, "K042") {
			t.Errorf("want K042 for %q, got %v", strings.TrimSpace(src), got)
		}
	}
}

// Every diagnostic must name a fix and a real code.
func TestEveryDiagnosticIsWellFormed(t *testing.T) {
	srcs := []string{
		"\nroute r\n  path /r\n  view\n    page\n      text task.titel\n",
		"\naction a\n  allow session.nope == 1\n  do\n    set Nope[1].x = 1\n",
		"\nroute r\n  path /r\n  view\n    page\n      paragraph \"x\" wobble=1 style=nope\n",
	}
	for _, src := range srcs {
		for _, g := range run(t, src) {
			if strings.TrimSpace(g.Fix) == "" {
				t.Errorf("%s has no fix: %s", g.Code, g.Msg)
			}
			if _, ok := diag.Lookup(g.Code); !ok {
				t.Errorf("%s has no entry in the explain table", g.Code)
			}
		}
	}
}

// A link to a record is the most ordinary thing a web page does. It has to be
// expressible, and the way it is spelled wrong has to be caught — the version
// in quotes renders the same dead link on every row and used to pass.
func TestRecordLinks(t *testing.T) {
	route := func(to string) string {
		return `
route listing
  path /listing
  data
    tasks many Task where rank >= 0
  view
    page
      each tasks as t
        link t.title to=` + to + `

route detail
  path /d/:id
  view
    page
      head 1 "Detail"
`
	}

	if got := codes(t, route(`"/d/" + t.id`)); len(got) > 0 {
		t.Errorf("building a path by concatenation should check clean, got %v", got)
	}
	if got := codes(t, route(`"/d/" + t.title`)); len(got) > 0 {
		t.Errorf("concatenating text should check clean, got %v", got)
	}
	if got := codes(t, route(`"/d/t.id"`)); !hasCode(got, "K026") {
		t.Errorf("a field reference inside a quoted path should be K026, got %v", got)
	}
	// A real filename is not a botched interpolation: the difference is whether
	// the part before the dot is bound in scope.
	if got := codes(t, route(`"/robots.txt"`)); hasCode(got, "K026") {
		t.Errorf("/robots.txt is not an interpolation, got %v", got)
	}
	// Dead links are still caught, and concrete paths still resolve.
	if got := codes(t, route(`"/nowhere"`)); !hasCode(got, "K023") {
		t.Errorf("a dead link should still be K023, got %v", got)
	}
	if got := codes(t, route(`"/d/1"`)); len(got) > 0 {
		t.Errorf("a concrete path should resolve, got %v", got)
	}
}

// The K026 message has to name the repair, since the whole problem is that the
// broken form looks right.
func TestK026NamesTheRepair(t *testing.T) {
	src := `
route listing
  path /listing
  data
    tasks many Task where rank >= 0
  view
    page
      each tasks as t
        link t.title to="/d/t.id"

route detail
  path /d/:id
  view
    page
      head 1 "Detail"
`
	for _, g := range run(t, src) {
		if g.Code != "K026" {
			continue
		}
		if !strings.Contains(g.Fix, `"/d/" + t.id`) {
			t.Errorf("the fix should spell out the concatenation, got %q", g.Fix)
		}
		return
	}
	t.Fatal("no K026 reported")
}

// A timestamp has no literal form, so a fixture spells one as text. Without
// that, no time-ordered query could be tested.
func TestTimestampsCanBeSeededAsText(t *testing.T) {
	src := `
test "ordering"
  seed Task id=1 title="a" rank=0 created="2026-01-01"
  expect Task[1].title == "a"
`
	if got := codes(t, src); len(got) > 0 {
		t.Errorf("seeding an at field from a date string should check clean, got %v", got)
	}
}

// A schema field could be nullable while the parameter feeding it could not,
// so an empty form field stored "" and every reader had to handle two empties.
func TestNullableParameters(t *testing.T) {
	nullable := `
action note
  in
    id ref Task
    body text? max 200
  allow true
  do
    set Task[id].title = coalesce(body, "none")

route r
  path /r
  view
    page
      button "Note" do=note id=1 body=null`
	if got := codes(t, nullable); len(got) > 0 {
		t.Errorf("null should be passable to a nullable parameter, got %v", got)
	}

	// Without the marker, null is refused and the message says how to allow it.
	strict := strings.Replace(nullable, "body text? max 200", "body text max 200", 1)
	found := false
	for _, g := range run(t, strict) {
		if g.Code != "K030" {
			continue
		}
		found = true
		if !strings.Contains(g.Fix, "body text?") {
			t.Errorf("the fix should name the marker, got %q", g.Fix)
		}
	}
	if !found {
		t.Errorf("passing null to a non-nullable parameter should be K030, got %v", codes(t, strict))
	}
}

func TestNullableParameterSurvivesFormatting(t *testing.T) {
	src := "action a\n  in\n    body text? max 200\n  allow true\n  do\n    toast \"x\"\n"
	var d diag.List
	p := parse.Program([]parse.File{{Path: "t.kiln", Src: src}}, &d)
	if !d.Empty() {
		shown, _ := d.Resolved()
		t.Fatalf("did not parse: %+v", shown)
	}
	param := p.Actions[0].In[0]
	if !param.Nullable {
		t.Error("the ? was lost in parsing")
	}
	if param.Max != 200 {
		t.Errorf("max = %d, want 200", param.Max)
	}
}

// One missing guard is one mistake. Reporting it once per action that trips
// over it buries the cause under its own consequences.
func TestUnguardedRouteReportsOnceNamingEveryAction(t *testing.T) {
	src := `
action edit
  in
    id ref Task
  allow session.user == Task[id].project.owner
  do
    set Task[id].done = true

action wipe
  in
    id ref Task
  allow session.user == Task[id].project.owner
  do
    del Task[id]

route r
  path /r
  view
    page
      button "Edit" do=edit id=1
      button "Wipe" do=wipe id=1`

	var found []diag.Diag
	for _, g := range run(t, src) {
		if g.Code == "K040" {
			found = append(found, g)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want 1 K040 for 1 missing guard, got %d: %+v", len(found), found)
	}
	for _, action := range []string{"edit", "wipe"} {
		if !strings.Contains(found[0].Msg, action) {
			t.Errorf("the message should name %s: %q", action, found[0].Msg)
		}
	}
	// The insecure repair may be mentioned, but never as an equal option.
	if !strings.Contains(found[0].Fix, "removes the permission") {
		t.Errorf("the fix must say what widening the rule costs: %q", found[0].Fix)
	}
	if !strings.HasPrefix(found[0].Fix, "add `guard") {
		t.Errorf("the fix should lead with the guard: %q", found[0].Fix)
	}
}

// Two routes each missing a guard are two mistakes, not one.
func TestUnguardedRoutesAreNotCollapsedAcrossRoutes(t *testing.T) {
	src := `
action edit
  in
    id ref Task
  allow session.user == Task[id].project.owner
  do
    set Task[id].done = true

route one
  path /one
  view
    page
      button "Edit" do=edit id=1

route two
  path /two
  view
    page
      button "Edit" do=edit id=1`
	var n int
	for _, g := range run(t, src) {
		if g.Code == "K040" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("want one K040 per unguarded route, got %d", n)
	}
}

// The depth limit is about the view tree. route > view > page is fixed
// overhead every route pays, and counting it rejected ordinary layouts.
func TestViewDepthIsMeasuredFromThePage(t *testing.T) {
	body := func(levels int) string {
		var b strings.Builder
		b.WriteString("\nroute r\n  path /r\n  data\n    tasks many Task where rank >= 0\n  view\n    page\n")
		indent := "      "
		for i := 0; i < levels-1; i++ {
			b.WriteString(indent + "col\n")
			indent += "  "
		}
		b.WriteString(indent + "text \"deep\"\n")
		return b.String()
	}

	if got := codes(t, body(maxViewDepth)); hasCode(got, "K012") {
		t.Errorf("a view %d levels below page should be allowed, got %v", maxViewDepth, got)
	}
	if got := codes(t, body(maxViewDepth+1)); !hasCode(got, "K012") {
		t.Errorf("a view %d levels below page should be rejected, got %v", maxViewDepth+1, got)
	}
}

// The layout that made this cap wrong twice: a list of cards, each holding a
// row of controls with something underneath.
func TestOrdinaryNestedLayoutIsAllowed(t *testing.T) {
	src := `
action edit
  in
    id ref Task
  allow true
  do
    set Task[id].done = true

route r
  path /r
  data
    tasks many Task where rank >= 0
  view
    page
      card
        col gap=2
          each tasks as t
            row gap=2
              text t.title
              button "Edit" do=edit id=t.id`
	if got := codes(t, src); len(got) > 0 {
		t.Errorf("a card holding a list of rows should check clean, got %v", got)
	}
}
