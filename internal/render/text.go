// Package render turns a route's view into output.
//
// The text rendering is the one an agent reads. It shows the page as it
// actually resolved — loops expanded, conditions taken, values substituted —
// so a UI change is verified by diffing text rather than by looking at pixels.
package render

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"kiln/internal/ast"
	"kiln/internal/eval"
)

// Text renders a route's view against the data already bound in env.
func Text(e *eval.Evaluator, r *ast.Route, env *eval.Env) string {
	w := &writer{e: e}
	w.nodes(r.View, env, 0)
	return strings.TrimRight(w.sb.String(), "\n") + "\n"
}

type writer struct {
	e  *eval.Evaluator
	sb strings.Builder
	// form is the action of the form being written, so a bare select can
	// report the options that action accepts.
	form *ast.Action
}

func (w *writer) line(depth int, parts ...string) {
	w.sb.WriteString(strings.Repeat("  ", depth))
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	w.sb.WriteString(strings.Join(kept, " "))
	w.sb.WriteByte('\n')
}

func (w *writer) nodes(list []*ast.Node, env *eval.Env, depth int) {
	for _, n := range list {
		w.node(n, env, depth)
	}
}

func (w *writer) node(n *ast.Node, env *eval.Env, depth int) {
	switch n.Kind {
	case "each":
		// A loop is expanded, so the snapshot shows the rows that exist rather
		// than the fact that a loop was written.
		list, ok := w.e.Eval(n.List, env).(eval.List)
		if !ok || len(list.Rows) == 0 {
			return
		}
		for _, row := range list.Rows {
			inner := env.Bind(n.Var, eval.Record{Table: list.Table, Row: row})
			w.nodes(n.Children, inner, depth)
		}
		return

	case "when":
		if truthyValue(w.e.Eval(n.Cond, env)) {
			w.nodes(n.Children, env, depth)
		} else {
			w.nodes(n.Else, env, depth)
		}
		return
	}

	parts := []string{n.Kind}
	parts = append(parts, w.args(n, env)...)
	parts = append(parts, w.attrs(n, env)...)
	// A bare select takes its options from the action, so the source line does
	// not show them. The snapshot has to, or the one channel that pins what a
	// page actually offers would be silent about the values.
	if n.Kind == "select" {
		if _, ok := n.Attr("from"); !ok {
			if vals := w.selectEnum(n); len(vals) > 0 {
				parts = append(parts, "options="+strconv.Quote(strings.Join(vals, ", ")))
			}
		}
	}
	w.line(depth, parts...)
	if n.Kind == "form" {
		outer := w.form
		w.form = w.action(n)
		w.nodes(n.Children, env, depth+1)
		w.form = outer
		return
	}
	w.nodes(n.Children, env, depth+1)
}

// selectEnum finds the values a bare select offers.
func (w *writer) selectEnum(n *ast.Node) []string {
	if w.form == nil || len(n.Args) == 0 {
		return nil
	}
	name, ok := n.Args[0].(*ast.Name)
	if !ok {
		return nil
	}
	for _, p := range w.form.In {
		if p.Name == name.String() {
			return p.Enum
		}
	}
	return nil
}

// action resolves a node's do= to the action it names.
func (w *writer) action(n *ast.Node) *ast.Action {
	a, ok := n.Attr("do")
	if !ok {
		return nil
	}
	name, ok := a.Value.(*ast.Name)
	if !ok {
		return nil
	}
	act, _ := w.e.P.Action(name.String())
	return act
}

// args renders positional arguments. input, select and area name a form field
// and a type rather than carrying values, so they print as written.
func (w *writer) args(n *ast.Node, env *eval.Env) []string {
	switch n.Kind {
	case "input", "select", "area":
		out := make([]string, 0, len(n.Args))
		for _, a := range n.Args {
			out = append(out, a.String())
		}
		return out
	case "head":
		out := []string{}
		for i, a := range n.Args {
			if i == 0 {
				out = append(out, a.String()) // the level
				continue
			}
			out = append(out, quote(eval.Text(w.e.Eval(a, env))))
		}
		return out
	}
	out := make([]string, 0, len(n.Args))
	for _, a := range n.Args {
		out = append(out, quote(eval.Text(w.e.Eval(a, env))))
	}
	return out
}

// attrs renders attributes with their values resolved, and folds an action
// binding into one do=name(args) term so a control's whole behavior is on the
// line that shows it.
func (w *writer) attrs(n *ast.Node, env *eval.Env) []string {
	var action string
	var actionArgs []string
	var plain []string

	for _, a := range n.Attrs {
		if a.Name == "do" {
			action = a.Value.String()
			continue
		}
		if isActionArg(n, a.Name) {
			actionArgs = append(actionArgs, a.Name+"="+w.value(a.Value, env))
			continue
		}
		plain = append(plain, a.Name+"="+w.value(a.Value, env))
	}
	sort.Strings(plain)
	sort.Strings(actionArgs)

	var out []string
	if action != "" {
		out = append(out, fmt.Sprintf("do=%s(%s)", action, strings.Join(actionArgs, ", ")))
	}
	return append(out, plain...)
}

// value renders one attribute value. An event binding stays symbolic: it names
// what will be sent, which is not known until the control is used.
func (w *writer) value(x ast.Expr, env *eval.Env) string {
	if ev, ok := x.(*ast.EventVar); ok {
		return "$" + ev.Name
	}
	v := w.e.Eval(x, env)
	if s, ok := v.(string); ok {
		// A bare word that evaluates to itself is a token or an enum value,
		// not text, so it prints the way it was written.
		if n, ok := x.(*ast.Name); ok && len(n.Parts) == 1 && n.Parts[0] == s {
			return s
		}
		return quote(s)
	}
	return eval.Text(v)
}

// isActionArg reports whether an attribute is an argument to the element's
// action rather than a presentation attribute.
func isActionArg(n *ast.Node, name string) bool {
	if _, ok := n.Attr("do"); !ok {
		return false
	}
	switch name {
	case "gap", "pad", "align", "style", "confirm", "value", "to":
		return false
	}
	return true
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }

func truthyValue(v eval.Value) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case int64:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x != ""
	case eval.List:
		return len(x.Rows) > 0
	}
	return true
}
