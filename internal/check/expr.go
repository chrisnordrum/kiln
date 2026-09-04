package check

import (
	"strings"

	"kiln/internal/ast"
	"kiln/internal/diag"
)

// scope is what names mean at one point in a program.
//
// Kiln has no closures and no imports, so a scope is small and flat: route
// path parameters, session state, whatever the surrounding data block and
// loops bound, and — inside a query's where clause — the fields of the table
// being queried.
type scope struct {
	c        *checker
	params   map[string]Type // route path parameters
	vars     map[string]Type // data keys, loop variables, action parameters
	table    string          // implicit table for bare field names
	event    Type            // what $value carries here
	hasEvent bool
}

func (s *scope) varNames() []string {
	out := make([]string, 0, len(s.vars))
	for n := range s.vars {
		out = append(out, n)
	}
	return out
}

// typeOf resolves an expression's type, reporting anything unresolvable.
func (c *checker) typeOf(e ast.Expr, sc *scope) Type {
	switch x := e.(type) {
	case nil:
		return tUnknown

	case *ast.Lit:
		switch x.Kind {
		case "string":
			if strings.HasPrefix(x.Text, "/") {
				return tPath
			}
			return tText
		case "number":
			if strings.Contains(x.Text, ".") {
				return tNum
			}
			return tInt
		case "bool":
			return tBool
		}
		return tNull

	case *ast.EventVar:
		if !sc.hasEvent {
			c.errf(x.Pos, "K021", "$%s is only available on an element that fires an event", x.Name).
				fix("use $value inside a check, input or select")
			return tUnknown
		}
		return sc.event

	case *ast.Name:
		return c.resolveName(x, sc)

	case *ast.Index:
		t, ok := c.table(x.Table, x.Pos)
		if !ok {
			return tUnknown
		}
		c.checkKey(x.Key, t, sc, x.Pos)
		return c.walk(recordOf(t.Name), x.Path, x.Pos)

	case *ast.Call:
		return c.checkCall(x, sc)

	case *ast.Unary:
		got := c.typeOf(x.X, sc)
		if x.Op == "not" {
			c.want(got, Bool, x.Pos, "not")
			return tBool
		}
		if got.Kind != Unknown && !got.numeric() {
			c.errf(x.Pos, "K030", "cannot negate %s", got).fix("negate a number")
		}
		return got

	case *ast.Binary:
		return c.checkBinary(x, sc)

	case *ast.Cond:
		c.want(c.typeOf(x.If, sc), Bool, x.Pos, "if")
		then := c.typeOf(x.Then, sc)
		els := c.typeOf(x.Else, sc)
		if !comparable(then, els) {
			c.errf(x.Pos, "K030", "if yields %s on one branch and %s on the other", then, els).
				fix("make both branches the same type")
		}
		if then.Kind == Unknown {
			return els
		}
		return then
	}
	return tUnknown
}

// want reports a type that is not the kind required in this position.
func (c *checker) want(got Type, kind Kind, pos ast.Pos, what string) {
	if got.Kind == Unknown || got.Kind == kind {
		return
	}
	c.errf(pos, "K030", "%s needs a %s, got %s", what, Type{Kind: kind}, got).
		fix("supply a " + Type{Kind: kind}.String())
}

// resolveName resolves a dotted name against the scope.
func (c *checker) resolveName(n *ast.Name, sc *scope) Type {
	head, rest := n.Parts[0], n.Parts[1:]

	switch head {
	case "params":
		if len(rest) == 0 {
			c.errf(n.Pos, "K021", "params needs a name").fix("write params.<name>")
			return tUnknown
		}
		t, ok := sc.params[rest[0]]
		if !ok {
			c.errf(n.Pos, "K021", "the route path declares no parameter %s", rest[0]).
				near(diag.Suggest(rest[0], keys(sc.params))).
				fix("add it to the path, as in /projects/:" + rest[0])
			return tUnknown
		}
		return c.walk(t, rest[1:], n.Pos)

	case "session":
		if len(rest) == 0 {
			c.errf(n.Pos, "K021", "session needs a name").fix("write session.<name>")
			return tUnknown
		}
		t, ok := c.sessionType(rest[0])
		if !ok {
			c.errf(n.Pos, "K021", "app.kiln declares no session %s", rest[0]).
				near(diag.Suggest(rest[0], c.sessionNames())).
				root("session:" + rest[0]).
				fix("add `session " + rest[0] + " ref <Table>` to app.kiln")
			return tUnknown
		}
		return c.walk(t, rest[1:], n.Pos)
	}

	if t, ok := sc.vars[head]; ok {
		return c.walk(t, rest, n.Pos)
	}

	// Inside a query's where clause, a bare name is a field of that table.
	if sc.table != "" {
		if t, ok := c.p.Table(sc.table); ok {
			if f, ok := t.Field(head); ok {
				return c.walk(fieldType(f), rest, n.Pos)
			}
		}
	}

	// A bare capitalized name is most likely a table used without a key.
	if _, ok := c.p.Table(head); ok {
		c.errf(n.Pos, "K030", "%s is a table, not a value", head).
			fix(sprintf("index it: %s[<id>]", head))
		return tUnknown
	}

	candidates := sc.varNames()
	if sc.table != "" {
		if t, ok := c.p.Table(sc.table); ok {
			candidates = append(candidates, t.FieldNames()...)
		}
	}
	c.errf(n.Pos, "K021", "nothing named %s is in scope here", head).
		near(diag.Suggest(head, candidates)).
		root("name:" + head).
		fix("use a data key, a loop variable, params.<x> or session.<x>")
	return tUnknown
}

// walk follows a field path from a starting type. A ref is followed into the
// table it points at, so Task[id].project.owner reads through two tables.
func (c *checker) walk(t Type, path []string, pos ast.Pos) Type {
	for _, part := range path {
		var table string
		switch t.Kind {
		case Record, Ref:
			table = t.Table
		case List:
			c.errf(pos, "K030", "%s is a list; read a field inside an `each`", t).
				fix("loop over it: each <list> as x")
			return tUnknown
		default:
			c.errf(pos, "K030", "%s has no fields to read", t).
				fix("read a field from a record or a reference")
			return tUnknown
		}
		tbl, ok := c.p.Table(table)
		if !ok {
			return tUnknown
		}
		f, ok := tbl.Field(part)
		if !ok {
			c.errf(pos, "K021", "%s has no field %s", table, part).
				near(diag.Suggest(part, tbl.FieldNames())).
				root(table + "." + part).
				fix("read a field the table declares")
			return tUnknown
		}
		t = fieldType(f)
	}
	return t
}

func (c *checker) checkCall(x *ast.Call, sc *scope) Type {
	sig, ok := stdlib[x.Fn]
	if !ok {
		c.errf(x.Pos, "K025", "no stdlib function named %s", x.Fn).
			near(diag.Suggest(x.Fn, StdlibNames())).
			root("fn:" + x.Fn).
			fix("Kiln has no user-defined functions; see `kiln docs --section stdlib`")
		for _, a := range x.Args {
			c.typeOf(a, sc)
		}
		return tUnknown
	}
	if len(x.Args) != len(sig.params) {
		c.errf(x.Pos, "K031", "%s takes %d argument(s), got %d", x.Fn, len(sig.params), len(x.Args)).
			fix("see `kiln docs --section stdlib`")
	}
	for i, a := range x.Args {
		got := c.typeOf(a, sc)
		if i < len(sig.params) && !acceptsKind(sig.params[i], got) {
			c.errf(a.Position(), "K030", "%s argument %d is %s, want %s",
				x.Fn, i+1, got, Type{Kind: sig.params[i]}).
				fix("supply a " + Type{Kind: sig.params[i]}.String())
		}
	}
	// coalesce and default return whatever they were given.
	if sig.ret.Kind == Unknown && len(x.Args) > 0 {
		return c.typeOf(x.Args[0], sc)
	}
	return sig.ret
}

func (c *checker) checkBinary(x *ast.Binary, sc *scope) Type {
	l := c.typeOf(x.L, sc)
	r := c.typeOf(x.R, sc)

	switch x.Op {
	case "and", "or":
		c.want(l, Bool, x.Pos, x.Op)
		c.want(r, Bool, x.Pos, x.Op)
		return tBool

	case "==", "!=":
		if !comparable(l, r) {
			c.errf(x.Pos, "K030", "cannot compare %s with %s", l, r).
				fix("compare values of the same type")
		}
		return tBool

	case "<", "<=", ">", ">=":
		if !orderable(l) || !orderable(r) {
			c.errf(x.Pos, "K030", "cannot order %s against %s", l, r).
				fix("order numbers or timestamps")
		}
		return tBool

	case "+":
		if l.Kind == Text || r.Kind == Text {
			return tText
		}
		fallthrough
	default:
		if l.Kind != Unknown && !l.numeric() || r.Kind != Unknown && !r.numeric() {
			c.errf(x.Pos, "K030", "cannot apply %s to %s and %s", x.Op, l, r).
				fix("arithmetic needs numbers")
			return tUnknown
		}
		if l.Kind == Num || r.Kind == Num {
			return tNum
		}
		return tInt
	}
}

func orderable(t Type) bool { return t.Kind == Unknown || t.numeric() || t.Kind == At }

func keys(m map[string]Type) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
