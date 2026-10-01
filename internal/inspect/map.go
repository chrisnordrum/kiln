// Package inspect answers questions about a program's shape without an agent
// having to read the program.
//
// A grep returns lines; these return structure. `kiln map` is the orientation
// read that replaces opening six files to learn what an app contains, and
// `kiln where` replaces a fan of greps with one answer.
package inspect

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chrisnordrum/kiln/internal/ast"
)

// Map renders the whole app as a short outline: every table, action, route and
// test, and how they connect.
func Map(p *ast.Program) string {
	var b strings.Builder

	if p.App != nil {
		var parts []string
		for _, s := range p.App.Session {
			parts = append(parts, fmt.Sprintf("session.%s ref %s", s.Name, s.Table))
		}
		for _, e := range p.App.Effects {
			parts = append(parts, fmt.Sprintf("effect %s via %s", e.Name, e.Via))
		}
		fmt.Fprintf(&b, "app %s", p.App.Name)
		if len(parts) > 0 {
			fmt.Fprintf(&b, " — %s", strings.Join(parts, ", "))
		}
		b.WriteString("\n")
	}

	if len(p.Tables) > 0 {
		b.WriteString("\nTABLES\n")
		w := width(tableNames(p))
		for _, t := range p.Tables {
			fmt.Fprintf(&b, "  %-*s  %s", w, t.Name, strings.Join(fieldSummaries(t), " "))
			for _, ix := range t.Indexes {
				fmt.Fprintf(&b, "  index(%s)", strings.Join(ix, ", "))
			}
			for _, u := range t.Uniques {
				fmt.Fprintf(&b, "  unique(%s)", strings.Join(u, ", "))
			}
			b.WriteString("\n")
		}
	}

	if len(p.Actions) > 0 {
		b.WriteString("\nACTIONS\n")
		w := width(p.ActionNames())
		for _, a := range p.Actions {
			fmt.Fprintf(&b, "  %-*s  (%s)  %s  allow %s\n", w, a.Name,
				strings.Join(paramSummaries(a), ", "), effectSummary(a), allowSummary(a))
		}
	}

	if len(p.Routes) > 0 {
		b.WriteString("\nROUTES\n")
		w := width(routePaths(p))
		for _, r := range p.Routes {
			fmt.Fprintf(&b, "  %-*s  %s", w, r.Path, r.Name)
			if r.Guard != nil {
				fmt.Fprintf(&b, "  guard %s", r.Guard.Cond)
			}
			if len(r.Data) > 0 {
				fmt.Fprintf(&b, "  data(%s)", strings.Join(dataSummaries(r), ", "))
			}
			if acts := boundActions(r); len(acts) > 0 {
				fmt.Fprintf(&b, "  → %s", strings.Join(acts, ", "))
			}
			b.WriteString("\n")
		}
	}

	if len(p.Tests) > 0 {
		b.WriteString("\nTESTS\n")
		for _, t := range p.Tests {
			fmt.Fprintf(&b, "  %s\n", t.Name)
		}
	}
	return b.String()
}

// fieldSummaries names each column, marking references with an arrow so the
// shape of the schema is readable in one line per table.
func fieldSummaries(t *ast.Table) []string {
	out := make([]string, 0, len(t.Fields))
	for _, f := range t.Fields {
		s := f.Name
		if f.Type == "ref" {
			s += "→" + f.Ref
		}
		if f.Nullable {
			s += "?"
		}
		out = append(out, s)
	}
	return out
}

func paramSummaries(a *ast.Action) []string {
	out := make([]string, 0, len(a.In))
	for _, p := range a.In {
		s := p.Name + " " + p.Type
		if p.Type == "ref" {
			s = p.Name + " ref " + p.Ref
		}
		out = append(out, s)
	}
	return out
}

// effectSummary says what an action changes, which is usually the only thing
// worth knowing about it from the outside.
func effectSummary(a *ast.Action) string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range a.Do {
		switch x := s.(type) {
		case *ast.Set:
			add("set " + x.Table + "." + x.Field)
		case *ast.SetSession:
			add("set session." + x.Name)
		case *ast.New:
			add("new " + x.Table)
		case *ast.Del:
			add("del " + x.Table)
		case *ast.Send:
			add("send " + x.Effect)
		}
	}
	if len(out) == 0 {
		return "—"
	}
	return strings.Join(out, ", ")
}

func allowSummary(a *ast.Action) string {
	if a.Allow == nil {
		return "—"
	}
	return a.Allow.String()
}

func dataSummaries(r *ast.Route) []string {
	out := make([]string, 0, len(r.Data))
	for _, q := range r.Data {
		out = append(out, fmt.Sprintf("%s %s %s", q.Key, q.Card, q.Table))
	}
	return out
}

// boundActions lists the actions a route's view can invoke.
func boundActions(r *ast.Route) []string {
	var out []string
	seen := map[string]bool{}
	var walk func([]*ast.Node)
	walk = func(list []*ast.Node) {
		for _, n := range list {
			if a, ok := n.Attr("do"); ok {
				name := a.Value.String()
				if !seen[name] {
					seen[name] = true
					out = append(out, name)
				}
			}
			walk(n.Children)
			walk(n.Else)
		}
	}
	walk(r.View)
	sort.Strings(out)
	return out
}

func tableNames(p *ast.Program) []string { return p.TableNames() }

func routePaths(p *ast.Program) []string {
	out := make([]string, 0, len(p.Routes))
	for _, r := range p.Routes {
		out = append(out, r.Path)
	}
	return out
}

// width is the longest string's length, for column alignment in output that is
// read rather than edited.
func width(list []string) int {
	n := 0
	for _, s := range list {
		if len(s) > n {
			n = len(s)
		}
	}
	return n
}
