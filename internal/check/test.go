package check

import (
	"github.com/chrisnordrum/kiln/internal/ast"
	"github.com/chrisnordrum/kiln/internal/diag"
)

// checkTests resolves every test step against the program it exercises, so a
// test that names a field or an action that no longer exists fails at check
// time rather than at run time.
func (c *checker) checkTests() {
	for _, t := range c.p.Tests {
		c.checkTest(t)
	}
}

func (c *checker) checkTest(t *ast.Test) {
	// Seeded rows are addressable by later steps, and `as` sets the session.
	sc := &scope{c: c, params: map[string]Type{}, vars: map[string]Type{}}

	for _, s := range t.Steps {
		switch s.Kind {
		case "seed":
			c.checkSeed(s, sc)
		case "as":
			for _, a := range s.Attrs {
				if _, ok := c.sessionType(a.Name); !ok {
					c.errf(a.Pos, "K021", "app.kiln declares no session %s", a.Name).
						near(diag.Suggest(a.Name, c.sessionNames())).
						root("session:" + a.Name).
						fix("add `session " + a.Name + " ref <Table>` to app.kiln")
				}
			}
		case "call":
			c.checkCallStep(s, sc)
		case "visit":
			c.checkPath(s.Target, s.Pos)
		case "expect":
			c.checkExpect(s, sc)
		}
	}
}

func (c *checker) checkSeed(s *ast.Step, sc *scope) {
	t, ok := c.table(s.Target, s.Pos)
	if !ok {
		return
	}
	for _, a := range s.Attrs {
		f, ok := t.Field(a.Name)
		if !ok {
			c.errf(a.Pos, "K021", "%s has no field %s", t.Name, a.Name).
				near(diag.Suggest(a.Name, t.FieldNames())).
				root(t.Name + "." + a.Name).
				fix("seed a field the table declares")
			continue
		}
		want := fieldType(f)
		got := c.typeOf(a.Value, sc)
		if !comparable(want, got) {
			c.errf(a.Pos, "K030", "%s.%s is %s but is being seeded with %s", t.Name, a.Name, want, got).
				fix("seed a value of type " + want.String())
		}
	}
}

func (c *checker) checkCallStep(s *ast.Step, sc *scope) {
	a, ok := c.p.Action(s.Target)
	if !ok {
		c.errf(s.Pos, "K022", "no action named %s", s.Target).
			near(diag.Suggest(s.Target, c.p.ActionNames())).
			root("action:" + s.Target).
			fix("call an action declared in actions/")
		return
	}
	supplied := map[string]bool{}
	for _, attr := range s.Attrs {
		supplied[attr.Name] = true
		c.checkActionArg(a, attr, sc)
	}
	for _, p := range a.In {
		if !supplied[p.Name] {
			c.errf(s.Pos, "K031", "%s needs %s, which this call does not supply", a.Name, p.Name).
				fix(sprintf("add %s=<value>", p.Name))
		}
	}
}

func (c *checker) checkExpect(s *ast.Step, sc *scope) {
	if s.Denied {
		if s.As != nil {
			if _, ok := c.sessionType(s.As.Name); !ok {
				c.errf(s.As.Pos, "K021", "app.kiln declares no session %s", s.As.Name).
					near(diag.Suggest(s.As.Name, c.sessionNames())).
					root("session:" + s.As.Name).
					fix("use a session field app.kiln declares")
			}
		}
		return
	}
	if s.Redirect != "" {
		c.checkPath(s.Redirect, s.Pos)
		return
	}
	if s.Text != "" {
		return
	}
	if got := c.typeOf(s.Cond, sc); got.Kind != Bool && got.Kind != Unknown {
		c.errf(s.Pos, "K030", "expect is %s, not a condition", got).
			fix("write a condition, as in `expect Task[1].done == true`")
	}
}
