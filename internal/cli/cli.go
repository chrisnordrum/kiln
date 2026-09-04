// Package cli dispatches kiln subcommands.
package cli

import (
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"kiln/internal/diag"
	"kiln/internal/docs"
)

// Version is the build version, overridden at link time.
var Version = "dev"

// Command is one kiln subcommand. Phase records the build phase that lands it;
// a command with a nil Run is documented but not yet implemented, which keeps
// the CLI surface and the language reference in sync as the tool grows.
type Command struct {
	Name  string
	Blurb string
	Phase int
	Run   func(args []string, out, errw io.Writer) error
}

var commands = map[string]*Command{
	"docs":    {Name: "docs", Blurb: "print the language reference", Phase: 1, Run: runDocs},
	"fmt":     {Name: "fmt", Blurb: "rewrite to canonical form", Phase: 2},
	"check":   {Name: "check", Blurb: "verify the whole program", Phase: 3},
	"explain": {Name: "explain", Blurb: "what an error code means", Phase: 3, Run: runExplain},
	"map":     {Name: "map", Blurb: "whole-app outline", Phase: 6},
	"where":   {Name: "where", Blurb: "find a symbol's definition and uses", Phase: 6},
	"snap":    {Name: "snap", Blurb: "render a route to stable text", Phase: 4},
	"test":    {Name: "test", Blurb: "run tests/", Phase: 5},
	"dev":     {Name: "dev", Blurb: "dev server", Phase: 5},
	"new":     {Name: "new", Blurb: "scaffold a route, table or action", Phase: 6},
}

// Names returns every registered command name, sorted.
func Names() []string {
	out := make([]string, 0, len(commands))
	for n := range commands {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Run dispatches one invocation. args excludes the program name.
func Run(args []string, out, errw io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage(out)
		return 0
	}
	if args[0] == "version" || args[0] == "--version" {
		fmt.Fprintln(out, Version)
		return 0
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(errw, "kiln: no command %q\n\n", args[0])
		usage(errw)
		return 2
	}
	if cmd.Run == nil {
		fmt.Fprintf(errw, "kiln %s: not implemented yet (phase %d)\n", cmd.Name, cmd.Phase)
		return 2
	}
	if err := cmd.Run(args[1:], out, errw); err != nil {
		fmt.Fprintf(errw, "kiln %s: %v\n", cmd.Name, err)
		return 1
	}
	return 0
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "kiln — a full-stack web language for coding agents")
	fmt.Fprint(w, "\nusage: kiln <command> [args]\n\n")
	for _, n := range Names() {
		c := commands[n]
		mark := ""
		if c.Run == nil {
			mark = fmt.Sprintf("  (phase %d)", c.Phase)
		}
		fmt.Fprintf(w, "  %-8s %s%s\n", c.Name, c.Blurb, mark)
	}
	fmt.Fprintln(w, "\n  kiln docs        the complete language, small enough to read in full")
}

func runDocs(args []string, out, errw io.Writer) error {
	fs := flag.NewFlagSet("docs", flag.ContinueOnError)
	fs.SetOutput(errw)
	section := fs.String("section", "", "print one section: "+strings.Join(docs.Sections(), " "))
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *section == "" {
		fmt.Fprintln(out, docs.Full())
		return nil
	}
	body, err := docs.Section(*section)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, body)
	return nil
}

func runExplain(args []string, out, errw io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: kiln explain <code>\ncodes: %s", strings.Join(diag.Codes(), " "))
	}
	code := strings.ToUpper(args[0])
	text, ok := diag.Lookup(code)
	if !ok {
		near := diag.Suggest(code, diag.Codes())
		if len(near) > 0 {
			return fmt.Errorf("no code %s; did you mean %s", code, strings.Join(near, ", "))
		}
		return fmt.Errorf("no code %s\ncodes: %s", code, strings.Join(diag.Codes(), " "))
	}
	fmt.Fprintf(out, "%s\n\n%s\n", code, text)
	return nil
}
