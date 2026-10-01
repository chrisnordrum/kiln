package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/chrisnordrum/kiln/internal/check"
	"github.com/chrisnordrum/kiln/internal/diag"
	"github.com/chrisnordrum/kiln/internal/parse"
	"github.com/chrisnordrum/kiln/internal/project"
)

func runCheck(args []string, out, errw io.Writer) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(errw)
	asJSON := fs.Bool("json", false, "emit diagnostics as JSON")
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

	var d diag.List
	p := parse.Program(files, &d)
	// Only resolve names once the shape is sound: reference errors reported
	// against a half-parsed tree are noise, not signal.
	if d.Empty() {
		check.Program(p, &d)
	}
	if err := d.Render(out, *asJSON); err != nil {
		return err
	}
	if shown, _ := d.Resolved(); len(shown) > 0 {
		return errQuiet
	}
	return nil
}

// errQuiet exits non-zero without printing anything further: the diagnostics
// have already been rendered in the form the caller asked for.
var errQuiet = fmt.Errorf("")
