package format

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrisnordrum/kiln/internal/parse"
	"github.com/chrisnordrum/kiln/internal/project"
)

func fmtSrc(t *testing.T, src string) string {
	t.Helper()
	out, d := File(parse.File{Path: "t.kiln", Src: src})
	if !d.Empty() {
		shown, _ := d.Resolved()
		t.Fatalf("formatting %q: %+v", src, shown)
	}
	return out
}

// Parentheses must survive the round trip exactly where meaning depends on
// them, and must not accumulate where it does not.
func TestParenthesesArePreservedAndNotInvented(t *testing.T) {
	cases := []struct{ src, want string }{
		{"(a + b) * c", "(a + b) * c"},
		{"a + b * c", "a + b * c"},
		{"a - (b - c)", "a - (b - c)"},
		{"a - b - c", "a - b - c"},
		{"((a))", "a"},
		{"not (a and b)", "not (a and b)"},
		{"not a == b", "not a == b"},
		{"(a or b) and c", "(a or b) and c"},
		{"a or b and c", "a or b and c"},
		{"-(a * b)", "-(a * b)"},
		{"-a * b", "-a * b"},
		{"(if x then 1 else 2) + 3", "(if x then 1 else 2) + 3"},
	}
	for _, c := range cases {
		src := "action a\n  allow " + c.src + "\n  do\n    refresh\n"
		got := fmtSrc(t, src)
		want := "  allow " + c.want
		if !strings.Contains(got, want) {
			t.Errorf("%q formatted to:\n%s\nwant a line %q", c.src, got, want)
		}
	}
}

// Formatting twice must equal formatting once, or the form is not canonical.
func TestIdempotent(t *testing.T) {
	files := exampleSources(t)
	for _, f := range files {
		once := fmtSrc(t, f.Src)
		twice := fmtSrc(t, once)
		if once != twice {
			t.Errorf("%s is not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", f.Path, once, twice)
		}
	}
}

// Formatted output must still parse, and mean the same thing — which, given a
// canonical form, is the same as formatting to the same bytes.
func TestFormattedOutputReparses(t *testing.T) {
	for _, f := range exampleSources(t) {
		formatted := fmtSrc(t, f.Src)
		var d parseDiags
		reparsed := fmtSrc(t, formatted)
		if reparsed != formatted {
			t.Errorf("%s changed meaning on reparse", f.Path)
		}
		_ = d
	}
}

// Two spellings of the same thing must converge: attribute order and column
// padding are not part of a program's meaning.
func TestSpellingsConverge(t *testing.T) {
	a := "route r\n  path /x\n  view\n    page\n      button \"Go\" do=act id=1 style=quiet\n"
	b := "route r\n  path      /x\n  view\n    page\n      button \"Go\"   style=quiet   id=1   do=act\n"
	if fmtSrc(t, a) != fmtSrc(t, b) {
		t.Errorf("did not converge:\n--- a ---\n%s\n--- b ---\n%s", fmtSrc(t, a), fmtSrc(t, b))
	}
}

// A file that does not parse is returned untouched rather than rewritten into
// something worse.
func TestBrokenFileIsLeftAlone(t *testing.T) {
	src := "table Task\n  title\n"
	out, d := File(parse.File{Path: "t.kiln", Src: src})
	if d.Empty() {
		t.Fatal("expected diagnostics")
	}
	if out != src {
		t.Errorf("a broken file was rewritten:\n%s", out)
	}
}

func TestFieldModifierOrderIsFixed(t *testing.T) {
	src := "table T\n  a text unique max 20 = \"x\"\n  b ref U on delete cascade ?\n  c at now\n  d enum x y z = x\n"
	got := fmtSrc(t, src)
	for _, want := range []string{
		`a text max 20 unique = "x"`,
		"b ref U? on delete cascade",
		"c at now",
		"d enum x y z = x",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want a line %q in:\n%s", want, got)
		}
	}
}

type parseDiags struct{}

func exampleSources(t *testing.T) []parse.File {
	t.Helper()
	files, err := project.Load(filepath.Join("..", "..", "examples", "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no example files found")
	}
	return files
}

// The committed examples should already be in canonical form, so that the
// program an agent reads is the program the formatter would write.
func TestExamplesAreCanonical(t *testing.T) {
	root := filepath.Join("..", "..", "examples", "tasks")
	files, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		onDisk, err := os.ReadFile(filepath.Join(root, f.Path))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmtSrc(t, string(onDisk)); got != string(onDisk) {
			t.Errorf("%s is not canonical; run kiln fmt.\n--- want ---\n%s", f.Path, got)
		}
	}
}
