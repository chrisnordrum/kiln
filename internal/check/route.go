package check

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/chrisnordrum/kiln/internal/ast"
	"github.com/chrisnordrum/kiln/internal/diag"
)

func sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }
func join(list []string, sep string) string  { return strings.Join(list, sep) }

// --- routes ---

func (c *checker) checkRoutes() {
	seen := map[string]bool{}
	for _, r := range c.p.Routes {
		if seen[r.Name] {
			c.errf(r.Pos, "K042", "route %s is declared twice", r.Name).fix("rename one of them")
		}
		seen[r.Name] = true
		c.checkRoute(r)
	}
}

func (c *checker) checkRoute(r *ast.Route) {
	clear(c.pending)
	sc := &scope{c: c, params: pathParams(r.Path), vars: map[string]Type{}}

	if r.Guard != nil {
		if got := c.typeOf(r.Guard.Cond, sc); got.Kind != Bool && got.Kind != Unknown && got.Kind != Ref {
			c.errf(r.Guard.Pos, "K030", "guard is %s, not a condition", got).
				fix("write a condition, as in `guard session.user else redirect /login`")
		}
		c.checkPath(r.Guard.Redirect, r.Guard.Pos)
	}

	for _, q := range r.Data {
		c.checkQuery(q, sc)
	}
	c.checkNodes(r.View, sc, r, 0)
	c.reportUnguarded(r)
}

// pathParams reads :name segments out of a route path.
func pathParams(path string) map[string]Type {
	out := map[string]Type{}
	for _, seg := range strings.Split(path, "/") {
		if strings.HasPrefix(seg, ":") && len(seg) > 1 {
			out[seg[1:]] = Type{Kind: Param}
		}
	}
	return out
}

func (c *checker) checkQuery(q *ast.Query, sc *scope) {
	t, ok := c.table(q.Table, q.Pos)
	if !ok {
		return
	}
	// Inside where, bare names are this table's fields; everything already in
	// scope stays visible so a query can reference an earlier data key.
	inner := &scope{c: c, params: sc.params, vars: sc.vars, table: t.Name}
	if q.Where != nil {
		if got := c.typeOf(q.Where, inner); got.Kind != Bool && got.Kind != Unknown {
			c.errf(q.Pos, "K030", "where is %s, not a condition", got).
				fix("write a condition, as in `where project == params.id`")
		}
	}
	for _, o := range q.Order {
		if _, ok := t.Field(o.Field); !ok {
			c.errf(q.Pos, "K021", "cannot order %s by %s, which it has no field for", t.Name, o.Field).
				near(diag.Suggest(o.Field, t.FieldNames())).
				root(t.Name + "." + o.Field).
				fix("order by a field the table declares")
		}
	}
	if q.Offset != nil {
		c.want(c.typeOf(q.Offset, sc), Int, q.Pos, "offset")
	}

	switch q.Card {
	case "one":
		sc.vars[q.Key] = recordOf(t.Name)
	case "many":
		sc.vars[q.Key] = listOf(t.Name)
	}
}

// checkLiteralPath verifies a quoted path, and first catches the mistake that
// quoting invites: writing a field reference inside the quotes.
//
// A quoted path is used exactly as written, so "/projects/p.id" renders the
// same dead link on every row — and it passes route resolution, because the
// :id segment is a wildcard that accepts any text. That combination is a
// silent failure, which is the one thing this language exists to prevent.
//
// The scope is what makes the check precise: "p" in "/projects/p.id" is a
// bound variable, while "robots" in "/robots.txt" is not, so a real filename
// is never mistaken for a botched interpolation.
func (c *checker) checkLiteralPath(path string, sc *scope, pos ast.Pos) {
	for _, seg := range strings.Split(path, "/") {
		dot := strings.IndexByte(seg, '.')
		if dot <= 0 {
			continue
		}
		head := seg[:dot]
		_, bound := sc.vars[head]
		if !bound && head != "params" && head != "session" {
			continue
		}
		c.errf(pos, "K026", "%q inside a quoted path is text, not a value", seg).
			fix(sprintf("build the path instead: to=%q + %s", path[:strings.Index(path, seg)], seg))
		return
	}
	c.checkPath(path, pos)
}

// checkPath verifies a static path resolves to a declared route.
func (c *checker) checkPath(path string, pos ast.Pos) {
	if path == "" {
		return
	}
	if _, ok := c.paths[path]; ok {
		return
	}
	for pattern := range c.paths {
		if pathMatches(pattern, path) {
			return
		}
	}
	c.errf(pos, "K023", "no route serves %s", path).
		near(diag.Suggest(path, c.routePaths())).
		root("path:" + path).
		fix("declare a route with that path, or link to one that exists")
}

func (c *checker) routePaths() []string {
	out := make([]string, 0, len(c.paths))
	for p := range c.paths {
		out = append(out, p)
	}
	return out
}

// pathMatches reports whether a concrete path is an instance of a pattern,
// so /projects/1 resolves against /projects/:id.
func pathMatches(pattern, path string) bool {
	pp := strings.Split(pattern, "/")
	cp := strings.Split(path, "/")
	if len(pp) != len(cp) {
		return false
	}
	for i := range pp {
		if strings.HasPrefix(pp[i], ":") {
			continue
		}
		if pp[i] != cp[i] {
			return false
		}
	}
	return true
}

// --- view ---

func (c *checker) checkNodes(nodes []*ast.Node, sc *scope, r *ast.Route, depth int) {
	if depth > maxViewDepth && len(nodes) > 0 {
		n := nodes[0]
		c.errf(n.Pos, "K012", "the view nests %d levels below its page, past the cap of %d",
			depth, maxViewDepth).
			root("depth:" + n.File).
			fix("lift part of this into a sibling block, or split the screen into another route")
		return
	}
	for _, n := range nodes {
		c.checkNode(n, sc, r, depth)
	}
}

func (c *checker) checkNode(n *ast.Node, sc *scope, r *ast.Route, depth int) {
	switch n.Kind {
	case "each":
		list := c.typeOf(n.List, sc)
		if list.Kind != List && list.Kind != Unknown {
			c.errf(n.Pos, "K030", "each needs a list, got %s", list).
				fix("loop over a `many` data key")
		}
		inner := childScope(sc)
		if list.Kind == List {
			inner.vars[n.Var] = recordOf(list.Table)
		} else {
			inner.vars[n.Var] = tUnknown
		}
		c.checkNodes(n.Children, inner, r, depth+1)
		return

	case "when":
		c.want(c.typeOf(n.Cond, sc), Bool, n.Pos, "when")
		c.checkNodes(n.Children, sc, r, depth+1)
		c.checkNodes(n.Else, sc, r, depth+1)
		return
	}

	spec, ok := elements[n.Kind]
	if !ok {
		c.errf(n.Pos, "K050", "no view element named %s", n.Kind).
			near(diag.Suggest(n.Kind, ElementNames())).
			root("element:" + n.Kind).
			fix("see `kiln docs --section view` for the whole vocabulary")
		return
	}
	if len(n.Args) < spec.minArgs || len(n.Args) > spec.maxArgs {
		c.errf(n.Pos, "K051", "%s takes %s, got %d",
			n.Kind, argCount(spec.minArgs, spec.maxArgs), len(n.Args)).
			fix("see `kiln docs --section view`")
	}
	if !spec.container && len(n.Children) > 0 {
		c.errf(n.Pos, "K051", "%s cannot contain other elements", n.Kind).
			fix("move the children out, or use a row, col or card")
	}

	c.checkNodeArgs(n, sc)
	c.checkNodeAttrs(n, spec, sc, r)
	c.checkNodes(n.Children, sc, r, depth+1)
}

// childScope copies a scope so a loop variable does not leak to siblings.
func childScope(sc *scope) *scope {
	vars := make(map[string]Type, len(sc.vars))
	for k, v := range sc.vars {
		vars[k] = v
	}
	return &scope{c: sc.c, params: sc.params, vars: vars, table: sc.table}
}

func argCount(min, max int) string {
	if min == max {
		return sprintf("%d argument(s)", min)
	}
	return sprintf("%d to %d arguments", min, max)
}

// checkNodeArgs type-checks positional arguments. input and area name a field
// and a type rather than taking expressions, so they are read literally.
func (c *checker) checkNodeArgs(n *ast.Node, sc *scope) {
	if n.Kind == "input" {
		if len(n.Args) >= 2 {
			if name, ok := n.Args[1].(*ast.Name); ok {
				if !contains(inputTypes, name.String()) {
					c.errf(n.Pos, "K021", "%q is not an input type", name.String()).
						near(diag.Suggest(name.String(), inputTypes)).
						fix("use one of: " + join(inputTypes, ", "))
				}
			}
		}
		if len(n.Args) == 3 {
			if flag, ok := n.Args[2].(*ast.Name); !ok || flag.String() != "required" {
				c.errf(n.Pos, "K051", "the third word of an input can only be `required`").
					fix("write: input <name> <type> required")
			}
		}
		return
	}
	if n.Kind == "head" && len(n.Args) >= 1 {
		if lit, ok := n.Args[0].(*ast.Lit); ok {
			if lvl, err := strconv.Atoi(lit.Text); err != nil || lvl < 1 || lvl > 4 {
				c.errf(n.Pos, "K030", "heading level must be 1 to 4, got %s", lit.Text).
					fix("use head 1, 2, 3 or 4")
			}
		}
		if len(n.Args) > 1 {
			c.typeOf(n.Args[1], sc)
		}
		return
	}
	if n.Kind == "select" || n.Kind == "area" {
		return // the argument names a form field, not a value
	}
	for _, a := range n.Args {
		c.typeOf(a, sc)
	}
}

func (c *checker) checkNodeAttrs(n *ast.Node, spec element, sc *scope, r *ast.Route) {
	var action *ast.Action
	if spec.actionArgs {
		action = c.resolveDo(n, sc, r)
	}

	inner := sc
	if ev, ok := eventType(n); ok {
		inner = childScope(sc)
		inner.event, inner.hasEvent = ev, true
	}

	supplied := map[string]bool{}
	for _, a := range n.Attrs {
		switch {
		case spec.actionArgs && a.Name == "do":
			// Already resolved to an action above; it is a name, not a value.
		case spec.attrAllowed(a.Name):
			c.checkCommonAttr(n, a, inner)
		case spec.actionArgs && action != nil:
			supplied[a.Name] = true
			c.checkActionArg(action, a, inner)
		case spec.actionArgs:
			c.typeOf(a.Value, inner)
		default:
			c.errf(a.Pos, "K052", "%s has no attribute %s", n.Kind, a.Name).
				near(diag.Suggest(a.Name, spec.knownAttrs())).
				fix("see `kiln docs --section view`")
		}
	}

	for _, req := range spec.required {
		if _, ok := n.Attr(req); !ok {
			c.errf(n.Pos, "K053", "%s needs %s=", n.Kind, req).
				fix(sprintf("add %s=<value>", req))
		}
	}

	if action != nil {
		// A form's inputs supply parameters by name, alongside its attributes.
		if n.Kind == "form" {
			for _, ch := range n.Children {
				collectFormFields(ch, supplied)
				c.checkSelects(ch, action)
			}
		}
		for _, p := range action.In {
			if !supplied[p.Name] {
				c.errf(n.Pos, "K031", "%s needs %s, which this %s does not supply",
					action.Name, p.Name, n.Kind).
					fix(sprintf("add %s=<value>, or an input named %s", p.Name, p.Name))
			}
		}
	}
}

// checkSelects validates every select that leans on its action for options.
//
// `select status` with no from= offers the values of the action's status
// parameter, so that parameter has to be an enum. Deriving the options from
// the action rather than repeating them in the view is what stops the two
// disagreeing: there is one list, and it is the one the call is checked
// against.
func (c *checker) checkSelects(n *ast.Node, action *ast.Action) {
	if n.Kind == "select" {
		if _, ok := n.Attr("from"); !ok {
			c.checkSelectParam(n, action)
		}
	}
	for _, ch := range n.Children {
		c.checkSelects(ch, action)
	}
	for _, ch := range n.Else {
		c.checkSelects(ch, action)
	}
}

func (c *checker) checkSelectParam(n *ast.Node, action *ast.Action) {
	name, ok := n.Args[0].(*ast.Name)
	if !ok {
		return
	}
	for _, p := range action.In {
		if p.Name != name.String() {
			continue
		}
		if p.Type != "enum" {
			c.errf(n.Pos, "K030", "select %s has no from= and %s's %s is %s, not an enum",
				name, action.Name, p.Name, p.Type).
				fix("add from=<list>, or declare the parameter as `" + p.Name + " enum a b c`")
		}
		return
	}
	c.errf(n.Pos, "K031", "%s has no parameter named %s", action.Name, name).
		near(diag.Suggest(name.String(), paramNames(action))).
		fix("add from=<list>, or name a parameter the action declares")
}

func paramNames(a *ast.Action) []string {
	out := make([]string, 0, len(a.In))
	for _, p := range a.In {
		out = append(out, p.Name)
	}
	return out
}

// collectFormFields records the parameter names a form's inputs supply.
func collectFormFields(n *ast.Node, into map[string]bool) {
	switch n.Kind {
	case "input", "select", "area":
		if len(n.Args) > 0 {
			if name, ok := n.Args[0].(*ast.Name); ok {
				into[name.String()] = true
			}
		}
	}
	for _, ch := range n.Children {
		collectFormFields(ch, into)
	}
	for _, ch := range n.Else {
		collectFormFields(ch, into)
	}
}

// eventType reports what $value carries on an element that fires one.
func eventType(n *ast.Node) (Type, bool) {
	switch n.Kind {
	case "check":
		return tBool, true
	case "select", "area":
		return tText, true
	case "input":
		if len(n.Args) >= 2 {
			if name, ok := n.Args[1].(*ast.Name); ok {
				switch name.String() {
				case "int":
					return tInt, true
				case "num":
					return tNum, true
				case "bool":
					return tBool, true
				case "at":
					return tAt, true
				}
			}
		}
		return tText, true
	}
	return tUnknown, false
}

// resolveDo resolves an element's do= to a declared action.
func (c *checker) resolveDo(n *ast.Node, sc *scope, r *ast.Route) *ast.Action {
	attr, ok := n.Attr("do")
	if !ok {
		return nil
	}
	name, ok := attr.Value.(*ast.Name)
	if !ok || len(name.Parts) != 1 {
		c.errf(attr.Pos, "K022", "do= needs an action name").
			fix("write do=<action>")
		return nil
	}
	action, ok := c.p.Action(name.Parts[0])
	if !ok {
		c.errf(attr.Pos, "K022", "no action named %s", name.Parts[0]).
			near(diag.Suggest(name.Parts[0], c.p.ActionNames())).
			root("action:" + name.Parts[0]).
			fix("declare it in actions/, or bind one that exists")
		return nil
	}
	c.checkReachable(action, r, attr.Pos)
	return action
}

func (c *checker) checkActionArg(a *ast.Action, attr *ast.Attr, sc *scope) {
	p, ok := a.Param(attr.Name)
	if !ok {
		c.errf(attr.Pos, "K032", "%s has no parameter %s", a.Name, attr.Name).
			near(diag.Suggest(attr.Name, a.ParamNames())).
			fix("pass a parameter the action declares, or add it to its in block")
		c.typeOf(attr.Value, sc)
		return
	}
	want := paramType(p)
	got := c.typeOf(attr.Value, sc)
	if got.Kind == Null && !p.Nullable {
		c.errf(attr.Pos, "K030", "%s.%s is %s and cannot be null", a.Name, p.Name, want).
			fix(sprintf("mark it nullable in the action: %s %s?", p.Name, p.Type))
		return
	}
	if !comparable(want, got) {
		c.errf(attr.Pos, "K030", "%s.%s is %s but is being passed %s", a.Name, p.Name, want, got).
			fix("pass a value of type " + want.String())
	}
}

func (c *checker) checkCommonAttr(n *ast.Node, a *ast.Attr, sc *scope) {
	switch a.Name {
	case "style":
		c.checkToken(a, styleTokens, "style")
	case "align":
		c.checkToken(a, alignTokens, "align")
	case "gap", "pad":
		c.checkSpacing(a)
	case "cols", "rows":
		c.want(c.typeOf(a.Value, sc), Int, a.Pos, a.Name)
	case "to":
		if lit, ok := a.Value.(*ast.Lit); ok && lit.Kind == "string" {
			c.checkLiteralPath(lit.Text, sc, a.Pos)
		} else {
			c.typeOf(a.Value, sc)
		}
	case "id":
		// An element id is a literal name, not a value to resolve.
	default:
		c.typeOf(a.Value, sc)
	}
}

func (c *checker) checkToken(a *ast.Attr, allowed []string, what string) {
	name, ok := a.Value.(*ast.Name)
	if !ok || !contains(allowed, name.String()) {
		c.errf(a.Pos, "K024", "%q is not a %s token", a.Value.String(), what).
			near(diag.Suggest(a.Value.String(), allowed)).
			fix("use one of: " + join(allowed, ", "))
	}
}

func (c *checker) checkSpacing(a *ast.Attr) {
	lit, ok := a.Value.(*ast.Lit)
	if !ok || lit.Kind != "number" {
		c.errf(a.Pos, "K030", "%s takes a step from 0 to %d", a.Name, spacingMax).
			fix(sprintf("write %s=2", a.Name))
		return
	}
	n, err := strconv.Atoi(lit.Text)
	if err != nil || n < 0 || n > spacingMax {
		c.errf(a.Pos, "K030", "%s must be 0 to %d, got %s", a.Name, spacingMax, lit.Text).
			fix(sprintf("use a step from 0 to %d", spacingMax))
	}
}

// --- reachability ---

// checkReachable records a control that can never succeed: the action's allow
// rule reads session state that the route's guard does not establish, so an
// anonymous visitor can reach a button whose action will always deny them.
//
// Recording rather than reporting, so one missing guard produces one
// diagnostic naming every action it blocks, instead of one per action.
func (c *checker) checkReachable(a *ast.Action, r *ast.Route, pos ast.Pos) {
	if a.Allow == nil || r == nil {
		return
	}
	for _, field := range sessionRoots(a.Allow) {
		if guardEstablishes(r.Guard, field) {
			continue
		}
		u, ok := c.pending[field]
		if !ok {
			u = &unguarded{pos: pos, seen: map[string]bool{}}
			c.pending[field] = u
		}
		if !u.seen[a.Name] {
			u.seen[a.Name] = true
			u.actions = append(u.actions, a.Name)
		}
	}
}

// reportUnguarded emits the collected reachability failures for one route.
func (c *checker) reportUnguarded(r *ast.Route) {
	fields := make([]string, 0, len(c.pending))
	for f := range c.pending {
		fields = append(fields, f)
	}
	sort.Strings(fields)

	for _, field := range fields {
		u := c.pending[field]
		sort.Strings(u.actions)
		c.errf(u.pos, "K040", "route %s does not guard session.%s, which %s require: %s",
			r.Name, field, plural(len(u.actions), "bound action", "bound actions"),
			join(u.actions, ", ")).
			root(sprintf("reach:%s:%s", r.Name, field)).
			fix(sprintf("add `guard session.%s else redirect /login` to the route. "+
				"Widening an allow rule would also silence this, but that removes the "+
				"permission rather than satisfying it", field))
	}
	clear(c.pending)
}

// plural picks a word form, so a message reads correctly for one or many.
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return sprintf("%d %s", n, many)
}

// sessionRoots lists the session fields an expression reads.
func sessionRoots(e ast.Expr) []string {
	var out []string
	seen := map[string]bool{}
	var walk func(ast.Expr)
	walk = func(e ast.Expr) {
		switch x := e.(type) {
		case *ast.Name:
			if x.Root() == "session" && len(x.Parts) > 1 && !seen[x.Parts[1]] {
				seen[x.Parts[1]] = true
				out = append(out, x.Parts[1])
			}
		case *ast.Index:
			walk(x.Key)
		case *ast.Call:
			for _, a := range x.Args {
				walk(a)
			}
		case *ast.Unary:
			walk(x.X)
		case *ast.Binary:
			walk(x.L)
			walk(x.R)
		case *ast.Cond:
			walk(x.If)
			walk(x.Then)
			walk(x.Else)
		}
	}
	walk(e)
	return out
}

// guardEstablishes reports whether a guard requires the given session field.
func guardEstablishes(g *ast.Guard, root string) bool {
	if g == nil {
		return false
	}
	return contains(sessionRoots(g.Cond), root)
}
