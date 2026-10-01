// Package server runs a Kiln program over HTTP.
//
// Every request follows the same path: resolve the route, apply its guard, run
// its data block, render. Every interaction follows the same path: enforce the
// action's allow rule, mutate, re-run the route's data block, send back fresh
// markup. There is no client state to reconcile and no boundary to get wrong.
package server

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/chrisnordrum/kiln/internal/ast"
	"github.com/chrisnordrum/kiln/internal/eval"
	"github.com/chrisnordrum/kiln/internal/render"
	"github.com/chrisnordrum/kiln/internal/runner"
)

//go:embed client.js
var clientJS string

//go:embed style.css
var styleCSS string

// Server holds the program and the store every request shares.
type Server struct {
	p    *ast.Program
	mu   sync.Mutex
	r    *runner.Runner
	data string // file to keep the store in, empty for in-memory only
}

// New creates a server over a fresh in-memory store.
func New(p *ast.Program) *Server {
	return &Server{p: p, r: runner.New(p)}
}

// UseFile makes the server durable: it reads the store from path if that file
// exists, and writes it back after every action. A missing file is not an
// error — it is the first run, and the caller seeds instead.
//
// Only `kiln dev` calls this. `kiln test` and `kiln snap` start empty every
// time on purpose.
func (s *Server) UseFile(path string) (loaded bool, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		s.data = path
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := s.r.Store().Restore(b); err != nil {
		// The file is adopted only once it has been read. A caller that
		// carried on past this error would otherwise save over the file it
		// could not understand, which is the one copy of the data.
		return false, fmt.Errorf("%s: %w", path, err)
	}
	s.data = path
	return true, nil
}

// Save writes the store out, if the server was given a file. Callers already
// holding the lock use save.
func (s *Server) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save()
}

func (s *Server) save() error {
	if s.data == "" {
		return nil
	}
	b, err := s.r.Store().Snapshot()
	if err != nil {
		return err
	}
	// Write beside the file and rename over it, so an interrupted save cannot
	// leave a half-written file where the data used to be.
	tmp := s.data + ".writing"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.data)
}

// Seed inserts a row before serving, so `kiln dev` starts with something to
// look at rather than an empty database.
func (s *Server) Seed(table string, fields map[string]eval.Value) error {
	return s.r.Seed(table, fields)
}

// SignIn adopts an identity for the dev session.
func (s *Server) SignIn(name string, v eval.Value) { s.r.SignIn(name, v) }

// Handler builds the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/_k/client.js", asset("application/javascript", clientJS))
	mux.HandleFunc("/_k/style.css", asset("text/css", styleCSS))
	mux.HandleFunc("/_k/action", s.handleAction)
	mux.HandleFunc("/", s.handlePage)
	return mux
}

func asset(contentType, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType+"; charset=utf-8")
		fmt.Fprint(w, body)
	}
}

func (s *Server) handlePage(w http.ResponseWriter, req *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	page, err := s.r.Visit(req.URL.Path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		s.writeShell(w, "Not found", fmt.Sprintf(
			"<main><h1>No route serves %s</h1><p>Routes are declared in routes/.</p></main>",
			html.EscapeString(req.URL.Path)))
		return
	}
	if page.Redirect != "" {
		http.Redirect(w, req, page.Redirect, http.StatusSeeOther)
		return
	}
	body, title := s.renderRoute(page.Route, req.URL.Path)
	s.writeShell(w, title, body)
}

// renderRoute re-runs a route's data block and renders it. Both a page load
// and an action response go through here, so what an interaction returns is
// exactly what a reload would produce.
func (s *Server) renderRoute(route *ast.Route, path string) (body, title string) {
	params := s.r.Params(route, path)
	env := &eval.Env{Params: params, Session: s.r.Session, Vars: map[string]eval.Value{}}
	bound := s.r.E.RunData(route, env)
	return render.HTML(s.r.E, route, bound), render.Title(s.r.E, route, bound)
}

type actionRequest struct {
	Action string         `json:"action"`
	Args   map[string]any `json:"args"`
	Path   string         `json:"path"`
}

type actionResponse struct {
	HTML     string `json:"html,omitempty"`
	Redirect string `json:"redirect,omitempty"`
	Toast    string `json:"toast,omitempty"`
	Error    string `json:"error,omitempty"`
}

func (s *Server) handleAction(w http.ResponseWriter, req *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var in actionRequest
	if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
		writeJSON(w, actionResponse{Error: "could not read the request"})
		return
	}
	action, ok := s.p.Action(in.Action)
	if !ok {
		writeJSON(w, actionResponse{Error: "no action named " + in.Action})
		return
	}

	args, err := coerceArgs(action, in.Args)
	if err != nil {
		writeJSON(w, actionResponse{Error: err.Error()})
		return
	}
	if err := s.r.Call(action, args); err != nil {
		if err == runner.Denied {
			writeJSON(w, actionResponse{Error: "not allowed"})
			return
		}
		writeJSON(w, actionResponse{Error: err.Error()})
		return
	}
	// The action has already changed the store, so a failed save is not a
	// failed action — it is the file and the running server disagreeing, which
	// the developer has to be told about rather than discover at the next
	// restart.
	if err := s.save(); err != nil {
		log.Printf("WARNING: %s is now behind the running store: %v", s.data, err)
	}

	out := actionResponse{}
	for _, st := range action.After {
		switch x := st.(type) {
		case *ast.Goto:
			out.Redirect = x.Path
		case *ast.Toast:
			out.Toast = x.Msg
		}
	}
	if out.Redirect == "" {
		route, _, ok := s.r.Match(in.Path)
		if ok {
			out.HTML, _ = s.renderRoute(route, in.Path)
		}
	}
	writeJSON(w, out)
}

// coerceArgs converts JSON values into the types the action declared. The
// browser sends every form field as text, so this is where "3" becomes 3.
func coerceArgs(a *ast.Action, in map[string]any) (map[string]eval.Value, error) {
	out := map[string]eval.Value{}
	for _, p := range a.In {
		raw, ok := in[p.Name]
		if !ok {
			return nil, fmt.Errorf("%s needs %s", a.Name, p.Name)
		}
		// A browser sends an untouched field as the empty string. For a
		// nullable parameter that means "not given", and turning it into null
		// here is what stops "" and null both meaning empty downstream.
		if p.Nullable && (raw == nil || raw == "") {
			out[p.Name] = nil
			continue
		}
		v, err := eval.Coerce(raw, p.Type)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", p.Name, err)
		}
		out[p.Name] = v
	}
	return out, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// writeShell wraps rendered markup in the document every page shares.
func (s *Server) writeShell(w http.ResponseWriter, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<link rel="stylesheet" href="/_k/style.css">
</head>
<body>
%s<script src="/_k/client.js"></script>
</body>
</html>
`, html.EscapeString(title), body)
}

// Listen serves until the process is stopped.
func (s *Server) Listen(addr string) error {
	log.Printf("kiln dev listening on http://%s", addr)
	return http.ListenAndServe(addr, s.Handler())
}
