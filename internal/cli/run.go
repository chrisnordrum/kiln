package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chrisnordrum/kiln/internal/ast"
	"github.com/chrisnordrum/kiln/internal/check"
	"github.com/chrisnordrum/kiln/internal/diag"
	"github.com/chrisnordrum/kiln/internal/parse"
	"github.com/chrisnordrum/kiln/internal/project"
	"github.com/chrisnordrum/kiln/internal/runner"
)

// loadChecked parses and checks a project, reporting diagnostics rather than
// running a program that has not been verified.
func loadChecked(args []string, errw io.Writer) (*ast.Program, string, error) {
	root, err := rootFrom(args)
	if err != nil {
		return nil, "", err
	}
	files, err := project.Load(root)
	if err != nil {
		return nil, "", err
	}
	var d diag.List
	p := parse.Program(files, &d)
	if d.Empty() {
		check.Program(p, &d)
	}
	if !d.Empty() {
		if err := d.Render(errw, false); err != nil {
			return nil, "", err
		}
		return nil, "", errQuiet
	}
	return p, root, nil
}

func runTest(args []string, out, errw io.Writer) error {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(errw)
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, _, err := loadChecked(fs.Args(), errw)
	if err != nil {
		return err
	}
	if len(p.Tests) == 0 {
		fmt.Fprintln(out, "no tests")
		return nil
	}

	var failed int
	for _, res := range runner.RunTests(p) {
		if res.OK() {
			fmt.Fprintf(out, "ok    %s\n", res.Name)
			continue
		}
		failed++
		fmt.Fprintf(out, "FAIL  %s\n", res.Name)
		for _, f := range res.Failures {
			fmt.Fprintf(out, "        %s\n", f)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d tests failed", failed, len(p.Tests))
	}
	fmt.Fprintf(out, "\n%d tests passed\n", len(p.Tests))
	return nil
}

func runSnap(args []string, out, errw io.Writer) error {
	fs := flag.NewFlagSet("snap", flag.ContinueOnError)
	fs.SetOutput(errw)
	checkOnly := fs.Bool("check", false, "compare against committed snapshots instead of writing them")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, root, err := loadChecked(fs.Args(), errw)
	if err != nil {
		return err
	}

	dir := filepath.Join(root, "snap")
	if !*checkOnly {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	var stale, written int
	var any bool
	for _, res := range runner.RunTests(p) {
		if len(res.Pages) == 0 {
			continue
		}
		any = true
		if !res.OK() {
			fmt.Fprintf(errw, "skipping %s: the test itself fails\n", res.Name)
			continue
		}
		body := snapshotBody(res)
		path := filepath.Join(dir, slug(res.Name)+".txt")
		rel := filepath.Join("snap", slug(res.Name)+".txt")

		if *checkOnly {
			old, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(out, "%s: no snapshot yet\n", rel)
				stale++
				continue
			}
			if string(old) != body {
				fmt.Fprintf(out, "%s: changed\n%s", rel, diffLines(string(old), body))
				stale++
			}
			continue
		}
		old, _ := os.ReadFile(path)
		if string(old) == body {
			continue
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
		fmt.Fprintln(out, rel)
		written++
	}

	if !any {
		fmt.Fprintln(out, "no tests visit a route, so there is nothing to snapshot")
		return nil
	}
	if *checkOnly {
		if stale > 0 {
			return fmt.Errorf("%d snapshot(s) differ; run `kiln snap` to accept", stale)
		}
		fmt.Fprintln(out, "snapshots match")
		return nil
	}
	if written == 0 {
		fmt.Fprintln(out, "snapshots unchanged")
	}
	return nil
}

// snapshotBody renders one test's visited pages, labelled by path.
func snapshotBody(res runner.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", res.Name)
	for _, p := range res.Pages {
		fmt.Fprintf(&b, "\n## %s\n\n%s", p.Path, p.Text)
	}
	return b.String()
}

// diffLines shows changed lines with a leading -/+ so a snapshot change reads
// at a glance.
func diffLines(oldText, newText string) string {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")
	var b strings.Builder
	for i := 0; i < len(oldLines) || i < len(newLines); i++ {
		var o, n string
		if i < len(oldLines) {
			o = oldLines[i]
		}
		if i < len(newLines) {
			n = newLines[i]
		}
		if o == n {
			continue
		}
		if o != "" {
			fmt.Fprintf(&b, "  - %s\n", o)
		}
		if n != "" {
			fmt.Fprintf(&b, "  + %s\n", n)
		}
	}
	return b.String()
}

func slug(s string) string {
	var b strings.Builder
	dash := true
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
