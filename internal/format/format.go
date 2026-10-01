// Package format renders a parsed program back to canonical source.
//
// Design rule 8: one canonical byte-form per program, so two sessions writing
// the same intent produce identical bytes and line numbers stay stable.
//
// Two choices follow from rule 9 rather than from taste. Columns are not
// aligned: aligning types would mean renaming one field rewrites every line in
// the block, which is exactly the diff churn that breaks an agent's next edit.
// Attributes are sorted alphabetically so that two orderings of the same line
// converge on one form.
package format

import (
	"sort"
	"strconv"
	"strings"

	"github.com/chrisnordrum/kiln/internal/ast"
	"github.com/chrisnordrum/kiln/internal/diag"
	"github.com/chrisnordrum/kiln/internal/lex"
	"github.com/chrisnordrum/kiln/internal/parse"
)

// File formats one source file. On a parse error it returns the input
// unchanged, so a broken file is never rewritten into something worse.
func File(f parse.File) (string, *diag.List) {
	var d diag.List
	p := parse.Program([]parse.File{f}, &d)
	if !d.Empty() {
		return f.Src, &d
	}
	return render(p), &d
}

// render emits every declaration in source order, one blank line between them
// and none inside — a rule with no ambiguity to resolve.
func render(p *ast.Program) string {
	type decl struct {
		line int
		text string
	}
	var decls []decl
	add := func(pos ast.Pos, text string) {
		decls = append(decls, decl{pos.Line, text})
	}
	if p.App != nil {
		add(p.App.Pos, app(p.App))
	}
	for _, t := range p.Tables {
		add(t.Pos, table(t))
	}
	for _, a := range p.Actions {
		add(a.Pos, action(a))
	}
	for _, r := range p.Routes {
		add(r.Pos, route(r))
	}
	for _, t := range p.Tests {
		add(t.Pos, test(t))
	}
	sort.SliceStable(decls, func(i, j int) bool { return decls[i].line < decls[j].line })

	var parts []string
	for _, dcl := range decls {
		parts = append(parts, dcl.text)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// buf accumulates indented lines.
type buf struct {
	sb    strings.Builder
	depth int
}

func (b *buf) line(parts ...string) {
	b.sb.WriteString(strings.Repeat(" ", b.depth*lex.IndentWidth))
	b.sb.WriteString(joinWords(parts))
	b.sb.WriteByte('\n')
}

func (b *buf) in(f func()) {
	b.depth++
	f()
	b.depth--
}

func (b *buf) String() string { return strings.TrimRight(b.sb.String(), "\n") }

// joinWords joins non-empty parts with single spaces, then repairs the two
// places where a token attaches without one.
func joinWords(parts []string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	s := strings.Join(kept, " ")
	s = strings.ReplaceAll(s, " ?", "?")
	return s
}

func app(a *ast.App) string {
	var b buf
	b.line("app", a.Name)
	b.in(func() {
		if a.Title != "" {
			b.line("title", strconv.Quote(a.Title))
		}
		for _, s := range a.Session {
			b.line("session", s.Name, "ref", s.Table)
		}
		for _, e := range a.Effects {
			b.line("effect", e.Name, "via", e.Via)
		}
	})
	return b.String()
}

func table(t *ast.Table) string {
	var b buf
	b.line("table", t.Name)
	b.in(func() {
		for _, f := range t.Fields {
			b.line(field(f)...)
		}
		for _, ix := range t.Indexes {
			b.line("index", strings.Join(ix, ", "))
		}
		for _, u := range t.Uniques {
			b.line("unique", strings.Join(u, ", "))
		}
	})
	return b.String()
}

// field emits modifiers in one fixed order, so the same column always spells
// the same way.
func field(f *ast.Field) []string {
	parts := []string{f.Name, f.Type}
	if f.Type == "ref" {
		parts = append(parts, f.Ref)
	}
	if f.Type == "enum" {
		parts = append(parts, f.Enum...)
	}
	if f.DefaultNow {
		parts = append(parts, "now")
	}
	if f.Nullable {
		parts = append(parts, "?")
	}
	if f.Max > 0 {
		parts = append(parts, "max", strconv.Itoa(f.Max))
	}
	if f.Unique {
		parts = append(parts, "unique")
	}
	if f.OnDelete != "" {
		parts = append(parts, "on", "delete", f.OnDelete)
	}
	if f.Default != nil {
		parts = append(parts, "=", Expr(f.Default))
	}
	return parts
}

func action(a *ast.Action) string {
	var b buf
	b.line("action", a.Name)
	b.in(func() {
		if len(a.In) > 0 {
			b.line("in")
			b.in(func() {
				for _, p := range a.In {
					parts := []string{p.Name, p.Type}
					if p.Type == "ref" {
						parts = append(parts, p.Ref)
					}
					if p.Type == "enum" {
						parts = append(parts, p.Enum...)
					}
					if p.Nullable {
						parts = append(parts, "?")
					}
					if p.Max > 0 {
						parts = append(parts, "max", strconv.Itoa(p.Max))
					}
					b.line(parts...)
				}
			})
		}
		if a.Allow != nil {
			b.line("allow", Expr(a.Allow))
		}
		if len(a.Do) > 0 {
			b.line("do")
			b.in(func() {
				for _, s := range a.Do {
					b.line(stmt(s)...)
				}
			})
		}
		if len(a.After) > 0 {
			b.line("after")
			b.in(func() {
				for _, s := range a.After {
					b.line(stmt(s)...)
				}
			})
		}
	})
	return b.String()
}

func stmt(s ast.Stmt) []string {
	switch x := s.(type) {
	case *ast.Set:
		return []string{"set", x.Table + "[" + Expr(x.Key) + "]." + x.Field, "=", Expr(x.Value)}
	case *ast.SetSession:
		return []string{"set", "session." + x.Name, "=", Expr(x.Value)}
	case *ast.New:
		return append([]string{"new", x.Table}, attrs(x.Fields)...)
	case *ast.Del:
		return []string{"del", x.Table + "[" + Expr(x.Key) + "]"}
	case *ast.Send:
		return append([]string{"send", x.Effect}, attrs(x.Args)...)
	case *ast.Refresh:
		return []string{"refresh"}
	case *ast.Goto:
		return []string{"goto", x.Path}
	case *ast.Toast:
		return []string{"toast", strconv.Quote(x.Msg)}
	}
	return nil
}

// attrs renders key=value pairs, sorted so one ordering is canonical.
func attrs(list []*ast.Attr) []string {
	sorted := make([]*ast.Attr, len(list))
	copy(sorted, list)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	out := make([]string, 0, len(sorted))
	for _, a := range sorted {
		out = append(out, a.Name+"="+Expr(a.Value))
	}
	return out
}

func route(r *ast.Route) string {
	var b buf
	b.line("route", r.Name)
	b.in(func() {
		b.line("path", r.Path)
		if r.Guard != nil {
			b.line("guard", Expr(r.Guard.Cond), "else", "redirect", r.Guard.Redirect)
		}
		if len(r.Data) > 0 {
			b.line("data")
			b.in(func() {
				for _, q := range r.Data {
					b.line(query(q)...)
				}
			})
		}
		b.line("view")
		b.in(func() { nodes(&b, r.View) })
	})
	return b.String()
}

func query(q *ast.Query) []string {
	parts := []string{q.Key, q.Card, q.Table}
	if q.Where != nil {
		parts = append(parts, "where", Expr(q.Where))
	}
	if len(q.Order) > 0 {
		var terms []string
		for _, o := range q.Order {
			dir := "asc"
			if o.Desc {
				dir = "desc"
			}
			terms = append(terms, o.Field+" "+dir)
		}
		parts = append(parts, "order", strings.Join(terms, ", "))
	}
	if q.Limit > 0 {
		parts = append(parts, "limit", strconv.Itoa(q.Limit))
	}
	if q.Offset != nil {
		parts = append(parts, "offset", Expr(q.Offset))
	}
	return parts
}

func nodes(b *buf, list []*ast.Node) {
	for _, n := range list {
		switch n.Kind {
		case "each":
			b.line("each", Expr(n.List), "as", n.Var)
		case "when":
			b.line("when", Expr(n.Cond))
		default:
			parts := []string{n.Kind}
			for _, a := range n.Args {
				parts = append(parts, Expr(a))
			}
			parts = append(parts, attrs(n.Attrs)...)
			b.line(parts...)
		}
		b.in(func() { nodes(b, n.Children) })
		if len(n.Else) > 0 {
			b.line("else")
			b.in(func() { nodes(b, n.Else) })
		}
	}
}

func test(t *ast.Test) string {
	var b buf
	b.line("test", strconv.Quote(t.Name))
	b.in(func() {
		for _, s := range t.Steps {
			b.line(step(s)...)
		}
	})
	return b.String()
}

func step(s *ast.Step) []string {
	switch s.Kind {
	case "seed":
		return append([]string{"seed", s.Target}, attrs(s.Attrs)...)
	case "as":
		return append([]string{"as"}, attrs(s.Attrs)...)
	case "call":
		return append([]string{"call", s.Target}, attrs(s.Attrs)...)
	case "visit":
		return []string{"visit", s.Target}
	case "expect":
		switch {
		case s.Denied && s.As != nil:
			return []string{"expect", "denied", "when", "as", s.As.Name + "=" + Expr(s.As.Value)}
		case s.Denied:
			return []string{"expect", "denied"}
		case s.Redirect != "":
			return []string{"expect", "redirect", s.Redirect}
		case s.Text != "" && s.Negate:
			return []string{"expect", "no", "text", strconv.Quote(s.Text)}
		case s.Text != "":
			return []string{"expect", "text", strconv.Quote(s.Text)}
		default:
			return []string{"expect", Expr(s.Cond)}
		}
	}
	return nil
}
