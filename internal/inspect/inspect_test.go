package inspect

import (
	"strings"
	"testing"

	"kiln/internal/ast"
	"kiln/internal/diag"
	"kiln/internal/parse"
	"kiln/internal/project"
)

func example(t *testing.T) *ast.Program {
	t.Helper()
	files, err := project.Load("../../examples/tasks")
	if err != nil {
		t.Fatal(err)
	}
	var d diag.List
	p := parse.Program(files, &d)
	if !d.Empty() {
		shown, _ := d.Resolved()
		t.Fatalf("example did not parse: %+v", shown)
	}
	return p
}

// The orientation read has to stay short enough to be worth reading instead of
// opening the files.
func TestMapIsCompact(t *testing.T) {
	out := Map(example(t))
	if n := strings.Count(out, "\n"); n > 60 {
		t.Errorf("map is %d lines; it is meant to replace reading the program", n)
	}
	for _, want := range []string{
		"app tasks", "session.user ref User",
		"TABLES", "Task", "project→Project", "index(project, rank)",
		"ACTIONS", "toggle_task", "set Task.done",
		"ROUTES", "/projects/:id", "guard session.user", "data(project one Project",
		"TESTS", "owner toggles a task",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("map is missing %q:\n%s", want, out)
		}
	}
}

// A route's outline must name the actions its view can invoke — that link is
// the one an agent otherwise has to read the whole view to find.
func TestMapNamesBoundActions(t *testing.T) {
	out := Map(example(t))
	for _, want := range []string{"add_task", "delete_task", "toggle_task"} {
		if !strings.Contains(out, want) {
			t.Errorf("map does not link %s to its route:\n%s", want, out)
		}
	}
}

func TestWhereFindsFieldAcrossTheProgram(t *testing.T) {
	got := Where(example(t), "Task.done")
	if len(got) == 0 {
		t.Fatal("found nothing")
	}
	if !got[0].Defined || !strings.Contains(got[0].What, "field of Task") {
		t.Errorf("the definition should come first, got %+v", got[0])
	}
	var files []string
	for _, o := range got {
		files = append(files, o.Pos.File)
	}
	joined := strings.Join(files, " ")
	for _, want := range []string{"schema/", "actions/", "routes/", "tests/"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Task.done should be traced into %s, got %v", want, files)
		}
	}
}

func TestWhereDistinguishesDefinitionFromUse(t *testing.T) {
	got := Where(example(t), "toggle_task")
	var defs, uses int
	for _, o := range got {
		if o.Defined {
			defs++
		} else {
			uses++
		}
	}
	if defs != 1 {
		t.Errorf("want exactly 1 definition, got %d", defs)
	}
	if uses < 2 {
		t.Errorf("want the view binding and the test call, got %d uses", uses)
	}
}

// A bare field name matches in any table; a qualified one only in its own.
func TestWhereQualifiedVersusBare(t *testing.T) {
	p := example(t)
	bare := Where(p, "title")
	qualified := Where(p, "Task.title")
	if len(bare) <= len(qualified) {
		t.Errorf("a bare field name should match more widely: bare=%d qualified=%d",
			len(bare), len(qualified))
	}
	for _, o := range qualified {
		if o.Defined && !strings.Contains(o.What, "field of Task") {
			t.Errorf("Task.title matched a definition in another table: %+v", o)
		}
	}
}

func TestWhereFindsTablesAndRoutes(t *testing.T) {
	p := example(t)
	if got := Where(p, "Project"); len(got) < 2 {
		t.Errorf("Project should be defined and referenced, got %+v", got)
	}
	if got := Where(p, "/login"); len(got) == 0 {
		t.Error("a route path should resolve")
	}
	if got := Where(p, "session"); len(got) != 0 {
		_ = got // "session" is a scope, not a symbol; matching nothing is fine
	}
}

func TestWhereOnNothing(t *testing.T) {
	if got := Where(example(t), "Nonexistent"); len(got) != 0 {
		t.Errorf("want no occurrences, got %+v", got)
	}
	if !strings.Contains(Render("Nonexistent", nil), "nothing named") {
		t.Error("want a plain message when a symbol does not exist")
	}
}
