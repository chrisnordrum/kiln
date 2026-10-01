package parse

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chrisnordrum/kiln/internal/ast"
	"github.com/chrisnordrum/kiln/internal/diag"
	"github.com/chrisnordrum/kiln/internal/lex"
)

// parseOneExpr parses a bare expression, for tests.
func parseOneExpr(t *testing.T, src string) (ast.Expr, *diag.List) {
	t.Helper()
	var d diag.List
	lines := lex.Lex("t.kiln", src+"\n", &d)
	if len(lines) == 0 {
		t.Fatalf("%q lexed to nothing", src)
	}
	c := &cursor{toks: lines[0].Tokens, file: "t.kiln", d: &d}
	return parseExpr(c), &d
}

// parens renders an expression fully parenthesized, so precedence is visible.
func parens(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Binary:
		return fmt.Sprintf("(%s %s %s)", parens(x.L), x.Op, parens(x.R))
	case *ast.Unary:
		return fmt.Sprintf("(%s %s)", x.Op, parens(x.X))
	case *ast.Cond:
		return fmt.Sprintf("(if %s then %s else %s)", parens(x.If), parens(x.Then), parens(x.Else))
	case *ast.Call:
		var args []string
		for _, a := range x.Args {
			args = append(args, parens(a))
		}
		return fmt.Sprintf("%s(%s)", x.Fn, strings.Join(args, ", "))
	default:
		return e.String()
	}
}

func TestPrecedence(t *testing.T) {
	cases := []struct{ src, want string }{
		{"a and b or c", "((a and b) or c)"},
		{"a or b and c", "(a or (b and c))"},
		{"a == b and c == d", "((a == b) and (c == d))"},
		{"not a == b", "(not (a == b))"},
		{"a + b * c", "(a + (b * c))"},
		{"(a + b) * c", "((a + b) * c)"},
		{"a - b - c", "((a - b) - c)"},
		{"-5 + 3", "((- 5) + 3)"},
	}
	for _, c := range cases {
		got, d := parseOneExpr(t, c.src)
		if !d.Empty() {
			shown, _ := d.Resolved()
			t.Errorf("%q: %+v", c.src, shown)
			continue
		}
		if p := parens(got); p != c.want {
			t.Errorf("%q parsed as %s, want %s", c.src, p, c.want)
		}
	}
}

func TestKeyedLookupWithFieldWalk(t *testing.T) {
	got, d := parseOneExpr(t, "session.user == Task[id].project.owner")
	if !d.Empty() {
		shown, _ := d.Resolved()
		t.Fatalf("%+v", shown)
	}
	bin, ok := got.(*ast.Binary)
	if !ok {
		t.Fatalf("want a comparison, got %T", got)
	}
	name, ok := bin.L.(*ast.Name)
	if !ok || name.String() != "session.user" {
		t.Errorf("left = %v (%T)", bin.L, bin.L)
	}
	idx, ok := bin.R.(*ast.Index)
	if !ok {
		t.Fatalf("right = %T, want an Index", bin.R)
	}
	if idx.Table != "Task" {
		t.Errorf("table = %q", idx.Table)
	}
	if strings.Join(idx.Path, ".") != "project.owner" {
		t.Errorf("path = %v", idx.Path)
	}
	if idx.Key.String() != "id" {
		t.Errorf("key = %v", idx.Key)
	}
}

func TestCallsAndLiterals(t *testing.T) {
	cases := []struct{ src, want string }{
		{"count(tasks) == 0", "(count(tasks) == 0)"},
		{"truncate(title, 20)", "truncate(title, 20)"},
		{`plural(n, "task", "tasks")`, `plural(n, "task", "tasks")`},
		{"coalesce(a, b)", "coalesce(a, b)"},
		{"done == true", "(done == true)"},
		{"notes == null", "(notes == null)"},
		{"if done then 1 else 0", "(if done then 1 else 0)"},
	}
	for _, c := range cases {
		got, d := parseOneExpr(t, c.src)
		if !d.Empty() {
			shown, _ := d.Resolved()
			t.Errorf("%q: %+v", c.src, shown)
			continue
		}
		if p := parens(got); p != c.want {
			t.Errorf("%q parsed as %s, want %s", c.src, p, c.want)
		}
	}
}

func TestEventVar(t *testing.T) {
	got, d := parseOneExpr(t, "$value")
	if !d.Empty() {
		t.Fatal("unexpected diagnostics")
	}
	if v, ok := got.(*ast.EventVar); !ok || v.Name != "value" {
		t.Errorf("want $value as an EventVar, got %T %v", got, got)
	}
}

func TestSyntaxErrorsAreReported(t *testing.T) {
	for _, src := range []string{
		"a ==",
		"if x then 1",
		"count(",
		"a . ",
		"* 3",
	} {
		_, d := parseOneExpr(t, src)
		if d.Empty() {
			t.Errorf("%q should not have parsed clean", src)
		}
	}
}

// An if expression always has an else, so it always has a value; the error
// should say so rather than just naming a missing token.
func TestIfWithoutElseExplainsItself(t *testing.T) {
	_, d := parseOneExpr(t, "if x then 1")
	shown, _ := d.Resolved()
	if len(shown) == 0 || !strings.Contains(shown[0].Fix, "else") {
		t.Errorf("want a fix mentioning else, got %+v", shown)
	}
}
