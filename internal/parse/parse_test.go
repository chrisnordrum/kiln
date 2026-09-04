package parse

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kiln/internal/ast"
	"kiln/internal/diag"
)

// exampleFiles loads the example program that the toolchain is built toward.
func exampleFiles(t *testing.T) []File {
	t.Helper()
	var out []File
	root := filepath.Join("..", "..", "examples", "tasks")
	err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || filepath.Ext(path) != ".kiln" {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, File{Path: path, Src: string(src)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func parseExample(t *testing.T) (*ast.Program, *diag.List) {
	t.Helper()
	var d diag.List
	p := Program(exampleFiles(t), &d)
	return p, &d
}

func TestExampleParsesClean(t *testing.T) {
	p, d := parseExample(t)
	if !d.Empty() {
		shown, _ := d.Resolved()
		for _, s := range shown {
			t.Errorf("%s:%d: %s: %s", s.File, s.Line, s.Code, s.Msg)
		}
		t.FailNow()
	}
	if p.App == nil {
		t.Fatal("no app declaration parsed")
	}
	if got := len(p.Tables); got != 3 {
		t.Errorf("want 3 tables (User, Project, Task), got %d", got)
	}
	if got := len(p.Actions); got != 4 {
		t.Errorf("want 4 actions, got %d", got)
	}
	if got := len(p.Routes); got != 2 {
		t.Errorf("want 2 routes, got %d", got)
	}
	if got := len(p.Tests); got != 2 {
		t.Errorf("want 2 tests, got %d", got)
	}
}

func TestSchemaDetail(t *testing.T) {
	p, _ := parseExample(t)
	task, ok := p.Table("Task")
	if !ok {
		t.Fatal("no Task table")
	}
	title, ok := task.Field("title")
	if !ok {
		t.Fatal("Task has no title field")
	}
	if title.Type != "text" || title.Max != 200 {
		t.Errorf("title = %+v, want text max 200", title)
	}
	project, _ := task.Field("project")
	if project.Type != "ref" || project.Ref != "Project" || project.OnDelete != "cascade" {
		t.Errorf("project = %+v, want ref Project on delete cascade", project)
	}
	done, _ := task.Field("done")
	if done.Type != "bool" || done.Default == nil || done.Default.String() != "false" {
		t.Errorf("done = %+v, want bool defaulting to false", done)
	}
	created, _ := task.Field("created")
	if created.Type != "at" || !created.DefaultNow {
		t.Errorf("created = %+v, want at now", created)
	}
	if len(task.Indexes) != 1 || strings.Join(task.Indexes[0], ",") != "project,rank" {
		t.Errorf("indexes = %v", task.Indexes)
	}
	user, _ := p.Table("User")
	email, _ := user.Field("email")
	if !email.Unique {
		t.Error("User.email should be unique")
	}
}

func TestActionDetail(t *testing.T) {
	p, _ := parseExample(t)
	a, ok := p.Action("toggle_task")
	if !ok {
		t.Fatal("no toggle_task action")
	}
	if len(a.In) != 2 {
		t.Fatalf("want 2 params, got %d", len(a.In))
	}
	id, _ := a.Param("id")
	if id.Type != "ref" || id.Ref != "Task" {
		t.Errorf("id param = %+v", id)
	}
	if a.Allow == nil || a.Allow.String() != "session.user == Task[id].project.owner" {
		t.Errorf("allow = %v", a.Allow)
	}
	if len(a.Do) != 1 || a.Do[0].StmtKind() != "set" {
		t.Errorf("do = %+v", a.Do)
	}
	if len(a.After) != 1 || a.After[0].StmtKind() != "refresh" {
		t.Errorf("after = %+v", a.After)
	}
}

func TestRouteDetail(t *testing.T) {
	p, _ := parseExample(t)
	r, ok := p.Route("project_detail")
	if !ok {
		t.Fatal("no project_detail route")
	}
	if r.Name != "project_detail" || r.Path != "/projects/:id" {
		t.Errorf("route = %s %s", r.Name, r.Path)
	}
	if r.Guard == nil || r.Guard.Redirect != "/login" {
		t.Errorf("guard = %+v", r.Guard)
	}
	tasks, ok := r.Query("tasks")
	if !ok {
		t.Fatal("no tasks data key")
	}
	if tasks.Card != "many" || tasks.Table != "Task" {
		t.Errorf("tasks = %+v", tasks)
	}
	if len(tasks.Order) != 1 || tasks.Order[0].Field != "rank" || tasks.Order[0].Desc {
		t.Errorf("order = %+v", tasks.Order)
	}
	if tasks.Where == nil || tasks.Where.String() != "project == params.id" {
		t.Errorf("where = %v", tasks.Where)
	}
}

func TestViewTree(t *testing.T) {
	p, _ := parseExample(t)
	r, ok := p.Route("project_detail")
	if !ok {
		t.Fatal("no project_detail route")
	}
	view := r.View
	if len(view) != 1 || view[0].Kind != "page" {
		t.Fatalf("view root = %+v", view)
	}
	page := view[0]
	if _, ok := page.Attr("title"); !ok {
		t.Error("page has no title attribute")
	}

	var each, when *ast.Node
	for _, ch := range page.Children {
		switch ch.Kind {
		case "each":
			each = ch
		case "when":
			when = ch
		}
	}
	if when == nil || when.Cond == nil {
		t.Fatal("no when node with a condition")
	}
	if len(when.Children) != 1 || when.Children[0].Kind != "empty" {
		t.Errorf("when children = %+v", when.Children)
	}
	if each == nil {
		t.Fatal("no each node")
	}
	if each.Var != "t" || each.List.String() != "tasks" {
		t.Errorf("each = list %v as %q", each.List, each.Var)
	}
	row := each.Children[0]
	if row.Kind != "row" {
		t.Fatalf("each child = %s", row.Kind)
	}
	check := row.Children[0]
	do, ok := check.Attr("do")
	if !ok || do.Value.String() != "toggle_task" {
		t.Errorf("check do = %+v", check.Attrs)
	}
	doneAttr, ok := check.Attr("done")
	if !ok || doneAttr.Value.String() != "$value" {
		t.Errorf("check should pass the event value, got %+v", check.Attrs)
	}
}

func TestTestsParse(t *testing.T) {
	p, _ := parseExample(t)
	first := p.Tests[0]
	if first.Name != "owner toggles a task" {
		t.Errorf("name = %q", first.Name)
	}
	var kinds []string
	for _, s := range first.Steps {
		kinds = append(kinds, s.Kind)
	}
	want := "seed seed seed seed as call expect expect"
	if got := strings.Join(kinds, " "); got != want {
		t.Errorf("steps = %q, want %q", got, want)
	}
	last := first.Steps[len(first.Steps)-1]
	if !last.Denied || last.As == nil || last.As.Value.String() != "2" {
		t.Errorf("last step = %+v", last)
	}
	second := p.Tests[1]
	visit := second.Steps[len(second.Steps)-2]
	if visit.Kind != "visit" || visit.Target != "/projects/1" {
		t.Errorf("visit = %+v", visit)
	}
	textExpect := second.Steps[len(second.Steps)-1]
	if textExpect.Text != "Ship it" {
		t.Errorf("text expectation = %+v", textExpect)
	}
}
