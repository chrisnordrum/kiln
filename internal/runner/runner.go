// Package runner executes a Kiln program's tests and renders its routes.
//
// It is the shared core beneath `kiln test` and `kiln snap`: both need to seed
// data, adopt a session, call actions with permissions enforced, and render a
// route the way a request would.
package runner

import (
	"fmt"
	"strings"

	"kiln/internal/ast"
	"kiln/internal/eval"
	"kiln/internal/render"
)

// Denied is returned when an action's allow rule refuses the caller. It is an
// ordinary outcome, not a failure: tests assert on it.
var Denied = fmt.Errorf("denied")

// Runner holds one program and the store it is acting on.
type Runner struct {
	P       *ast.Program
	E       *eval.Evaluator
	Session map[string]eval.Value
}

// New starts a runner with an empty store.
func New(p *ast.Program) *Runner {
	return &Runner{P: p, E: eval.New(p), Session: map[string]eval.Value{}}
}

// env builds the environment an expression sees at the top of a request.
func (r *Runner) env() *eval.Env {
	return &eval.Env{
		Params:  map[string]eval.Value{},
		Session: r.Session,
		Vars:    map[string]eval.Value{},
	}
}

// Seed inserts a row directly, bypassing permissions, the way a fixture does.
func (r *Runner) Seed(table string, fields map[string]eval.Value) error {
	_, err := r.E.S.Insert(table, eval.Row(fields))
	return err
}

// SignIn adopts a session identity: `as user=1`.
func (r *Runner) SignIn(name string, v eval.Value) { r.Session[name] = v }

// Call runs an action with its allow rule enforced. The rule is evaluated
// before any statement executes, so a denied call leaves the store untouched.
func (r *Runner) Call(a *ast.Action, args map[string]eval.Value) error {
	env := r.env()
	for k, v := range args {
		env.Vars[k] = v
	}
	if a.Allow != nil && !truthy(r.E.Eval(a.Allow, env)) {
		return Denied
	}
	for _, s := range a.Do {
		if err := r.exec(s, env); err != nil {
			return err
		}
	}
	for _, s := range a.After {
		if err := r.exec(s, env); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) exec(s ast.Stmt, env *eval.Env) error {
	switch x := s.(type) {
	case *ast.Set:
		return r.E.S.Update(x.Table, r.E.Eval(x.Key, env), x.Field, r.E.Eval(x.Value, env))

	case *ast.SetSession:
		v := r.E.Eval(x.Value, env)
		if rec, ok := v.(eval.Record); ok {
			v = rec.Row["id"]
		}
		r.Session[x.Name] = v
		return nil

	case *ast.New:
		row := eval.Row{}
		for _, f := range x.Fields {
			row[f.Name] = r.E.Eval(f.Value, env)
		}
		_, err := r.E.S.Insert(x.Table, row)
		return err

	case *ast.Del:
		return r.E.S.Delete(x.Table, r.E.Eval(x.Key, env))

	case *ast.Send, *ast.Refresh, *ast.Goto, *ast.Toast:
		// Effects and post-action navigation are the server's business; a test
		// asserts on the data they leave behind.
		return nil
	}
	return nil
}

// Page is a rendered route.
type Page struct {
	Route    *ast.Route
	Text     string
	Redirect string // set when the guard refused
}

// Visit resolves a path to a route, applies its guard, runs its data block and
// renders it.
func (r *Runner) Visit(path string) (*Page, error) {
	route, params, ok := r.match(path)
	if !ok {
		return nil, fmt.Errorf("no route serves %s", path)
	}
	env := r.env()
	env.Params = params

	if route.Guard != nil && !truthy(r.E.Eval(route.Guard.Cond, env)) {
		return &Page{Route: route, Redirect: route.Guard.Redirect}, nil
	}
	bound := r.E.RunData(route, env)
	return &Page{Route: route, Text: render.Text(r.E, route, bound)}, nil
}

// match finds the route serving a concrete path and extracts its parameters.
func (r *Runner) match(path string) (*ast.Route, map[string]eval.Value, bool) {
	for _, route := range r.P.Routes {
		if route.Path == path {
			return route, map[string]eval.Value{}, true
		}
	}
	want := strings.Split(path, "/")
	for _, route := range r.P.Routes {
		got := strings.Split(route.Path, "/")
		if len(got) != len(want) {
			continue
		}
		params := map[string]eval.Value{}
		matched := true
		for i := range got {
			if strings.HasPrefix(got[i], ":") {
				params[got[i][1:]] = want[i]
				continue
			}
			if got[i] != want[i] {
				matched = false
				break
			}
		}
		if matched {
			return route, params, true
		}
	}
	return nil, nil, false
}

// Eval exposes expression evaluation for test expectations.
func (r *Runner) Eval(x ast.Expr) eval.Value { return r.E.Eval(x, r.env()) }

func truthy(v eval.Value) bool {
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
