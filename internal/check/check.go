package check

import (
	"kiln/internal/ast"
	"kiln/internal/diag"
)

// checker carries the program and the diagnostics being accumulated.
type checker struct {
	p     *ast.Program
	d     *diag.List
	paths map[string]*ast.Route // route path to route

	// pending collects, for the route being checked, each session field its
	// bound actions require but its guard does not establish. Reported once per
	// field at the end of the route rather than once per action, because the
	// missing guard is the root cause and the actions are what trip over it.
	pending map[string]*unguarded
}

// unguarded is one session field a route fails to establish, and the actions
// that need it.
type unguarded struct {
	pos     ast.Pos
	actions []string
	seen    map[string]bool
}

// Program checks a whole parsed program.
func Program(p *ast.Program, d *diag.List) {
	c := &checker{p: p, d: d, paths: map[string]*ast.Route{}, pending: map[string]*unguarded{}}
	c.checkApp()
	c.checkSchema()
	for _, r := range p.Routes {
		if r.Path != "" {
			if prev, dup := c.paths[r.Path]; dup {
				c.errf(r.Pos, "K042", "route %s already claims %s", prev.Name, r.Path).
					fix("give one of them a different path")
			}
			c.paths[r.Path] = r
		}
	}
	c.checkActions()
	c.checkRoutes()
	c.checkTests()
}

// --- diagnostic helpers ---

// builder lets a diagnostic be finished fluently.
type builder struct {
	c *checker
	d diag.Diag
}

func (c *checker) errf(pos ast.Pos, code, format string, a ...any) *builder {
	return &builder{c: c, d: diag.Diag{
		Code: code, File: pos.File, Line: pos.Line, Col: pos.Col, Msg: sprintf(format, a...),
	}}
}

func (b *builder) fix(s string) *builder    { b.d.Fix = s; b.c.d.Add(b.d); return b }
func (b *builder) near(n []string) *builder { b.d.Near = n; return b }
func (b *builder) root(r string) *builder   { b.d.Root = r; return b }

// --- app ---

func (c *checker) checkApp() {
	if c.p.App == nil {
		c.d.Add(diag.Diag{Code: "K011", File: "app.kiln", Line: 1,
			Msg: "no app declaration",
			Fix: "add app.kiln with `app <name>` and a session line"})
		return
	}
	for _, s := range c.p.App.Session {
		if _, ok := c.p.Table(s.Table); !ok {
			c.errf(s.Pos, "K020", "session %s refers to unknown table %s", s.Name, s.Table).
				near(diag.Suggest(s.Table, c.p.TableNames())).
				root("table:" + s.Table).
				fix("declare the table in schema/, or point session at one that exists")
		}
	}
	for _, e := range c.p.App.Effects {
		if e.Via == "" {
			c.errf(e.Pos, "K011", "effect %s has no transport", e.Name).
				fix("write: effect " + e.Name + " via <transport>")
		}
	}
}

// sessionType returns the type of session.<name>.
func (c *checker) sessionType(name string) (Type, bool) {
	if c.p.App == nil {
		return tUnknown, false
	}
	for _, s := range c.p.App.Session {
		if s.Name == name {
			return refTo(s.Table), true
		}
	}
	return tUnknown, false
}

func (c *checker) sessionNames() []string {
	if c.p.App == nil {
		return nil
	}
	out := make([]string, 0, len(c.p.App.Session))
	for _, s := range c.p.App.Session {
		out = append(out, s.Name)
	}
	return out
}

func (c *checker) effectDeclared(name string) bool {
	if c.p.App == nil {
		return false
	}
	for _, e := range c.p.App.Effects {
		if e.Name == name {
			return true
		}
	}
	return false
}

func (c *checker) effectNames() []string {
	if c.p.App == nil {
		return nil
	}
	out := make([]string, 0, len(c.p.App.Effects))
	for _, e := range c.p.App.Effects {
		out = append(out, e.Name)
	}
	return out
}

// --- schema ---

func (c *checker) checkSchema() {
	seen := map[string]*ast.Table{}
	for _, t := range c.p.Tables {
		if prev, dup := seen[t.Name]; dup {
			c.errf(t.Pos, "K042", "table %s is already declared at %s:%d", t.Name, prev.File, prev.Line).
				fix("rename one of them")
		}
		seen[t.Name] = t
		c.checkTable(t)
	}
}

func (c *checker) checkTable(t *ast.Table) {
	fields := map[string]bool{}
	for _, f := range t.Fields {
		if fields[f.Name] {
			c.errf(f.Pos, "K042", "%s.%s is declared twice", t.Name, f.Name).
				fix("remove one of them")
		}
		fields[f.Name] = true

		if !contains(FieldTypeNames, f.Type) {
			c.errf(f.Pos, "K021", "%q is not a field type", f.Type).
				near(diag.Suggest(f.Type, FieldTypeNames)).
				fix("see `kiln docs --section schema`")
			continue
		}
		if f.Type == "ref" {
			if _, ok := c.p.Table(f.Ref); !ok {
				c.errf(f.Pos, "K020", "%s.%s refers to unknown table %s", t.Name, f.Name, f.Ref).
					near(diag.Suggest(f.Ref, c.p.TableNames())).
					root("table:" + f.Ref).
					fix("declare it in schema/, or point at a table that exists")
			}
		}
		if f.Type == "enum" && len(f.Enum) == 0 {
			c.errf(f.Pos, "K011", "enum %s has no values", f.Name).
				fix("write: " + f.Name + " enum a b c")
		}
		if f.Default != nil {
			want := fieldType(f)
			got := c.typeOf(f.Default, &scope{c: c})
			if !comparable(want, got) {
				c.errf(f.Pos, "K030", "%s.%s defaults to %s but is %s", t.Name, f.Name, got, want).
					fix("give a default matching the field's type")
			}
			if f.Type == "enum" && !contains(f.Enum, f.Default.String()) {
				c.errf(f.Pos, "K030", "%q is not one of %s's values", f.Default.String(), f.Name).
					near(diag.Suggest(f.Default.String(), f.Enum)).
					fix("use one of: " + join(f.Enum, ", "))
			}
		}
	}
	for _, group := range [][][]string{t.Indexes, t.Uniques} {
		for _, cols := range group {
			for _, col := range cols {
				if !fields[col] {
					c.errf(t.Pos, "K021", "%s has no field %s to index", t.Name, col).
						near(diag.Suggest(col, t.FieldNames())).
						fix("index a field the table declares")
				}
			}
		}
	}
}

// --- actions ---

func (c *checker) checkActions() {
	seen := map[string]bool{}
	for _, a := range c.p.Actions {
		if seen[a.Name] {
			c.errf(a.Pos, "K042", "action %s is declared twice", a.Name).fix("rename one of them")
		}
		seen[a.Name] = true
		c.checkAction(a)
	}
}

func (c *checker) checkAction(a *ast.Action) {
	sc := &scope{c: c, vars: map[string]Type{}}
	for _, p := range a.In {
		if !contains(ParamTypeNames, p.Type) {
			c.errf(p.Pos, "K021", "%q is not a parameter type", p.Type).
				near(diag.Suggest(p.Type, ParamTypeNames)).
				fix("see `kiln docs --section actions`")
			continue
		}
		if p.Type == "ref" {
			if _, ok := c.p.Table(p.Ref); !ok {
				c.errf(p.Pos, "K020", "parameter %s refers to unknown table %s", p.Name, p.Ref).
					near(diag.Suggest(p.Ref, c.p.TableNames())).
					root("table:" + p.Ref).
					fix("point at a table declared in schema/")
				continue
			}
		}
		sc.vars[p.Name] = paramType(p)
	}

	if a.Allow != nil {
		if got := c.typeOf(a.Allow, sc); got.Kind != Bool && got.Kind != Unknown {
			c.errf(a.Allow.Position(), "K030", "allow rule is %s, not a condition", got).
				fix("write a condition, as in `allow session.user == Task[id].project.owner`")
		}
	}
	for _, s := range a.Do {
		c.checkStmt(s, sc, false)
	}
	for _, s := range a.After {
		c.checkStmt(s, sc, true)
	}
}

func (c *checker) checkStmt(s ast.Stmt, sc *scope, after bool) {
	switch x := s.(type) {
	case *ast.Set:
		t, ok := c.table(x.Table, x.Pos)
		if !ok {
			return
		}
		c.checkKey(x.Key, t, sc, x.Pos)
		f, ok := t.Field(x.Field)
		if !ok {
			c.errf(x.Pos, "K021", "%s has no field %s", t.Name, x.Field).
				near(diag.Suggest(x.Field, t.FieldNames())).
				root(t.Name + "." + x.Field).
				fix("set a field the table declares")
			return
		}
		want := fieldType(f)
		got := c.typeOf(x.Value, sc)
		if !comparable(want, got) {
			c.errf(x.Pos, "K030", "%s.%s is %s but is being set to %s", t.Name, x.Field, want, got).
				fix("supply a value of type " + want.String())
		}

	case *ast.SetSession:
		want, ok := c.sessionType(x.Name)
		if !ok {
			c.errf(x.Pos, "K021", "app.kiln declares no session %s", x.Name).
				near(diag.Suggest(x.Name, c.sessionNames())).
				root("session:" + x.Name).
				fix("add `session " + x.Name + " ref <Table>` to app.kiln")
			return
		}
		got := c.typeOf(x.Value, sc)
		if got.Kind == Record {
			got = refTo(got.Table)
		}
		if !comparable(want, got) {
			c.errf(x.Pos, "K030", "session.%s is %s but is being set to %s", x.Name, want, got).
				fix("assign a " + want.String())
		}

	case *ast.New:
		t, ok := c.table(x.Table, x.Pos)
		if !ok {
			return
		}
		given := map[string]bool{}
		for _, f := range x.Fields {
			given[f.Name] = true
			field, ok := t.Field(f.Name)
			if !ok {
				c.errf(f.Pos, "K021", "%s has no field %s", t.Name, f.Name).
					near(diag.Suggest(f.Name, t.FieldNames())).
					root(t.Name + "." + f.Name).
					fix("set a field the table declares")
				continue
			}
			want := fieldType(field)
			got := c.typeOf(f.Value, sc)
			if !comparable(want, got) {
				c.errf(f.Pos, "K030", "%s.%s is %s but is being set to %s", t.Name, f.Name, want, got).
					fix("supply a value of type " + want.String())
			}
		}
		for _, f := range t.Fields {
			if needsValue(f) && !given[f.Name] {
				c.errf(x.Pos, "K053", "new %s does not set %s, which has no default", t.Name, f.Name).
					fix(sprintf("add %s=<value>, or give the field a default in schema/", f.Name))
			}
		}

	case *ast.Del:
		if t, ok := c.table(x.Table, x.Pos); ok {
			c.checkKey(x.Key, t, sc, x.Pos)
		}

	case *ast.Send:
		if !c.effectDeclared(x.Effect) {
			c.errf(x.Pos, "K022", "no effect named %s", x.Effect).
				near(diag.Suggest(x.Effect, c.effectNames())).
				root("effect:" + x.Effect).
				fix("declare it in app.kiln: effect " + x.Effect + " via <transport>")
		}
		for _, a := range x.Args {
			c.typeOf(a.Value, sc)
		}

	case *ast.Goto:
		c.checkPath(x.Path, x.Pos)

	case *ast.Refresh:
		if !after {
			c.errf(x.Pos, "K010", "refresh belongs in an after block").
				fix("move it under `after`")
		}
	}
}

// needsValue reports whether a new record must supply this field: it has no
// default, is not nullable, and is not the generated id.
func needsValue(f *ast.Field) bool {
	return f.Type != "id" && !f.Nullable && f.Default == nil && !f.DefaultNow
}

// table looks up a table, reporting it once if missing.
func (c *checker) table(name string, pos ast.Pos) (*ast.Table, bool) {
	t, ok := c.p.Table(name)
	if !ok {
		c.errf(pos, "K020", "no table named %s", name).
			near(diag.Suggest(name, c.p.TableNames())).
			root("table:" + name).
			fix("declare it in schema/, or use a table that exists")
	}
	return t, ok
}

// checkKey verifies a Table[key] lookup key is an id of that table.
func (c *checker) checkKey(key ast.Expr, t *ast.Table, sc *scope, pos ast.Pos) {
	got := c.typeOf(key, sc)
	if got.Kind == Unknown {
		return
	}
	if got.Kind == Ref && got.Table != t.Name {
		c.errf(pos, "K030", "%s[...] needs a %s id, got a %s id", t.Name, t.Name, got.Table).
			fix("index with a reference to " + t.Name)
		return
	}
	if !got.numeric() {
		c.errf(pos, "K030", "%s[...] needs an id, got %s", t.Name, got).
			fix("index with the record's id")
	}
}
