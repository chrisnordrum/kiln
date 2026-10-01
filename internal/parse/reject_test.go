package parse

import (
	"strings"
	"testing"

	"github.com/chrisnordrum/kiln/internal/diag"
)

// parseSrc parses a single synthetic file.
func parseSrc(src string) *diag.List {
	var d diag.List
	Program([]File{{Path: "t.kiln", Src: src}}, &d)
	return &d
}

// firstCode returns the first diagnostic code, or "" if it parsed clean.
func firstCode(src string) string {
	shown, _ := parseSrc(src).Resolved()
	if len(shown) == 0 {
		return ""
	}
	return shown[0].Code
}

func TestMalformedInputIsRejected(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"unknown top level", "widget foo\n  x 1\n", "K010"},
		{"table with no fields", "table Task\n", "K011"},
		{"field with no type", "table Task\n  title\n", "K010"},
		{"ref with no table", "table Task\n  owner ref\n", "K010"},
		{"bad field modifier", "table Task\n  title text wobble\n", "K010"},
		{"non numeric max", "table Task\n  title text max wide\n", "K030"},
		{"unknown block in action", "action a\n  allow true\n  do\n    refresh\n  wat\n    x\n", "K010"},
		{"action with no do", "action a\n  allow true\n", "K011"},
		{"action with no allow", "action a\n  do\n    refresh\n", "K011"},
		{"bad statement", "action a\n  allow true\n  do\n    frobnicate x\n", "K010"},
		{"set without a field", "action a\n  allow true\n  do\n    set Task[1] = 2\n", "K010"},
		{"route with no path", "route r\n  view\n    page\n", "K011"},
		{"route with no view", "route r\n  path /x\n", "K011"},
		{"guard with no else", "route r\n  path /x\n  guard session.user\n  view\n    page\n", "K011"},
		{"bad cardinality", "route r\n  path /x\n  data\n    a lots Task where id == 1\n  view\n    page\n", "K010"},
		{"bad query clause", "route r\n  path /x\n  data\n    a one Task where id == 1 sortby rank\n  view\n    page\n", "K010"},
		{"each without as", "route r\n  path /x\n  view\n    each tasks\n", "K010"},
		{"else with no when", "route r\n  path /x\n  view\n    page\n      else\n        text \"x\"\n", "K010"},
		{"test with no name", "test\n  seed Task id=1\n", "K010"},
		{"test with no steps", "test \"empty\"\n", "K011"},
		{"bad test step", "test \"t\"\n  wiggle Task\n", "K010"},
		{"expect text without a string", "test \"t\"\n  expect text 5\n", "K010"},
		{"positional after keyed", "route r\n  path /x\n  view\n    page\n      button do=go \"Label\"\n", "K010"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := firstCode(c.src); got != c.want {
				t.Errorf("want %s, got %q", c.want, got)
			}
		})
	}
}

// A misspelling should be named, not just rejected — that turns a
// guess-and-retry loop into one edit.
func TestSuggestionsAppearOnMisspellings(t *testing.T) {
	cases := []struct{ src, want string }{
		{"tabel Task\n  id id\n", "table"},
		{"route r\n  path /x\n  data\n    a onee Task where id == 1\n  view\n    page\n", "one"},
		{"action a\n  allow true\n  do\n    dell Task[1]\n", "del"},
	}
	for _, c := range cases {
		shown, _ := parseSrc(c.src).Resolved()
		if len(shown) == 0 {
			t.Errorf("%q parsed clean", c.src)
			continue
		}
		if !strings.Contains(strings.Join(shown[0].Near, " "), c.want) {
			t.Errorf("%q: want %q suggested, got %v", c.src, c.want, shown[0].Near)
		}
	}
}

// Every diagnostic the parser emits must carry a fix, not only a diagnosis.
func TestEveryParseDiagnosticHasAFix(t *testing.T) {
	srcs := []string{
		"widget foo\n", "table Task\n", "table Task\n  title\n",
		"action a\n", "route r\n", "test\n",
		"route r\n  path /x\n  view\n    each tasks\n",
	}
	for _, src := range srcs {
		shown, _ := parseSrc(src).Resolved()
		for _, s := range shown {
			if strings.TrimSpace(s.Fix) == "" {
				t.Errorf("%s at %s:%d has no fix: %s", s.Code, s.File, s.Line, s.Msg)
			}
		}
	}
}
