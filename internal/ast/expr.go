package ast

import (
	"fmt"
	"strings"
)

// Expr is any Kiln expression.
//
// The set is deliberately small and closed: no loops, no recursion, no
// user-defined functions. Every expression provably terminates, which is what
// lets the checker reason about a whole program instead of running it.
type Expr interface {
	Position() Pos
	String() string
}

// Lit is a literal: text, number, bool or null.
type Lit struct {
	Pos
	Kind string // string, number, bool, null
	Text string
}

func (l *Lit) String() string {
	if l.Kind == "string" {
		return fmt.Sprintf("%q", l.Text)
	}
	return l.Text
}

// Name is a dotted reference: params.id, session.user, t.done, project.title.
// The first part names a scope or a data key; the rest walk fields.
type Name struct {
	Pos
	Parts []string
}

func (n *Name) String() string { return strings.Join(n.Parts, ".") }

// Root is the leading part, which decides how the name resolves.
func (n *Name) Root() string { return n.Parts[0] }

// Index is a keyed record lookup with an optional field walk, as in
// Task[id].project.owner.
type Index struct {
	Pos
	Table string
	Key   Expr
	Path  []string
}

func (i *Index) String() string {
	s := fmt.Sprintf("%s[%s]", i.Table, i.Key)
	for _, p := range i.Path {
		s += "." + p
	}
	return s
}

// Call is a stdlib call. Kiln has no user-defined functions, so Fn always
// names a builtin.
type Call struct {
	Pos
	Fn   string
	Args []Expr
}

func (c *Call) String() string {
	parts := make([]string, len(c.Args))
	for i, a := range c.Args {
		parts[i] = a.String()
	}
	return fmt.Sprintf("%s(%s)", c.Fn, strings.Join(parts, ", "))
}

// Unary is negation or logical not.
type Unary struct {
	Pos
	Op string // - or not
	X  Expr
}

func (u *Unary) String() string {
	if u.Op == "not" {
		return "not " + u.X.String()
	}
	return u.Op + u.X.String()
}

// Binary is any infix operator.
type Binary struct {
	Pos
	Op   string
	L, R Expr
}

func (b *Binary) String() string { return fmt.Sprintf("%s %s %s", b.L, b.Op, b.R) }

// Cond is `if c then a else b`, the only branching an expression has.
type Cond struct {
	Pos
	If, Then, Else Expr
}

func (c *Cond) String() string {
	return fmt.Sprintf("if %s then %s else %s", c.If, c.Then, c.Else)
}

// EventVar is $value: what the interactive element that fired supplies.
type EventVar struct {
	Pos
	Name string
}

func (e *EventVar) String() string { return "$" + e.Name }
