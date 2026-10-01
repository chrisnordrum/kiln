package inspect

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chrisnordrum/kiln/internal/ast"
)

// Occurrence is one place a symbol is defined or used.
type Occurrence struct {
	Pos     ast.Pos
	Defined bool
	What    string
}

// Where finds every definition and use of a symbol: a table, a field, an
// action, a route, a data key, a session field or an effect. One call replaces
// the fan of greps an agent would otherwise run, and it knows the difference
// between a definition and a use.
func Where(p *ast.Program, sym string) []Occurrence {
	var out []Occurrence
	def := func(pos ast.Pos, format string, a ...any) {
		out = append(out, Occurrence{Pos: pos, Defined: true, What: fmt.Sprintf(format, a...)})
	}
	use := func(pos ast.Pos, format string, a ...any) {
		out = append(out, Occurrence{Pos: pos, What: fmt.Sprintf(format, a...)})
	}

	// Accept "Task.title" as well as a bare name.
	table, field := "", sym
	if i := strings.IndexByte(sym, '.'); i > 0 {
		table, field = sym[:i], sym[i+1:]
	}

	matchesField := func(t *ast.Table, name string) bool {
		if name != field {
			return false
		}
		return table == "" || table == t.Name
	}

	if p.App != nil {
		for _, s := range p.App.Session {
			if s.Name == sym {
				def(s.Pos, "session field, ref %s", s.Table)
			}
			if s.Table == sym {
				use(s.Pos, "session %s", s.Name)
			}
		}
		for _, e := range p.App.Effects {
			if e.Name == sym {
				def(e.Pos, "effect via %s", e.Via)
			}
		}
	}

	for _, t := range p.Tables {
		if t.Name == sym {
			def(t.Pos, "table with %d fields", len(t.Fields))
		}
		for _, f := range t.Fields {
			if matchesField(t, f.Name) {
				def(f.Pos, "field of %s, type %s", t.Name, f.Type)
			}
			if f.Type == "ref" && f.Ref == sym {
				use(f.Pos, "%s.%s references it", t.Name, f.Name)
			}
		}
	}

	for _, a := range p.Actions {
		if a.Name == sym {
			def(a.Pos, "action taking %d parameter(s)", len(a.In))
		}
		for _, param := range a.In {
			if param.Ref == sym {
				use(param.Pos, "parameter %s of action %s", param.Name, a.Name)
			}
		}
		if a.Allow != nil {
			findInExpr(a.Allow, sym, table, field, func(pos ast.Pos, what string) {
				use(pos, "%s in the allow rule of %s", what, a.Name)
			})
		}
		for _, s := range a.Do {
			findInStmt(s, sym, table, field, func(pos ast.Pos, what string) {
				use(pos, "%s in action %s", what, a.Name)
			})
		}
		for _, s := range a.After {
			findInStmt(s, sym, table, field, func(pos ast.Pos, what string) {
				use(pos, "%s after action %s", what, a.Name)
			})
		}
	}

	for _, r := range p.Routes {
		if r.Name == sym || r.Path == sym {
			def(r.Pos, "route at %s", r.Path)
		}
		if r.Guard != nil {
			findInExpr(r.Guard.Cond, sym, table, field, func(pos ast.Pos, what string) {
				use(pos, "%s in the guard of %s", what, r.Name)
			})
			if r.Guard.Redirect == sym {
				use(r.Guard.Pos, "guard redirect of %s", r.Name)
			}
		}
		for _, q := range r.Data {
			if q.Key == sym {
				def(q.Pos, "data key of %s, %s %s", r.Name, q.Card, q.Table)
			}
			if q.Table == sym {
				use(q.Pos, "queried by %s.%s", r.Name, q.Key)
			}
			for _, o := range q.Order {
				if table == "" && o.Field == field || table == q.Table && o.Field == field {
					use(q.Pos, "order key of %s.%s", r.Name, q.Key)
				}
			}
			if q.Where != nil {
				findInExpr(q.Where, sym, table, field, func(pos ast.Pos, what string) {
					use(pos, "%s in the where of %s.%s", what, r.Name, q.Key)
				})
			}
		}
		findInNodes(r.View, sym, table, field, func(pos ast.Pos, what string) {
			use(pos, "%s in the view of %s", what, r.Name)
		})
	}

	for _, t := range p.Tests {
		for _, s := range t.Steps {
			if s.Target == sym {
				use(s.Pos, "%s in test %q", s.Kind, t.Name)
			}
			for _, a := range s.Attrs {
				if a.Name == field && (table == "" || table == s.Target) {
					use(a.Pos, "%s in test %q", s.Kind, t.Name)
				}
				findInExpr(a.Value, sym, table, field, func(pos ast.Pos, what string) {
					use(pos, "%s in test %q", what, t.Name)
				})
			}
			if s.Cond != nil {
				findInExpr(s.Cond, sym, table, field, func(pos ast.Pos, what string) {
					use(pos, "%s in test %q", what, t.Name)
				})
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Defined != out[j].Defined {
			return out[i].Defined
		}
		if out[i].Pos.File != out[j].Pos.File {
			return out[i].Pos.File < out[j].Pos.File
		}
		return out[i].Pos.Line < out[j].Pos.Line
	})
	return dedupe(out)
}

func dedupe(list []Occurrence) []Occurrence {
	seen := map[string]bool{}
	var out []Occurrence
	for _, o := range list {
		key := fmt.Sprintf("%s:%d:%s", o.Pos.File, o.Pos.Line, o.What)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, o)
	}
	return out
}

// findInExpr reports each mention of a symbol inside an expression.
func findInExpr(x ast.Expr, sym, table, field string, hit func(ast.Pos, string)) {
	switch n := x.(type) {
	case *ast.Name:
		for i, part := range n.Parts {
			if part == sym && i == 0 {
				hit(n.Pos, sym)
				return
			}
			if part == field && i > 0 {
				hit(n.Pos, n.String())
				return
			}
		}
	case *ast.Index:
		if n.Table == sym {
			hit(n.Pos, n.Table+"[...]")
		}
		for _, part := range n.Path {
			if part == field && (table == "" || table == n.Table) {
				hit(n.Pos, n.String())
			}
		}
		findInExpr(n.Key, sym, table, field, hit)
	case *ast.Call:
		if n.Fn == sym {
			hit(n.Pos, n.Fn+"()")
		}
		for _, a := range n.Args {
			findInExpr(a, sym, table, field, hit)
		}
	case *ast.Unary:
		findInExpr(n.X, sym, table, field, hit)
	case *ast.Binary:
		findInExpr(n.L, sym, table, field, hit)
		findInExpr(n.R, sym, table, field, hit)
	case *ast.Cond:
		findInExpr(n.If, sym, table, field, hit)
		findInExpr(n.Then, sym, table, field, hit)
		findInExpr(n.Else, sym, table, field, hit)
	}
}

func findInStmt(s ast.Stmt, sym, table, field string, hit func(ast.Pos, string)) {
	switch x := s.(type) {
	case *ast.Set:
		if x.Table == sym {
			hit(x.Pos, "set on "+x.Table)
		}
		if x.Field == field && (table == "" || table == x.Table) {
			hit(x.Pos, "set "+x.Table+"."+x.Field)
		}
		findInExpr(x.Key, sym, table, field, hit)
		findInExpr(x.Value, sym, table, field, hit)
	case *ast.SetSession:
		if x.Name == sym {
			hit(x.Pos, "set session."+x.Name)
		}
		findInExpr(x.Value, sym, table, field, hit)
	case *ast.New:
		if x.Table == sym {
			hit(x.Pos, "new "+x.Table)
		}
		for _, f := range x.Fields {
			if f.Name == field && (table == "" || table == x.Table) {
				hit(f.Pos, "new "+x.Table+" sets "+f.Name)
			}
			findInExpr(f.Value, sym, table, field, hit)
		}
	case *ast.Del:
		if x.Table == sym {
			hit(x.Pos, "del "+x.Table)
		}
		findInExpr(x.Key, sym, table, field, hit)
	case *ast.Send:
		if x.Effect == sym {
			hit(x.Pos, "send "+x.Effect)
		}
		for _, a := range x.Args {
			findInExpr(a.Value, sym, table, field, hit)
		}
	case *ast.Goto:
		if x.Path == sym {
			hit(x.Pos, "goto "+x.Path)
		}
	}
}

func findInNodes(list []*ast.Node, sym, table, field string, hit func(ast.Pos, string)) {
	for _, n := range list {
		if n.List != nil {
			findInExpr(n.List, sym, table, field, hit)
		}
		if n.Cond != nil {
			findInExpr(n.Cond, sym, table, field, hit)
		}
		for _, a := range n.Args {
			findInExpr(a, sym, table, field, hit)
		}
		for _, a := range n.Attrs {
			if a.Name == "do" && a.Value.String() == sym {
				hit(a.Pos, "bound to "+n.Kind)
				continue
			}
			findInExpr(a.Value, sym, table, field, hit)
		}
		findInNodes(n.Children, sym, table, field, hit)
		findInNodes(n.Else, sym, table, field, hit)
	}
}

// Render formats occurrences for reading.
func Render(sym string, list []Occurrence) string {
	if len(list) == 0 {
		return fmt.Sprintf("nothing named %s\n", sym)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", sym)
	for _, o := range list {
		label := "used   "
		if o.Defined {
			label = "defined"
		}
		fmt.Fprintf(&b, "  %s  %s:%d  %s\n", label, o.Pos.File, o.Pos.Line, o.What)
	}
	return b.String()
}
