package runner

import (
	"fmt"
	"strings"

	"kiln/internal/ast"
	"kiln/internal/eval"
)

// Result is one test's outcome.
type Result struct {
	Name     string
	Failures []string
	Pages    []Visited // every route this test rendered, in order
}

// Visited pairs a visited path with what it rendered.
type Visited struct {
	Path string
	Text string
}

// OK reports whether the test passed.
func (r Result) OK() bool { return len(r.Failures) == 0 }

// RunTest executes one test against a fresh store.
func RunTest(p *ast.Program, t *ast.Test) Result {
	res := Result{Name: t.Name}
	run := New(p)

	// The most recent call, so `expect denied when as ...` can replay it.
	var lastAction *ast.Action
	var lastArgs map[string]eval.Value
	var lastText string

	fail := func(pos ast.Pos, format string, a ...any) {
		res.Failures = append(res.Failures,
			fmt.Sprintf("%s:%d: %s", pos.File, pos.Line, fmt.Sprintf(format, a...)))
	}

	for _, s := range t.Steps {
		switch s.Kind {
		case "seed":
			fields := map[string]eval.Value{}
			for _, a := range s.Attrs {
				fields[a.Name] = run.Eval(a.Value)
			}
			if err := run.Seed(s.Target, fields); err != nil {
				fail(s.Pos, "seed %s: %v", s.Target, err)
			}

		case "as":
			for _, a := range s.Attrs {
				run.SignIn(a.Name, run.Eval(a.Value))
			}

		case "call":
			action, ok := p.Action(s.Target)
			if !ok {
				fail(s.Pos, "no action %s", s.Target)
				continue
			}
			args := map[string]eval.Value{}
			for _, a := range s.Attrs {
				args[a.Name] = run.Eval(a.Value)
			}
			lastAction, lastArgs = action, args
			if err := run.Call(action, args); err != nil {
				if err == Denied {
					fail(s.Pos, "call %s was denied", s.Target)
				} else {
					fail(s.Pos, "call %s: %v", s.Target, err)
				}
			}

		case "visit":
			page, err := run.Visit(s.Target)
			if err != nil {
				fail(s.Pos, "%v", err)
				continue
			}
			if page.Redirect != "" {
				lastText = ""
				fail(s.Pos, "visit %s was redirected to %s by the route's guard", s.Target, page.Redirect)
				continue
			}
			lastText = page.Text
			res.Pages = append(res.Pages, Visited{Path: s.Target, Text: page.Text})

		case "expect":
			switch {
			case s.Denied:
				if lastAction == nil {
					fail(s.Pos, "expect denied has no call above it to replay")
					continue
				}
				// Replay the last call under a different identity, restoring
				// the original session afterwards.
				restore := map[string]eval.Value{}
				for k, v := range run.Session {
					restore[k] = v
				}
				if s.As != nil {
					run.SignIn(s.As.Name, run.Eval(s.As.Value))
				}
				err := run.Call(lastAction, lastArgs)
				run.Session = restore
				if err != Denied {
					who := "the current session"
					if s.As != nil {
						who = fmt.Sprintf("%s=%s", s.As.Name, s.As.Value)
					}
					fail(s.Pos, "%s was allowed to call %s, but should have been denied",
						who, lastAction.Name)
				}

			case s.Text != "":
				if !strings.Contains(lastText, s.Text) {
					fail(s.Pos, "the page does not contain %q", s.Text)
				}

			default:
				if !truthy(run.Eval(s.Cond)) {
					fail(s.Pos, "expected %s", s.Cond)
				}
			}
		}
	}
	return res
}

// RunTests executes every test in a program.
func RunTests(p *ast.Program) []Result {
	out := make([]Result, 0, len(p.Tests))
	for _, t := range p.Tests {
		out = append(out, RunTest(p, t))
	}
	return out
}
