package eval

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chrisnordrum/kiln/internal/ast"
)

// Env is what names mean at one point during evaluation.
type Env struct {
	Params  map[string]Value
	Session map[string]Value
	Vars    map[string]Value
	Event   Value
	// Row is the record whose fields bare names refer to, inside a where
	// clause.
	Row *Record
}

// child copies an environment so a loop variable does not leak to siblings.
func (e *Env) child() *Env {
	vars := make(map[string]Value, len(e.Vars)+1)
	for k, v := range e.Vars {
		vars[k] = v
	}
	return &Env{Params: e.Params, Session: e.Session, Vars: vars, Event: e.Event, Row: e.Row}
}

// Bind returns a copy of the environment with one more variable.
func (e *Env) Bind(name string, v Value) *Env {
	out := e.child()
	out.Vars[name] = v
	return out
}

// Evaluator runs expressions and queries against a store.
type Evaluator struct {
	P *ast.Program
	S *Store
}

// New creates an evaluator over a fresh store.
func New(p *ast.Program) *Evaluator { return &Evaluator{P: p, S: NewStore(p)} }

// Eval computes an expression's value. A checked program cannot reach the
// error paths here, so they return nil rather than failing loudly.
func (e *Evaluator) Eval(x ast.Expr, env *Env) Value {
	switch n := x.(type) {
	case nil:
		return nil

	case *ast.Lit:
		return litValue(n)

	case *ast.EventVar:
		return env.Event

	case *ast.Name:
		return e.evalName(n, env)

	case *ast.Index:
		row, ok := e.S.Get(n.Table, e.Eval(n.Key, env))
		if !ok {
			return nil
		}
		return e.walk(Record{Table: n.Table, Row: row}, n.Path)

	case *ast.Call:
		args := make([]Value, len(n.Args))
		for i, a := range n.Args {
			args[i] = e.Eval(a, env)
		}
		return e.call(n.Fn, args)

	case *ast.Unary:
		v := e.Eval(n.X, env)
		if n.Op == "not" {
			return !truthy(v)
		}
		if f, ok := number(v); ok {
			if isInt(v) {
				return -int64(f)
			}
			return -f
		}
		return nil

	case *ast.Binary:
		return e.evalBinary(n, env)

	case *ast.Cond:
		if truthy(e.Eval(n.If, env)) {
			return e.Eval(n.Then, env)
		}
		return e.Eval(n.Else, env)
	}
	return nil
}

// LitValue converts a literal to a runtime value, for callers that need one
// without a whole evaluator.
func LitValue(n *ast.Lit) Value { return litValue(n) }

func litValue(n *ast.Lit) Value {
	switch n.Kind {
	case "string":
		return n.Text
	case "number":
		if strings.Contains(n.Text, ".") {
			f, _ := strconv.ParseFloat(n.Text, 64)
			return f
		}
		i, _ := strconv.ParseInt(n.Text, 10, 64)
		return i
	case "bool":
		return n.Text == "true"
	}
	return nil
}

func (e *Evaluator) evalName(n *ast.Name, env *Env) Value {
	head, rest := n.Parts[0], n.Parts[1:]
	switch head {
	case "params":
		if len(rest) == 0 {
			return nil
		}
		return env.Params[rest[0]]
	case "session":
		if len(rest) == 0 {
			return nil
		}
		return e.walkValue(env.Session[rest[0]], rest[1:])
	}
	if v, ok := env.Vars[head]; ok {
		return e.walkValue(v, rest)
	}
	// Inside a where clause a bare name is a field of the row being tested.
	if env.Row != nil {
		if v, ok := env.Row.Row[head]; ok {
			return e.walkValue(v, rest)
		}
	}
	// A bare enum value is its own name.
	if len(rest) == 0 {
		return head
	}
	return nil
}

// walk follows a field path from a record, dereferencing each ref it crosses.
func (e *Evaluator) walk(rec Record, path []string) Value {
	var v Value = rec
	for _, part := range path {
		r, ok := v.(Record)
		if !ok {
			return nil
		}
		next, ok := r.Row[part]
		if !ok {
			return nil
		}
		v = next
		// If this field references another table and the path continues, read
		// through it.
		if t, ok := e.P.Table(r.Table); ok {
			if f, ok := t.Field(part); ok && f.Type == "ref" {
				if row, ok := e.S.Get(f.Ref, next); ok {
					v = Record{Table: f.Ref, Row: row}
				}
			}
		}
	}
	if r, ok := v.(Record); ok && len(path) > 0 {
		// A path that ends on a reference yields the referenced id, matching
		// how the checker types it.
		if id, ok := r.Row["id"]; ok {
			return id
		}
	}
	return v
}

// walkValue follows a path from any value, resolving a reference id into the
// record it names.
func (e *Evaluator) walkValue(v Value, path []string) Value {
	if len(path) == 0 {
		return v
	}
	if rec, ok := v.(Record); ok {
		return e.walk(rec, path)
	}
	// A list narrows to one of its columns, which is what `sum` and `join`
	// take. Only one step: a column of values has no fields of its own.
	if l, ok := v.(List); ok && len(path) == 1 && l.Field == "" {
		return List{Table: l.Table, Rows: l.Rows, Field: path[0]}
	}
	// A bare id in scope: find what it points at using the first path step.
	return nil
}

func (e *Evaluator) evalBinary(n *ast.Binary, env *Env) Value {
	// and/or short-circuit, which matters when one side reads a missing row.
	switch n.Op {
	case "and":
		return truthy(e.Eval(n.L, env)) && truthy(e.Eval(n.R, env))
	case "or":
		return truthy(e.Eval(n.L, env)) || truthy(e.Eval(n.R, env))
	}

	l := e.Eval(n.L, env)
	r := e.Eval(n.R, env)
	switch n.Op {
	case "==":
		return equal(l, r)
	case "!=":
		return !equal(l, r)
	case "<", "<=", ">", ">=":
		c, ok := compare(l, r)
		if !ok {
			return false
		}
		switch n.Op {
		case "<":
			return c < 0
		case "<=":
			return c <= 0
		case ">":
			return c > 0
		}
		return c >= 0
	case "+":
		if _, ok := l.(string); ok {
			return Text(l) + Text(r)
		}
		if _, ok := r.(string); ok {
			return Text(l) + Text(r)
		}
	}
	lf, lok := number(l)
	rf, rok := number(r)
	if !lok || !rok {
		return nil
	}
	whole := isInt(l) && isInt(r)
	var out float64
	switch n.Op {
	case "+":
		out = lf + rf
	case "-":
		out = lf - rf
	case "*":
		out = lf * rf
	case "/":
		if rf == 0 {
			return nil
		}
		out = lf / rf
		whole = false
	case "%":
		if rf == 0 {
			return nil
		}
		out = float64(int64(lf) % int64(rf))
	}
	if whole {
		return int64(out)
	}
	return out
}

// Query runs one data key and returns a Record, a List, or nil.
func (e *Evaluator) Query(q *ast.Query, env *Env) Value {
	var matched []Row
	for _, row := range e.S.All(q.Table) {
		inner := env.child()
		inner.Row = &Record{Table: q.Table, Row: row}
		if q.Where == nil || truthy(e.Eval(q.Where, inner)) {
			matched = append(matched, row)
		}
	}
	if len(q.Order) > 0 {
		keys := make([]SortKey, len(q.Order))
		for i, o := range q.Order {
			keys[i] = SortKey{Field: o.Field, Desc: o.Desc}
		}
		sortRows(matched, keys)
	}
	if q.Offset != nil {
		if n, ok := number(e.Eval(q.Offset, env)); ok && int(n) < len(matched) {
			matched = matched[int(n):]
		} else if ok {
			matched = nil
		}
	}
	if q.Limit > 0 && len(matched) > q.Limit {
		matched = matched[:q.Limit]
	}

	if q.Card == "one" {
		if len(matched) == 0 {
			return nil
		}
		return Record{Table: q.Table, Row: matched[0]}
	}
	return List{Table: q.Table, Rows: matched}
}

// RunData evaluates a route's whole data block, binding each key in turn so a
// later query can read an earlier one.
func (e *Evaluator) RunData(r *ast.Route, env *Env) *Env {
	out := env.child()
	for _, q := range r.Data {
		out.Vars[q.Key] = e.Query(q, out)
	}
	return out
}

var _ = fmt.Sprint
var _ = time.Now
