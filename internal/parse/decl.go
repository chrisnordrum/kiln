package parse

import (
	"github.com/chrisnordrum/kiln/internal/ast"
	"github.com/chrisnordrum/kiln/internal/diag"
	"github.com/chrisnordrum/kiln/internal/lex"
)

// ---------- actions ----------

func parseAction(line *lex.Line, d *diag.List) *ast.Action {
	a := &ast.Action{Pos: pos(line), Name: name(line, d)}
	var sawDo bool
	for _, ch := range line.Children {
		switch ch.Head() {
		case "in":
			for _, p := range ch.Children {
				if param := parseParam(p, d); param != nil {
					a.In = append(a.In, param)
				}
			}
		case "allow":
			a.Allow = parseExpr(cur(ch, d))
		case "do":
			sawDo = true
			for _, s := range ch.Children {
				if st := parseStmt(s, d); st != nil {
					a.Do = append(a.Do, st)
				}
			}
		case "after":
			for _, s := range ch.Children {
				if st := parseStmt(s, d); st != nil {
					a.After = append(a.After, st)
				}
			}
		default:
			unknownBlock(ch, "actions", []string{"in", "allow", "do", "after"}, d)
		}
	}
	if !sawDo {
		missing(line, "do", d)
	}
	if a.Allow == nil {
		d.Add(diag.Diag{Code: "K011", File: line.File, Line: line.Num,
			Msg: sprintf("action %s has no allow rule", a.Name),
			Fix: "add `allow <expr>`; an action with no rule would be callable by anyone"})
	}
	return a
}

// paramTypes are the types an action input may declare.
var paramTypes = []string{"text", "int", "num", "bool", "at", "ref", "enum"}

// isParamKeyword stops an enum's value list at the modifiers that may follow
// it, so `status enum todo done max 20` does not read "max" as a value.
func isParamKeyword(word string) bool { return word == "max" }

// parseParam reads one action input: `name type [ref Table] [max N]`.
func parseParam(line *lex.Line, d *diag.List) *ast.Param {
	if len(line.Tokens) < 2 {
		d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
			Msg: sprintf("parameter %q has no type", line.Head()),
			Fix: "write: <name> <type>, as in `id ref Task`"})
		return nil
	}
	p := &ast.Param{Pos: pos(line), Name: line.Head()}
	c := cur(line, d)
	p.Type = c.next().Text
	if p.Type == "ref" && !c.eof() && c.peek().Kind == lex.Ident {
		p.Ref = c.next().Text
	}
	p.Nullable = c.accept("?")
	if p.Type == "enum" {
		for !c.eof() && c.peek().Kind == lex.Ident && !isParamKeyword(c.peek().Text) {
			p.Enum = append(p.Enum, c.next().Text)
		}
		if len(p.Enum) == 0 {
			d.Add(diag.Diag{Code: "K011", File: line.File, Line: line.Num,
				Msg: sprintf("enum parameter %s has no values", p.Name),
				Fix: "write: " + p.Name + " enum a b c"})
		}
	}
	for !c.eof() {
		if c.acceptWord("max") {
			p.Max = intArg(c, d)
			continue
		}
		if c.accept("?") {
			p.Nullable = true
			continue
		}
		// One mistake, one diagnostic: everything after the first stray word
		// is a consequence of it, not a separate error.
		tok := c.next()
		d.Add(diag.Diag{Code: "K010", File: line.File, Line: tok.Line, Col: tok.Col,
			Msg: sprintf("%q is not a parameter modifier", tok.Text),
			Fix: "parameters take `?` to allow null, and an optional `max N`"})
		break
	}
	return p
}

// parseStmt reads one statement from a do or after block.
func parseStmt(line *lex.Line, d *diag.List) ast.Stmt {
	c := cur(line, d)
	p := pos(line)
	switch line.Head() {
	case "set":
		target := parsePostfix(c)
		if n, ok := target.(*ast.Name); ok {
			if n.Root() != "session" || len(n.Parts) != 2 {
				d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
					Msg: sprintf("cannot assign to %s", n),
					Fix: "write: set Table[key].field = <expr>, or set session.<name> = <expr>"})
				return nil
			}
			c.expect("=")
			return &ast.SetSession{Pos: p, Name: n.Parts[1], Value: parseExpr(c)}
		}
		idx, ok := target.(*ast.Index)
		if !ok || len(idx.Path) != 1 {
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
				Msg: "set needs one field of one record",
				Fix: "write: set Table[key].field = <expr>"})
			return nil
		}
		c.expect("=")
		return &ast.Set{Pos: p, Table: idx.Table, Key: idx.Key, Field: idx.Path[0], Value: parseExpr(c)}

	case "new":
		if c.eof() {
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
				Msg: "new needs a table", Fix: "write: new Table field=<expr> ..."})
			return nil
		}
		return &ast.New{Pos: p, Table: c.next().Text, Fields: parseAttrs(c, d)}

	case "del":
		target := parsePostfix(c)
		idx, ok := target.(*ast.Index)
		if !ok {
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
				Msg: "del needs a keyed record", Fix: "write: del Table[key]"})
			return nil
		}
		return &ast.Del{Pos: p, Table: idx.Table, Key: idx.Key}

	case "send":
		if c.eof() {
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
				Msg: "send needs an effect", Fix: "write: send <effect> to=<expr> ..."})
			return nil
		}
		return &ast.Send{Pos: p, Effect: c.next().Text, Args: parseAttrs(c, d)}

	case "refresh":
		return &ast.Refresh{Pos: p}

	case "goto":
		if c.eof() {
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
				Msg: "goto needs a path", Fix: "write: goto /path"})
			return nil
		}
		return &ast.Goto{Pos: p, Path: c.next().Text}

	case "toast":
		if c.eof() {
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
				Msg: "toast needs a message", Fix: `write: toast "message"`})
			return nil
		}
		return &ast.Toast{Pos: p, Msg: c.next().Text}
	}
	d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
		Msg:  sprintf("%q is not a statement", line.Head()),
		Near: diag.Suggest(line.Head(), []string{"set", "new", "del", "send", "refresh", "goto", "toast"}),
		Fix:  "see `kiln docs --section actions`"})
	return nil
}

// parseAttrs reads trailing key=value pairs.
func parseAttrs(c *cursor, d *diag.List) []*ast.Attr {
	var out []*ast.Attr
	for c.atAttr() {
		key := c.next()
		c.next() // =
		out = append(out, &ast.Attr{Pos: c.pos(key), Name: key.Text, Value: parseExpr(c)})
	}
	if !c.eof() {
		tok := c.peek()
		d.Add(diag.Diag{Code: "K010", File: c.file, Line: tok.Line, Col: tok.Col,
			Msg: sprintf("unexpected %q", tok.Text), Fix: "arguments here are written key=value"})
		c.i = len(c.toks)
	}
	return out
}

// ---------- routes ----------

func parseRoute(line *lex.Line, d *diag.List) *ast.Route {
	r := &ast.Route{Pos: pos(line), Name: name(line, d)}
	var sawView bool
	for _, ch := range line.Children {
		c := cur(ch, d)
		switch ch.Head() {
		case "path":
			if !c.eof() {
				r.Path = c.next().Text
			}
		case "guard":
			r.Guard = parseGuard(ch, d)
		case "data":
			for _, q := range ch.Children {
				if query := parseQuery(q, d); query != nil {
					r.Data = append(r.Data, query)
				}
			}
		case "view":
			sawView = true
			r.View = parseNodes(ch.Children, d)
		default:
			unknownBlock(ch, "routes", []string{"path", "guard", "data", "view"}, d)
		}
	}
	if r.Path == "" {
		missing(line, "path", d)
	}
	if !sawView {
		missing(line, "view", d)
	}
	return r
}

// parseGuard reads `guard <expr> else redirect /path`.
func parseGuard(line *lex.Line, d *diag.List) *ast.Guard {
	c := cur(line, d)
	g := &ast.Guard{Pos: pos(line)}
	g.Cond = parseExpr(c)
	if c.acceptWord("else") {
		c.acceptWord("redirect")
		if !c.eof() {
			g.Redirect = c.next().Text
		}
	}
	if g.Redirect == "" {
		d.Add(diag.Diag{Code: "K011", File: line.File, Line: line.Num,
			Msg: "guard has no else branch",
			Fix: "write: guard <expr> else redirect /path"})
	}
	return g
}

// parseQuery reads one data key:
// `key one|many Table where <expr> [order f asc|desc] [limit n] [offset e]`.
func parseQuery(line *lex.Line, d *diag.List) *ast.Query {
	if len(line.Tokens) < 3 {
		d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
			Msg: sprintf("data key %q is incomplete", line.Head()),
			Fix: "write: <key> one|many <Table> where <expr>"})
		return nil
	}
	q := &ast.Query{Pos: pos(line), Key: line.Head()}
	c := cur(line, d)
	q.Card = c.next().Text
	if q.Card != "one" && q.Card != "many" {
		d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
			Msg:  sprintf("%q is not a cardinality", q.Card),
			Near: diag.Suggest(q.Card, []string{"one", "many"}),
			Fix:  "use `one` for a single record or `many` for a list"})
	}
	q.Table = c.next().Text
	if c.acceptWord("where") {
		q.Where = parseExpr(c)
	}
	for !c.eof() {
		switch {
		case c.acceptWord("order"):
			for !c.eof() && c.peek().Kind == lex.Ident && !isQueryKeyword(c.peek().Text) {
				o := ast.OrderBy{Field: c.next().Text}
				if c.acceptWord("desc") {
					o.Desc = true
				} else {
					c.acceptWord("asc")
				}
				q.Order = append(q.Order, o)
				if !c.accept(",") {
					break
				}
			}
		case c.acceptWord("limit"):
			q.Limit = intArg(c, d)
		case c.acceptWord("offset"):
			q.Offset = parseExpr(c)
		default:
			tok := c.next()
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: tok.Line, Col: tok.Col,
				Msg:  sprintf("%q is not part of a query", tok.Text),
				Near: diag.Suggest(tok.Text, []string{"where", "order", "limit", "offset"}),
				Fix:  "see `kiln docs --section routes`"})
		}
	}
	return q
}

func isQueryKeyword(w string) bool {
	switch w {
	case "limit", "offset", "order", "where":
		return true
	}
	return false
}

// ---------- view ----------

// parseNodes reads a view block, attaching each `else` to the `when` above it.
func parseNodes(lines []*lex.Line, d *diag.List) []*ast.Node {
	var out []*ast.Node
	for _, line := range lines {
		if line.Head() == "else" {
			if len(out) == 0 || out[len(out)-1].Kind != "when" {
				d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
					Msg: "else with no when above it",
					Fix: "put else directly after the when block it belongs to"})
				continue
			}
			out[len(out)-1].Else = parseNodes(line.Children, d)
			continue
		}
		if n := parseNode(line, d); n != nil {
			out = append(out, n)
		}
	}
	return out
}

func parseNode(line *lex.Line, d *diag.List) *ast.Node {
	n := &ast.Node{Pos: pos(line), Kind: line.Head()}
	c := cur(line, d)

	switch n.Kind {
	case "each":
		n.List = parseExpr(c)
		if !c.acceptWord("as") {
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
				Msg: "each needs a loop variable", Fix: "write: each <list> as <name>"})
		} else if !c.eof() {
			n.Var = c.next().Text
		}
	case "when":
		n.Cond = parseExpr(c)
	default:
		n.Args, n.Attrs = parseArgsAndAttrs(c, d)
	}
	if !c.eof() {
		tok := c.peek()
		d.Add(diag.Diag{Code: "K010", File: line.File, Line: tok.Line, Col: tok.Col,
			Msg: sprintf("unexpected %q after %s", tok.Text, n.Kind),
			Fix: "see `kiln docs --section view`"})
	}
	n.Children = parseNodes(line.Children, d)
	return n
}

// parseArgsAndAttrs reads positional arguments followed by key=value pairs.
// Positional arguments always come first, which keeps a line unambiguous.
func parseArgsAndAttrs(c *cursor, d *diag.List) ([]ast.Expr, []*ast.Attr) {
	var args []ast.Expr
	for !c.eof() && !c.atAttr() {
		args = append(args, parseExpr(c))
	}
	return args, parseAttrs(c, d)
}

// ---------- tests ----------

func parseTest(line *lex.Line, d *diag.List) *ast.Test {
	t := &ast.Test{Pos: pos(line)}
	if len(line.Tokens) > 1 && line.Tokens[1].Kind == lex.String {
		t.Name = line.Tokens[1].Text
	} else {
		d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
			Msg: "test needs a name", Fix: `write: test "what it proves"`})
	}
	for _, ch := range line.Children {
		if s := parseStep(ch, d); s != nil {
			t.Steps = append(t.Steps, s)
		}
	}
	// As with tables: a step that failed to parse is the root cause, so do not
	// also claim the test is empty.
	if len(t.Steps) == 0 && len(line.Children) == 0 {
		missing(line, "step", d)
	}
	return t
}

func parseStep(line *lex.Line, d *diag.List) *ast.Step {
	c := cur(line, d)
	s := &ast.Step{Pos: pos(line), Kind: line.Head()}
	switch s.Kind {
	case "seed":
		if c.eof() {
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
				Msg: "seed needs a table", Fix: "write: seed Table field=<value> ..."})
			return nil
		}
		s.Target = c.next().Text
		s.Attrs = parseAttrs(c, d)
	case "as":
		s.Attrs = parseAttrs(c, d)
	case "call":
		if c.eof() {
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
				Msg: "call needs an action", Fix: "write: call <action> arg=<value> ..."})
			return nil
		}
		s.Target = c.next().Text
		s.Attrs = parseAttrs(c, d)
	case "visit":
		if c.eof() {
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
				Msg: "visit needs a path", Fix: "write: visit /path"})
			return nil
		}
		s.Target = c.next().Text
	case "expect":
		parseExpect(c, s, line, d)
	default:
		d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
			Msg:  sprintf("%q is not a test step", s.Kind),
			Near: diag.Suggest(s.Kind, []string{"seed", "as", "call", "visit", "expect"}),
			Fix:  "see `kiln docs --section tests`"})
		return nil
	}
	return s
}

// parseExpect reads the three shapes of expectation: a condition, rendered
// text, or a denial.
func parseExpect(c *cursor, s *ast.Step, line *lex.Line, d *diag.List) {
	switch {
	case c.acceptWord("denied"):
		s.Denied = true
		if c.acceptWord("when") && c.acceptWord("as") {
			if attrs := parseAttrs(c, d); len(attrs) > 0 {
				s.As = attrs[0]
			}
		}
	case c.acceptWord("redirect"):
		if c.eof() {
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
				Msg: "expect redirect needs a path", Fix: "write: expect redirect /login"})
			return
		}
		s.Redirect = c.next().Text

	case c.atWord("no") || c.atWord("text"):
		s.Negate = c.acceptWord("no")
		if !c.acceptWord("text") {
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
				Msg: "expect no needs `text`", Fix: `write: expect no text "something"`})
			return
		}
		if c.eof() || c.peek().Kind != lex.String {
			d.Add(diag.Diag{Code: "K010", File: line.File, Line: line.Num,
				Msg: "expect text needs a string", Fix: `write: expect text "something"`})
			return
		}
		s.Text = c.next().Text
	default:
		s.Cond = parseExpr(c)
	}
}
