package render

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"kiln/internal/ast"
	"kiln/internal/eval"
)

// HTML renders a route's view as the markup the browser receives.
//
// Controls carry their action and arguments as data attributes; the client
// script reads them, posts to the server, and swaps back what it returns. No
// behavior is generated into the page, so there is nothing per-app to debug on
// the client.
func HTML(e *eval.Evaluator, r *ast.Route, env *eval.Env) string {
	w := &htmlWriter{e: e}
	w.nodes(r.View, env, 1)
	return w.sb.String()
}

type htmlWriter struct {
	e  *eval.Evaluator
	sb strings.Builder
	// form is the action of the form currently being written, so a select
	// inside it can offer that action's enum values. The options belong to
	// the action, not to the view, which is why the view does not repeat
	// them.
	form *ast.Action
}

func (w *htmlWriter) pad(depth int) { w.sb.WriteString(strings.Repeat("  ", depth)) }

// attr writes one HTML attribute with its value escaped. Go's %q verb applies
// Go string escaping, not HTML escaping, so it is wrong here: a value holding a
// backslash or a quote would reach the page malformed.
func attr(name, value string) string {
	return fmt.Sprintf(" %s=\"%s\"", name, html.EscapeString(value))
}

func (w *htmlWriter) nodes(list []*ast.Node, env *eval.Env, depth int) {
	for _, n := range list {
		w.node(n, env, depth)
	}
}

func (w *htmlWriter) node(n *ast.Node, env *eval.Env, depth int) {
	switch n.Kind {
	case "each":
		list, ok := w.e.Eval(n.List, env).(eval.List)
		if !ok {
			return
		}
		for _, row := range list.Rows {
			w.nodes(n.Children, env.Bind(n.Var, eval.Record{Table: list.Table, Row: row}), depth)
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

	text := func(i int) string {
		if i < len(n.Args) {
			return html.EscapeString(eval.Text(w.e.Eval(n.Args[i], env)))
		}
		return ""
	}
	word := func(i int) string {
		if i < len(n.Args) {
			return n.Args[i].String()
		}
		return ""
	}

	switch n.Kind {
	case "page":
		w.open("main", n, env, depth, "")
		w.nodes(n.Children, env, depth+1)
		w.close("main", depth)
	case "col", "row", "grid", "card":
		w.open("div", n, env, depth, "k-"+n.Kind)
		w.nodes(n.Children, env, depth+1)
		w.close("div", depth)
	case "sep":
		w.pad(depth)
		w.sb.WriteString("<hr>\n")
	case "head":
		level := word(0)
		if level == "" {
			level = "2"
		}
		w.pad(depth)
		fmt.Fprintf(&w.sb, "<h%s>%s</h%s>\n", level, text(1), level)
	case "text":
		w.pad(depth)
		fmt.Fprintf(&w.sb, "<p%s>%s</p>\n", w.classAttr(n), text(0))
	case "rich":
		w.pad(depth)
		fmt.Fprintf(&w.sb, "<div class=\"k-rich\">%s</div>\n", text(0))
	case "badge":
		w.pad(depth)
		fmt.Fprintf(&w.sb, "<span class=\"k-badge\">%s</span>\n", text(0))
	case "empty":
		w.pad(depth)
		fmt.Fprintf(&w.sb, "<p class=\"k-empty\">%s</p>\n", text(0))
	case "img":
		w.pad(depth)
		fmt.Fprintf(&w.sb, "<img%s%s>\n", attr("src", w.attrText(n, "src", env)), attr("alt", w.attrText(n, "alt", env)))
	case "link":
		w.pad(depth)
		fmt.Fprintf(&w.sb, "<a%s%s>%s</a>\n", attr("href", w.attrText(n, "to", env)), w.classAttr(n), text(0))
	case "button":
		w.pad(depth)
		fmt.Fprintf(&w.sb, "<button type=\"button\"%s%s>%s</button>\n",
			w.classAttr(n), w.actionAttrs(n, env), text(0))
	case "check":
		checked := ""
		if truthyValue(w.e.Eval(exprOf(n, "value"), env)) {
			checked = " checked"
		}
		w.pad(depth)
		fmt.Fprintf(&w.sb, "<input type=\"checkbox\"%s%s%s>\n", checked, w.classAttr(n), w.actionAttrs(n, env))
	case "form":
		w.pad(depth)
		fmt.Fprintf(&w.sb, "<form%s%s>\n", w.classAttr(n), w.actionAttrs(n, env))
		outer := w.form
		w.form = w.action(n)
		w.nodes(n.Children, env, depth+1)
		w.form = outer
		w.close("form", depth)
	case "input":
		w.pad(depth)
		fmt.Fprintf(&w.sb, "<label>%s<input%s%s%s%s></label>\n",
			w.label(n, env), attr("name", word(0)), attr("type", inputHTMLType(word(1))),
			requiredAttr(n), maxAttr(n, w, env))
	case "area":
		w.pad(depth)
		fmt.Fprintf(&w.sb, "<label>%s<textarea%s%s></textarea></label>\n",
			w.label(n, env), attr("name", word(0)), attr("rows", w.attrTextOr(n, "rows", env, "4")))
	case "select":
		w.pad(depth)
		fmt.Fprintf(&w.sb, "<label>%s<select%s>%s</select></label>\n",
			w.label(n, env), attr("name", word(0)), w.options(n, env))
	case "submit":
		w.pad(depth)
		fmt.Fprintf(&w.sb, "<button type=\"submit\">%s</button>\n", text(0))
	}
}

func (w *htmlWriter) open(tag string, n *ast.Node, env *eval.Env, depth int, class string) {
	w.pad(depth)
	fmt.Fprintf(&w.sb, "<%s%s>\n", tag, w.classAttrWith(n, class))
}

func (w *htmlWriter) close(tag string, depth int) {
	w.pad(depth)
	fmt.Fprintf(&w.sb, "</%s>\n", tag)
}

// classAttr turns the closed token vocabulary into class names. There is no
// author-supplied CSS, so every class here comes from a value the checker has
// already verified.
func (w *htmlWriter) classAttr(n *ast.Node) string { return w.classAttrWith(n, "") }

func (w *htmlWriter) classAttrWith(n *ast.Node, base string) string {
	var classes []string
	if base != "" {
		classes = append(classes, base)
	}
	for _, name := range []string{"style", "align"} {
		if a, ok := n.Attr(name); ok {
			classes = append(classes, "k-"+name+"-"+a.Value.String())
		}
	}
	for _, name := range []string{"gap", "pad"} {
		if a, ok := n.Attr(name); ok {
			classes = append(classes, "k-"+name+"-"+a.Value.String())
		}
	}
	if len(classes) == 0 {
		return ""
	}
	return attr("class", strings.Join(classes, " "))
}

// actionAttrs writes the action binding a control carries: its name, its
// resolved arguments, and which argument the event fills in.
func (w *htmlWriter) actionAttrs(n *ast.Node, env *eval.Env) string {
	do, ok := n.Attr("do")
	if !ok {
		return ""
	}
	args := map[string]any{}
	eventArg := ""
	for _, a := range n.Attrs {
		if a.Name == "do" || !isActionArg(n, a.Name) {
			continue
		}
		if _, isEvent := a.Value.(*ast.EventVar); isEvent {
			eventArg = a.Name
			continue
		}
		args[a.Name] = jsonValue(w.e.Eval(a.Value, env))
	}
	encoded, _ := json.Marshal(args)
	out := attr("data-k-do", do.Value.String()) + attr("data-k-args", string(encoded))
	if eventArg != "" {
		out += attr("data-k-event", eventArg)
	}
	if c, ok := n.Attr("confirm"); ok {
		out += attr("data-k-confirm", eval.Text(w.e.Eval(c.Value, env)))
	}
	return out
}

// jsonValue converts a runtime value into something json.Marshal can carry.
func jsonValue(v eval.Value) any {
	switch x := v.(type) {
	case nil, bool, int64, float64, string:
		return x
	case eval.Record:
		return x.Row["id"]
	}
	return eval.Text(v)
}

func (w *htmlWriter) attrText(n *ast.Node, name string, env *eval.Env) string {
	return w.attrTextOr(n, name, env, "")
}

func (w *htmlWriter) attrTextOr(n *ast.Node, name string, env *eval.Env, fallback string) string {
	a, ok := n.Attr(name)
	if !ok {
		return fallback
	}
	return eval.Text(w.e.Eval(a.Value, env))
}

func (w *htmlWriter) label(n *ast.Node, env *eval.Env) string {
	if a, ok := n.Attr("label"); ok {
		return "<span>" + html.EscapeString(eval.Text(w.e.Eval(a.Value, env))) + "</span>"
	}
	return ""
}

// options renders a select's choices from the enum the field declares.
func (w *htmlWriter) options(n *ast.Node, env *eval.Env) string {
	var b strings.Builder
	a, ok := n.Attr("from")
	if !ok {
		// No from=: the options are the enum values of the parameter this
		// field supplies, which the checker has already proved is an enum.
		for _, v := range w.selectEnum(n) {
			esc := html.EscapeString(v)
			fmt.Fprintf(&b, "<option%s>%s</option>", attr("value", esc), esc)
		}
		return b.String()
	}
	if list, ok := w.e.Eval(a.Value, env).(eval.List); ok {
		for _, row := range list.Rows {
			id := html.EscapeString(eval.Text(row["id"]))
			fmt.Fprintf(&b, "<option%s>%s</option>", attr("value", id), id)
		}
	}
	return b.String()
}

// selectEnum finds the values a bare select offers.
func (w *htmlWriter) selectEnum(n *ast.Node) []string {
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
func (w *htmlWriter) action(n *ast.Node) *ast.Action {
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

func requiredAttr(n *ast.Node) string {
	for _, a := range n.Args {
		if a.String() == "required" {
			return " required"
		}
	}
	return ""
}

func maxAttr(n *ast.Node, w *htmlWriter, env *eval.Env) string {
	a, ok := n.Attr("max")
	if !ok {
		return ""
	}
	return attr("maxlength", eval.Text(w.e.Eval(a.Value, env)))
}

// inputHTMLType maps a Kiln field type to the browser's input type.
func inputHTMLType(kind string) string {
	switch kind {
	case "int", "num":
		return "number"
	case "bool":
		return "checkbox"
	case "at":
		return "datetime-local"
	}
	return "text"
}

func exprOf(n *ast.Node, name string) ast.Expr {
	if a, ok := n.Attr(name); ok {
		return a.Value
	}
	return nil
}

// Title renders a route's page title for the document head.
func Title(e *eval.Evaluator, r *ast.Route, env *eval.Env) string {
	for _, n := range r.View {
		if n.Kind == "page" {
			if a, ok := n.Attr("title"); ok {
				return eval.Text(e.Eval(a.Value, env))
			}
		}
	}
	return r.Name
}
