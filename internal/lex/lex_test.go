package lex

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kiln/internal/diag"
)

func lexOK(t *testing.T, src string) []*Line {
	t.Helper()
	var d diag.List
	lines := Lex("t.kiln", src, &d)
	if !d.Empty() {
		shown, _ := d.Resolved()
		t.Fatalf("unexpected diagnostics: %+v", shown)
	}
	return lines
}

func codesFrom(src string) []string {
	var d diag.List
	Lex("t.kiln", src, &d)
	shown, _ := d.Resolved()
	var out []string
	for _, s := range shown {
		out = append(out, s.Code)
	}
	return out
}

func TestIndentationTree(t *testing.T) {
	lines := lexOK(t, `route project_detail
  path /projects/:id
  view
    page title=project.title
      head 2 "Tasks"
`)
	if len(lines) != 1 {
		t.Fatalf("want 1 top-level line, got %d", len(lines))
	}
	route := lines[0]
	if route.Head() != "route" {
		t.Errorf("head = %q", route.Head())
	}
	if len(route.Children) != 2 {
		t.Fatalf("route should have path and view, got %d children", len(route.Children))
	}
	view, ok := route.Child("view")
	if !ok {
		t.Fatal("no view child")
	}
	page := view.Children[0]
	if page.Head() != "page" || page.Depth != 2 {
		t.Errorf("page: head=%q depth=%d", page.Head(), page.Depth)
	}
	if len(page.Children) != 1 || page.Children[0].Head() != "head" {
		t.Errorf("page children = %+v", page.Children)
	}
}

func TestPathAndVarTokens(t *testing.T) {
	lines := lexOK(t, "check value=t.done do=toggle_task done=$value\n")
	toks := lines[0].Tokens
	last := toks[len(toks)-1]
	if last.Kind != Var || last.Text != "value" {
		t.Errorf("want $value as Var, got %v %q", last.Kind, last.Text)
	}

	lines = lexOK(t, "path /projects/:id\n")
	if got := lines[0].Tokens[1]; got.Kind != Path || got.Text != "/projects/:id" {
		t.Errorf("want a Path token, got %v %q", got.Kind, got.Text)
	}
}

func TestDivisionIsNotAPath(t *testing.T) {
	lines := lexOK(t, "text a / b\n")
	if got := lines[0].Tokens[2]; got.Kind != Op || got.Text != "/" {
		t.Errorf("spaced / should be division, got %v %q", got.Kind, got.Text)
	}
}

func TestComments(t *testing.T) {
	lines := lexOK(t, "# whole line\ntext \"a # b\"  # trailing\n")
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(lines))
	}
	if got := lines[0].Tokens[1].Text; got != "a # b" {
		t.Errorf("# inside a string was stripped: %q", got)
	}
	if len(lines[0].Tokens) != 2 {
		t.Errorf("trailing comment survived: %+v", lines[0].Tokens)
	}
}

// Sign belongs to the expression parser, not the lexer, so that spacing never
// decides whether "-" means negation or subtraction.
func TestMinusIsAlwaysAnOperator(t *testing.T) {
	for _, src := range []string{"text -5\n", "text a - 5\n", "text a -5\n"} {
		lines := lexOK(t, src)
		var sawMinus bool
		for _, tok := range lines[0].Tokens {
			if tok.Kind == Number && strings.HasPrefix(tok.Text, "-") {
				t.Errorf("%q: lexer produced a signed number %q", src, tok.Text)
			}
			if tok.IsOp("-") {
				sawMinus = true
			}
		}
		if !sawMinus {
			t.Errorf("%q: no minus operator produced", src)
		}
	}
}

func TestStructuralErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"tab", "route a\n\tpath /x\n", "K001"},
		{"odd indent", "route a\n   path /x\n", "K002"},
		{"over indent", "route a\n      path /x\n", "K003"},
		{"unterminated string", "text \"oops\n", "K004"},
		{"stray character", "text a & b\n", "K005"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := codesFrom(c.src)
			if len(got) == 0 || got[0] != c.want {
				t.Errorf("want %s, got %v", c.want, got)
			}
		})
	}
}

func TestDepthCap(t *testing.T) {
	var b strings.Builder
	b.WriteString("route a\n")
	for i := 1; i <= MaxDepth+1; i++ {
		b.WriteString(strings.Repeat(" ", i*IndentWidth) + "row\n")
	}
	got := codesFrom(b.String())
	if len(got) == 0 || got[0] != "K012" {
		t.Errorf("want K012 past depth %d, got %v", MaxDepth, got)
	}
}

func TestLineCap(t *testing.T) {
	src := strings.Repeat("text \"x\"\n", MaxLines+1)
	if got := codesFrom(src); len(got) == 0 || got[0] != "K013" {
		t.Errorf("want K013 past %d lines, got %v", MaxLines, got)
	}
	// Blank lines and comments should not count toward the cap.
	spaced := strings.Repeat("text \"x\"\n\n# note\n", MaxLines/2)
	for _, c := range codesFrom(spaced) {
		if c == "K013" {
			t.Error("blank lines and comments should not count toward the line cap")
		}
	}
}

// The example program is the target the rest of the toolchain builds toward,
// so it must at least lex cleanly at every stage.
func TestExamplesLexClean(t *testing.T) {
	root := filepath.Join("..", "..", "examples")
	err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || filepath.Ext(path) != ".kiln" {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var d diag.List
		lines := Lex(path, string(src), &d)
		if !d.Empty() {
			shown, _ := d.Resolved()
			t.Errorf("%s: %+v", path, shown)
		}
		if len(lines) == 0 {
			t.Errorf("%s: lexed to nothing", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
