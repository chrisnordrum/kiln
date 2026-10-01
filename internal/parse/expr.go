package parse

import (
	"fmt"

	"github.com/chrisnordrum/kiln/internal/ast"
	"github.com/chrisnordrum/kiln/internal/diag"
	"github.com/chrisnordrum/kiln/internal/lex"
)

// binPrec is operator precedence, loosest first. `and`/`or`/`not` arrive as
// identifiers rather than punctuation, which the lookups below account for.
var binPrec = map[string]int{
	"or": 1, "and": 2,
	"==": 3, "!=": 3, "<": 3, "<=": 3, ">": 3, ">=": 3,
	"+": 4, "-": 4,
	"*": 5, "/": 5, "%": 5,
}

// notPrec is where `not` binds: tighter than and/or, looser than comparison,
// so `not a == b` reads as `not (a == b)`.
const notPrec = 3

// Precedence reports an infix operator's binding power, for the formatter.
func Precedence(op string) (int, bool) {
	p, ok := binPrec[op]
	return p, ok
}

// NotPrecedence is where a `not` operand binds.
const NotPrecedence = notPrec

// cursor walks one line's tokens.
type cursor struct {
	toks []lex.Token
	i    int
	file string
	d    *diag.List
}

func (c *cursor) eof() bool { return c.i >= len(c.toks) }

func (c *cursor) peek() lex.Token {
	if c.eof() {
		return lex.Token{}
	}
	return c.toks[c.i]
}

func (c *cursor) next() lex.Token {
	t := c.peek()
	c.i++
	return t
}

func (c *cursor) at(op string) bool { return !c.eof() && c.peek().IsOp(op) }

// atAttr reports whether a key=value pair starts here. "==" lexes as one
// token, so a comparison never looks like an assignment.
func (c *cursor) atAttr() bool {
	return c.i+1 < len(c.toks) && c.toks[c.i].Kind == lex.Ident && c.toks[c.i+1].IsOp("=")
}

func (c *cursor) atWord(w string) bool { return !c.eof() && c.peek().Is(w) }

// accept consumes the operator if present.
func (c *cursor) accept(op string) bool {
	if c.at(op) {
		c.i++
		return true
	}
	return false
}

// acceptWord consumes the keyword if present.
func (c *cursor) acceptWord(w string) bool {
	if c.atWord(w) {
		c.i++
		return true
	}
	return false
}

func (c *cursor) pos(t lex.Token) ast.Pos {
	return ast.Pos{File: c.file, Line: t.Line, Col: t.Col}
}

// here is the position of the current token, or of the last one at end of line.
func (c *cursor) here() ast.Pos {
	if c.eof() && len(c.toks) > 0 {
		return c.pos(c.toks[len(c.toks)-1])
	}
	return c.pos(c.peek())
}

// fail reports a syntax problem and yields a poison value, so one bad
// expression does not abort the rest of the file.
func (c *cursor) fail(code, msg, fix string) ast.Expr {
	p := c.here()
	c.d.Add(diag.Diag{Code: code, File: p.File, Line: p.Line, Col: p.Col, Msg: msg, Fix: fix})
	if !c.eof() {
		c.i++
	}
	return &ast.Lit{Pos: p, Kind: "null", Text: "null"}
}

// expect consumes a required operator.
func (c *cursor) expect(op string) bool {
	if c.accept(op) {
		return true
	}
	p := c.here()
	c.d.Add(diag.Diag{Code: "K005", File: p.File, Line: p.Line, Col: p.Col,
		Msg: sprintf("expected %q", op), Fix: sprintf("add the missing %q", op)})
	return false
}

// parseExpr reads a full expression.
func parseExpr(c *cursor) ast.Expr { return parseBinary(c, 0) }

func parseBinary(c *cursor, minPrec int) ast.Expr {
	left := parseUnary(c)
	for !c.eof() {
		op, prec, ok := binOp(c.peek())
		if !ok || prec < minPrec {
			break
		}
		tok := c.next()
		right := parseBinary(c, prec+1)
		left = &ast.Binary{Pos: c.pos(tok), Op: op, L: left, R: right}
	}
	return left
}

// binOp classifies a token as an infix operator.
func binOp(t lex.Token) (string, int, bool) {
	switch t.Kind {
	case lex.Op:
		if p, ok := binPrec[t.Text]; ok {
			return t.Text, p, true
		}
	case lex.Ident:
		if t.Text == "and" || t.Text == "or" {
			return t.Text, binPrec[t.Text], true
		}
	}
	return "", 0, false
}

func parseUnary(c *cursor) ast.Expr {
	if c.atWord("not") {
		tok := c.next()
		return &ast.Unary{Pos: c.pos(tok), Op: "not", X: parseBinary(c, notPrec)}
	}
	if c.at("-") {
		tok := c.next()
		return &ast.Unary{Pos: c.pos(tok), Op: "-", X: parseUnary(c)}
	}
	return parsePostfix(c)
}

// parsePostfix reads a primary and then any trailing field walk.
func parsePostfix(c *cursor) ast.Expr {
	x := parsePrimary(c)
	for c.at(".") {
		c.next()
		if c.eof() || c.peek().Kind != lex.Ident {
			return c.fail("K005", "expected a field name after '.'", "name a field, as in project.title")
		}
		part := c.next().Text
		switch n := x.(type) {
		case *ast.Name:
			n.Parts = append(n.Parts, part)
		case *ast.Index:
			n.Path = append(n.Path, part)
		default:
			return c.fail("K005", "cannot read a field from this expression",
				"a field walk starts from a name or a Table[key] lookup")
		}
	}
	return x
}

func parsePrimary(c *cursor) ast.Expr {
	if c.eof() {
		return c.fail("K005", "expression ended early", "complete the expression")
	}
	tok := c.peek()
	pos := c.pos(tok)

	switch tok.Kind {
	case lex.Number:
		c.next()
		return &ast.Lit{Pos: pos, Kind: "number", Text: tok.Text}
	case lex.String:
		c.next()
		return &ast.Lit{Pos: pos, Kind: "string", Text: tok.Text}
	case lex.Var:
		c.next()
		return &ast.EventVar{Pos: pos, Name: tok.Text}
	case lex.Path:
		c.next()
		return &ast.Lit{Pos: pos, Kind: "string", Text: tok.Text}
	case lex.Op:
		if tok.Text == "(" {
			c.next()
			inner := parseExpr(c)
			c.expect(")")
			return inner
		}
		return c.fail("K005", sprintf("unexpected %q in an expression", tok.Text),
			"see `kiln docs --section expr`")
	}

	// Identifier: a keyword, a call, a keyed lookup, or a name.
	switch tok.Text {
	case "true", "false":
		c.next()
		return &ast.Lit{Pos: pos, Kind: "bool", Text: tok.Text}
	case "null":
		c.next()
		return &ast.Lit{Pos: pos, Kind: "null", Text: "null"}
	case "if":
		return parseCond(c)
	}
	c.next()
	if c.at("(") {
		c.next()
		call := &ast.Call{Pos: pos, Fn: tok.Text}
		for !c.at(")") && !c.eof() {
			call.Args = append(call.Args, parseExpr(c))
			if !c.accept(",") {
				break
			}
		}
		c.expect(")")
		return call
	}
	if c.at("[") {
		c.next()
		idx := &ast.Index{Pos: pos, Table: tok.Text, Key: parseExpr(c)}
		c.expect("]")
		return idx
	}
	return &ast.Name{Pos: pos, Parts: []string{tok.Text}}
}

// parseCond reads `if c then a else b`, the only branch an expression has.
func parseCond(c *cursor) ast.Expr {
	tok := c.next() // if
	out := &ast.Cond{Pos: c.pos(tok)}
	out.If = parseBinary(c, 0)
	if !c.acceptWord("then") {
		return c.fail("K005", "expected `then`", "write: if <cond> then <a> else <b>")
	}
	out.Then = parseBinary(c, 0)
	if !c.acceptWord("else") {
		return c.fail("K005", "expected `else`",
			"an if expression always has an else, so it always has a value")
	}
	out.Else = parseBinary(c, 0)
	return out
}

// sprintf is fmt.Sprintf, named short because diagnostics use it constantly.
func sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }
