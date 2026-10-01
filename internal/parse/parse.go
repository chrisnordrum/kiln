// Package parse turns lexed lines into a typed syntax tree.
//
// Parsing is permissive about meaning and strict about shape: it records what
// was written and leaves every question of "does this name exist" to the
// checker, so one pass reports structure and the next reports references.
package parse

import (
	"strconv"

	"github.com/chrisnordrum/kiln/internal/ast"
	"github.com/chrisnordrum/kiln/internal/diag"
	"github.com/chrisnordrum/kiln/internal/lex"
)

// File is one source file to parse.
type File struct {
	Path string
	Src  string
}

// Program parses every file into one tree.
func Program(files []File, d *diag.List) *ast.Program {
	p := &ast.Program{}
	for _, f := range files {
		for _, line := range lex.Lex(f.Path, f.Src, d) {
			parseTop(p, line, d)
		}
	}
	return p
}

func parseTop(p *ast.Program, line *lex.Line, d *diag.List) {
	switch line.Head() {
	case "app":
		if app := parseApp(line, d); app != nil {
			p.App = app
		}
	case "table":
		if t := parseTable(line, d); t != nil {
			p.Tables = append(p.Tables, t)
		}
	case "action":
		if a := parseAction(line, d); a != nil {
			p.Actions = append(p.Actions, a)
		}
	case "route":
		if r := parseRoute(line, d); r != nil {
			p.Routes = append(p.Routes, r)
		}
	case "test":
		if t := parseTest(line, d); t != nil {
			p.Tests = append(p.Tests, t)
		}
	default:
		d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
			Msg:  sprintf("%q does not start a declaration", line.Head()),
			Near: diag.Suggest(line.Head(), []string{"app", "table", "action", "route", "test"}),
			Fix:  "a file declares app, table, action, route or test at the top level"})
	}
}

// cur builds a cursor over a line's tokens, skipping the head word.
func cur(line *lex.Line, d *diag.List) *cursor {
	return &cursor{toks: line.Tokens[1:], file: line.File, d: d}
}

// pos is a line's position.
func pos(line *lex.Line) ast.Pos {
	return ast.Pos{File: line.File, Line: line.Num, Col: 1}
}

// name reads the identifier that names a declaration.
func name(line *lex.Line, d *diag.List) string {
	if len(line.Tokens) < 2 || line.Tokens[1].Kind != lex.Ident {
		d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
			Msg: sprintf("%s needs a name", line.Head()),
			Fix: sprintf("write: %s <name>", line.Head())})
		return ""
	}
	return line.Tokens[1].Text
}

// missing reports a required block that was not written.
func missing(line *lex.Line, block string, d *diag.List) {
	d.Add(diag.Diag{Code: "K011", File: line.File, Line: line.Num,
		Msg: sprintf("%s %s has no %s block", line.Head(), nameOf(line), block),
		Fix: sprintf("add a %s block indented under it", block)})
}

func nameOf(line *lex.Line) string {
	if len(line.Tokens) > 1 {
		return line.Tokens[1].Text
	}
	return "?"
}

// ---------- app ----------

func parseApp(line *lex.Line, d *diag.List) *ast.App {
	app := &ast.App{Pos: pos(line), Name: name(line, d)}
	for _, ch := range line.Children {
		c := cur(ch, d)
		switch ch.Head() {
		case "title":
			if e := parseExpr(c); e != nil {
				if lit, ok := e.(*ast.Lit); ok {
					app.Title = lit.Text
				}
			}
		case "session":
			decl := &ast.SessionDecl{Pos: pos(ch)}
			if !c.eof() {
				decl.Name = c.next().Text
			}
			if c.acceptWord("ref") && !c.eof() {
				decl.Table = c.next().Text
			}
			app.Session = append(app.Session, decl)
		case "effect":
			eff := &ast.Effect{Pos: pos(ch)}
			if !c.eof() {
				eff.Name = c.next().Text
			}
			if c.acceptWord("via") && !c.eof() {
				eff.Via = c.next().Text
			}
			app.Effects = append(app.Effects, eff)
		default:
			unknownBlock(ch, "app", []string{"title", "session", "effect"}, d)
		}
	}
	return app
}

func unknownBlock(line *lex.Line, in string, allowed []string, d *diag.List) {
	d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
		Msg:  sprintf("%q is not part of %s", line.Head(), in),
		Near: diag.Suggest(line.Head(), allowed),
		Fix:  sprintf("see `kiln docs --section %s`", in)})
}

// ---------- schema ----------

func parseTable(line *lex.Line, d *diag.List) *ast.Table {
	t := &ast.Table{Pos: pos(line), Name: name(line, d)}
	for _, ch := range line.Children {
		switch ch.Head() {
		case "index":
			t.Indexes = append(t.Indexes, fieldList(ch, d))
		case "unique":
			t.Uniques = append(t.Uniques, fieldList(ch, d))
		default:
			if f := parseField(ch, d); f != nil {
				t.Fields = append(t.Fields, f)
			}
		}
	}
	// Only report an empty table when it genuinely has no field lines. If a
	// field was written but failed to parse, that diagnostic is the root cause
	// and this one would just bury it.
	if len(t.Fields) == 0 && len(line.Children) == 0 {
		missing(line, "field", d)
	}
	return t
}

// fieldList reads `index a, b` — a comma-separated list of field names.
func fieldList(line *lex.Line, d *diag.List) []string {
	c := cur(line, d)
	var out []string
	for !c.eof() {
		tok := c.next()
		if tok.Kind == lex.Ident {
			out = append(out, tok.Text)
		}
		c.accept(",")
	}
	return out
}

// parseField reads one column: `name type [modifiers]`.
func parseField(line *lex.Line, d *diag.List) *ast.Field {
	if len(line.Tokens) < 2 {
		d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
			Msg: sprintf("field %q has no type", line.Head()),
			Fix: "write: <name> <type>, as in `title text max 200`"})
		return nil
	}
	f := &ast.Field{Pos: pos(line), Name: line.Head()}
	c := cur(line, d)
	f.Type = c.next().Text

	if f.Type == "ref" {
		if c.eof() {
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
				Msg: "ref needs a table", Fix: "write: <name> ref <Table>"})
			return f
		}
		f.Ref = c.next().Text
	}
	if f.Type == "enum" {
		for !c.eof() && c.peek().Kind == lex.Ident && !isFieldKeyword(c.peek().Text) {
			f.Enum = append(f.Enum, c.next().Text)
		}
	}
	if f.Type == "at" && c.acceptWord("now") {
		f.DefaultNow = true
	}
	f.Nullable = c.accept("?")

	for !c.eof() {
		switch {
		case c.acceptWord("max"):
			f.Max = intArg(c, d)
		case c.acceptWord("unique"):
			f.Unique = true
		case c.acceptWord("on"):
			// `on delete cascade|restrict|null`
			c.acceptWord("delete")
			if !c.eof() {
				f.OnDelete = c.next().Text
			}
		case c.accept("="):
			f.Default = parseExpr(c)
		case c.accept("?"):
			f.Nullable = true
		default:
			tok := c.next()
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: tok.Line, Col: tok.Col,
				Msg:  sprintf("%q is not a field modifier", tok.Text),
				Near: diag.Suggest(tok.Text, []string{"max", "unique", "on", "now"}),
				Fix:  "see `kiln docs --section schema`"})
		}
	}
	return f
}

// isFieldKeyword reports whether a word ends an enum's value list.
func isFieldKeyword(w string) bool {
	switch w {
	case "max", "unique", "on", "now":
		return true
	}
	return false
}

func intArg(c *cursor, d *diag.List) int {
	if c.eof() {
		return 0
	}
	tok := c.next()
	n, err := strconv.Atoi(tok.Text)
	if err != nil {
		d.Add(diag.Diag{Code: "K030", File: c.file, Line: tok.Line, Col: tok.Col,
			Msg: sprintf("expected a whole number, got %q", tok.Text), Fix: "use a number, as in max 200"})
		return 0
	}
	return n
}
