package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"kiln/internal/format"
	"kiln/internal/project"
)

func runFmt(args []string, out, errw io.Writer) error {
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	fs.SetOutput(errw)
	check := fs.Bool("check", false, "list files that are not canonical and exit non-zero")
	if err := fs.Parse(args); err != nil {
		return err
	}

	root, err := rootFrom(fs.Args())
	if err != nil {
		return err
	}
	files, err := project.Load(root)
	if err != nil {
		return err
	}

	var changed []string
	for _, f := range files {
		formatted, d := format.File(f)
		if !d.Empty() {
			// A file that does not parse is left for `kiln check` to explain.
			if err := d.Render(errw, false); err != nil {
				return err
			}
			continue
		}
		if formatted == f.Src {
			continue
		}
		changed = append(changed, f.Path)
		if *check {
			continue
		}
		if err := os.WriteFile(filepath.Join(root, f.Path), []byte(formatted), 0o644); err != nil {
			return err
		}
	}

	if *check {
		for _, p := range changed {
			fmt.Fprintln(out, p)
		}
		if len(changed) > 0 {
			return fmt.Errorf("%d file(s) are not canonical; run `kiln fmt`", len(changed))
		}
		return nil
	}
	for _, p := range changed {
		fmt.Fprintln(out, p)
	}
	return nil
}

// rootFrom resolves the project directory: an explicit argument, or the
// nearest ancestor of the working directory that holds app.kiln.
func rootFrom(args []string) (string, error) {
	start := "."
	if len(args) > 0 {
		start = args[0]
	}
	if root, ok := project.Root(start); ok {
		return root, nil
	}
	// Allow pointing at a directory of .kiln files that has no app.kiln yet.
	if info, err := os.Stat(start); err == nil && info.IsDir() {
		files, err := project.Load(start)
		if err == nil && len(files) > 0 {
			return start, nil
		}
	}
	return "", fmt.Errorf("no Kiln project here: %s holds no app%s and no %s files", start, project.Ext, project.Ext)
}
