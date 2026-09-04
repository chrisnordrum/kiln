package cli

import (
	"flag"
	"fmt"
	"io"

	"kiln/internal/ast"
	"kiln/internal/eval"
	"kiln/internal/server"
)

func runDev(args []string, out, errw io.Writer) error {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	fs.SetOutput(errw)
	addr := fs.String("addr", "localhost:7777", "address to listen on")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, _, err := loadChecked(fs.Args(), errw)
	if err != nil {
		return err
	}

	srv := server.New(p)
	if n := seedFromTests(p, srv); n > 0 {
		fmt.Fprintf(out, "seeded %d row(s) from the first test that has fixtures\n", n)
	}
	for _, r := range p.Routes {
		fmt.Fprintf(out, "  http://%s%s\n", *addr, r.Path)
	}
	return srv.Listen(*addr)
}

// seedFromTests fills the dev store from the first test that seeds anything, so
// the server starts with something to look at instead of an empty database.
// Its `as` step also decides who the dev session is signed in as.
func seedFromTests(p *ast.Program, srv *server.Server) int {
	for _, t := range p.Tests {
		var rows int
		for _, s := range t.Steps {
			switch s.Kind {
			case "seed":
				fields := map[string]eval.Value{}
				for _, a := range s.Attrs {
					fields[a.Name] = literalValue(a.Value)
				}
				if err := srv.Seed(s.Target, fields); err == nil {
					rows++
				}
			case "as":
				for _, a := range s.Attrs {
					srv.SignIn(a.Name, literalValue(a.Value))
				}
			}
		}
		if rows > 0 {
			return rows
		}
	}
	return 0
}

// literalValue reads a seed's literal without a full evaluator, which is all a
// fixture ever contains.
func literalValue(x ast.Expr) eval.Value {
	switch v := x.(type) {
	case *ast.Lit:
		return eval.LitValue(v)
	case *ast.Name:
		return v.String()
	}
	return nil
}
