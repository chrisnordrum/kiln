// Package ast is Kiln's typed syntax tree.
//
// The parser produces this; the checker resolves names against it; the
// renderer and runtime walk it. Every node carries a Pos so a diagnostic can
// point at the line that caused it.
package ast

// Pos locates a node in source.
type Pos struct {
	File string
	Line int
	Col  int
}

// Position lets an embedded Pos satisfy the Node interfaces below.
func (p Pos) Position() Pos { return p }

// Program is a whole Kiln app: every file, resolved into one tree.
type Program struct {
	App     *App
	Tables  []*Table
	Actions []*Action
	Routes  []*Route
	Tests   []*Test
}

// Table returns a table by name.
func (p *Program) Table(name string) (*Table, bool) {
	for _, t := range p.Tables {
		if t.Name == name {
			return t, true
		}
	}
	return nil, false
}

// Action returns an action by name.
func (p *Program) Action(name string) (*Action, bool) {
	for _, a := range p.Actions {
		if a.Name == name {
			return a, true
		}
	}
	return nil, false
}

// TableNames lists every declared table, for suggestions.
func (p *Program) TableNames() []string {
	out := make([]string, 0, len(p.Tables))
	for _, t := range p.Tables {
		out = append(out, t.Name)
	}
	return out
}

// ActionNames lists every declared action, for suggestions.
func (p *Program) ActionNames() []string {
	out := make([]string, 0, len(p.Actions))
	for _, a := range p.Actions {
		out = append(out, a.Name)
	}
	return out
}

// App is app.kiln: the program's name, its session shape, and the only I/O it
// is permitted to perform.
type App struct {
	Pos
	Name    string
	Title   string
	Session []*SessionDecl
	Effects []*Effect
}

// SessionDecl binds session.<Name> to a table, so the checker knows the type of
// session state a guard or an allow rule is reading.
type SessionDecl struct {
	Pos
	Name  string
	Table string
}

// Effect is a declared outbound capability. Actions reach it with `send`, and
// there is no other way out of the language.
type Effect struct {
	Pos
	Name string
	Via  string
}

// Table is one schema declaration.
type Table struct {
	Pos
	Name    string
	Fields  []*Field
	Indexes [][]string
	Uniques [][]string
}

// Field returns a field by name.
func (t *Table) Field(name string) (*Field, bool) {
	for _, f := range t.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return nil, false
}

// FieldNames lists every field, for suggestions.
func (t *Table) FieldNames() []string {
	out := make([]string, 0, len(t.Fields))
	for _, f := range t.Fields {
		out = append(out, f.Name)
	}
	return out
}

// Field is one column.
type Field struct {
	Pos
	Name       string
	Type       string   // id, text, int, num, bool, at, enum, ref
	Ref        string   // target table, when Type is ref
	OnDelete   string   // cascade, restrict, null
	Max        int      // text length cap, 0 for none
	Enum       []string // permitted values, when Type is enum
	Nullable   bool
	Unique     bool
	Default    Expr // nil when there is none
	DefaultNow bool // `at now`
}

// Action is a named server mutation.
type Action struct {
	Pos
	Name  string
	In    []*Param
	Allow Expr
	Do    []Stmt
	After []Stmt
}

// Param returns a declared parameter by name.
func (a *Action) Param(name string) (*Param, bool) {
	for _, p := range a.In {
		if p.Name == name {
			return p, true
		}
	}
	return nil, false
}

// ParamNames lists every declared parameter, for suggestions.
func (a *Action) ParamNames() []string {
	out := make([]string, 0, len(a.In))
	for _, p := range a.In {
		out = append(out, p.Name)
	}
	return out
}

// Param is one typed action input. Every one is required.
type Param struct {
	Pos
	Name string
	Type string // text, int, num, bool, at, ref
	Ref  string // target table, when Type is ref
	Max  int
}

// Route is one screen: its path, its guard, its data and its view, in one file.
type Route struct {
	Pos
	Name  string
	Path  string
	Guard *Guard
	Data  []*Query
	View  []*Node
}

// Query returns a data key by name.
func (r *Route) Query(key string) (*Query, bool) {
	for _, q := range r.Data {
		if q.Key == key {
			return q, true
		}
	}
	return nil, false
}

// DataKeys lists every data key, for suggestions.
func (r *Route) DataKeys() []string {
	out := make([]string, 0, len(r.Data))
	for _, q := range r.Data {
		out = append(out, q.Key)
	}
	return out
}

// Guard gates a route, and establishes the session facts an action's allow rule
// is later checked against.
type Guard struct {
	Pos
	Cond     Expr
	Redirect string
}

// Query is one entry in a route's data block.
type Query struct {
	Pos
	Key    string
	Card   string // one or many
	Table  string
	Where  Expr
	Order  []OrderBy
	Limit  int
	Offset Expr
}

// OrderBy is one sort term.
type OrderBy struct {
	Field string
	Desc  bool
}

// Node is one element in a view tree.
type Node struct {
	Pos
	Kind     string // page, row, text, each, when, button, ...
	Args     []Expr // positional arguments
	Attrs    []*Attr
	Children []*Node

	List Expr // `each <List> as <Var>`
	Var  string
	Cond Expr // `when <Cond>`
	Else []*Node
}

// Attr returns an attribute by name.
func (n *Node) Attr(name string) (*Attr, bool) {
	for _, a := range n.Attrs {
		if a.Name == name {
			return a, true
		}
	}
	return nil, false
}

// Attr is one key=value on a view node or statement.
type Attr struct {
	Pos
	Name  string
	Value Expr
}

// Test is one spec-level test.
type Test struct {
	Pos
	Name  string
	Steps []*Step
}

// Step is one line of a test: seed, as, call, visit or expect.
type Step struct {
	Pos
	Kind   string // seed, as, call, visit, expect
	Target string // table name, action name, path, or session field
	Attrs  []*Attr
	Cond   Expr   // for expect
	Text   string // for `expect text`
	Denied bool   // for `expect denied`
	As     *Attr  // for `expect denied when as user=2`
}
