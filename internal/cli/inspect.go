package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"kiln/internal/ast"
	"kiln/internal/diag"
	"kiln/internal/inspect"
	"kiln/internal/parse"
	"kiln/internal/project"
)

// loadParsed parses without checking. Orientation and scaffolding are most
// useful on a program that does not yet check, which is exactly when an agent
// is looking around.
func loadParsed(args []string, errw io.Writer) (*ast.Program, string, error) {
	root, err := rootFrom(args)
	if err != nil {
		return nil, "", err
	}
	files, err := project.Load(root)
	if err != nil {
		return nil, "", err
	}
	var d diag.List
	return parse.Program(files, &d), root, nil
}

func runMap(args []string, out, errw io.Writer) error {
	fs := flag.NewFlagSet("map", flag.ContinueOnError)
	fs.SetOutput(errw)
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, _, err := loadParsed(fs.Args(), errw)
	if err != nil {
		return err
	}
	fmt.Fprint(out, inspect.Map(p))
	return nil
}

func runWhere(args []string, out, errw io.Writer) error {
	fs := flag.NewFlagSet("where", flag.ContinueOnError)
	fs.SetOutput(errw)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: kiln where <symbol> [project]\n" +
			"a table, field (Task.title), action, route, data key, session field or effect")
	}
	sym := fs.Arg(0)
	p, _, err := loadParsed(fs.Args()[1:], errw)
	if err != nil {
		return err
	}
	fmt.Fprint(out, inspect.Render(sym, inspect.Where(p, sym)))
	return nil
}

func runNew(args []string, out, errw io.Writer) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(errw)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: kiln new table|action|route <name> [project]")
	}
	kind, name := fs.Arg(0), fs.Arg(1)
	p, root, err := loadParsed(fs.Args()[2:], errw)
	if err != nil {
		return err
	}

	dir, body, err := scaffold(p, kind, name)
	if err != nil {
		return err
	}
	rel := filepath.Join(dir, fileName(name)+project.Ext)
	path := filepath.Join(root, rel)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", rel)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(out, rel)
	return nil
}

// scaffold returns a skeleton that already checks clean, so the first thing an
// agent does after creating a file is edit real code rather than repair a
// template.
func scaffold(p *ast.Program, kind, name string) (dir, body string, err error) {
	switch kind {
	case "table":
		return "schema", fmt.Sprintf("table %s\n  id id\n  title text max 200\n  created at now\n", name), nil

	case "action":
		return "actions", fmt.Sprintf("action %s\n  allow %s\n  do\n    toast \"%s is not implemented yet\"\n",
			name, sessionGuard(p), name), nil

	case "route":
		title := strings.ReplaceAll(name, "_", " ")
		return "routes", fmt.Sprintf("route %s\n  path /%s\n  view\n    page title=%q\n      head 1 %q\n",
			name, strings.ReplaceAll(name, "_", "-"), title, title), nil
	}
	return "", "", fmt.Errorf("kiln new takes table, action or route, not %q", kind)
}

// sessionGuard writes an allow rule against whatever session field the app
// actually declares, rather than assuming one is called user.
func sessionGuard(p *ast.Program) string {
	if p.App != nil && len(p.App.Session) > 0 {
		return "session." + p.App.Session[0].Name + " != null"
	}
	return "true"
}

// fileName converts a declaration name to a file name.
func fileName(name string) string {
	var b strings.Builder
	for i, r := range name {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r + 32)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
