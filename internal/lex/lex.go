// Package lex turns Kiln source into an indentation tree of token lines.
//
// Every Kiln construct has the same physical shape — a line of tokens, plus
// optional children indented one level in — so the lexer stays uniform and all
// language-specific knowledge lives further up the pipeline.
package lex

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/chrisnordrum/kiln/internal/diag"
)

// Structural limits from design rule 9. They exist so an agent's edits land:
// deep trees force exact-indentation matching, and long files cost a whole
// read to change one line.
const (
	IndentWidth = 2
	// MaxDepth is a safety net against runaway indentation, not a design rule.
	// The meaningful limit is on view nesting and lives in the checker, which
	// is the only place that knows a view from a schema.
	MaxDepth = 10
	MaxLines = 120
)

// Kind classifies a token.
type Kind uint8

const (
	Ident  Kind = iota // bare word: keyword, name, type
	Number             // 12, 1.5
	String             // "text"
	Path               // /projects/:id
	Var                // $value
	Op                 // punctuation and operators
)

// Token is one lexical unit with its position.
type Token struct {
	Kind Kind
	Text string
	Line int
	Col  int
}

// Is reports whether the token is an identifier with the given text.
func (t Token) Is(word string) bool { return t.Kind == Ident && t.Text == word }

// IsOp reports whether the token is the given operator.
func (t Token) IsOp(op string) bool { return t.Kind == Op && t.Text == op }

// Line is one source line and the block indented beneath it.
type Line struct {
	File     string
	Num      int
	Depth    int
	Tokens   []Token
	Children []*Line
}

// Head returns the first token's text, which selects how a line is parsed.
func (l *Line) Head() string {
	if len(l.Tokens) == 0 {
		return ""
	}
	return l.Tokens[0].Text
}

// Child returns the first child whose head matches, and whether one existed.
func (l *Line) Child(head string) (*Line, bool) {
	for _, c := range l.Children {
		if c.Head() == head {
			return c, true
		}
	}
	return nil, false
}

// multi-character operators, longest first so ">=" wins over ">".
var operators = []string{"==", "!=", "<=", ">=", "<", ">", "=", "+", "-", "*", "/", "%",
	"(", ")", ",", ".", "[", "]", "?", ":"}

// Lex reads source into top-level lines. Problems are reported into d; Lex
// recovers where it can so one bad line does not hide the rest of the file.
func Lex(file, src string, d *diag.List) []*Line {
	var roots []*Line
	stack := []*Line{{Depth: -1}} // sentinel parent for depth 0

	raw := strings.Split(src, "\n")
	if n := countCode(raw); n > MaxLines {
		d.Add(diag.Diag{Code: "K013", File: file, Line: MaxLines + 1,
			Msg: sprintf("file is %d lines, the cap is %d", n, MaxLines),
			Fix: "split this into another route, action or schema file"})
	}

	for i, text := range raw {
		num := i + 1
		body, indent, ok := measure(file, num, text, d)
		if !ok {
			continue
		}
		depth := indent / IndentWidth
		if depth > MaxDepth {
			d.Add(diag.Diag{Code: "K012", File: file, Line: num,
				Msg: sprintf("indented %d levels, which is past anything Kiln nests", depth),
				Fix: "check the indentation; a declaration never needs this depth"})
			continue
		}
		tokens := tokenize(file, num, indent, body, d)
		if len(tokens) == 0 {
			continue
		}
		line := &Line{File: file, Num: num, Depth: depth, Tokens: tokens}

		for len(stack) > 1 && stack[len(stack)-1].Depth >= depth {
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1]
		if depth > parent.Depth+1 {
			d.Add(diag.Diag{Code: "K003", File: file, Line: num,
				Msg: sprintf("indented %d levels under a line at level %d", depth, parent.Depth),
				Fix: sprintf("indent this %d spaces", (parent.Depth+1)*IndentWidth)})
			line.Depth = parent.Depth + 1
			depth = line.Depth
		}
		if depth == 0 {
			roots = append(roots, line)
		} else {
			parent.Children = append(parent.Children, line)
		}
		stack = append(stack, line)
	}
	return roots
}

// countCode counts lines that carry code, so blank lines and comments used for
// breathing room do not push a file over the cap.
func countCode(raw []string) int {
	n := 0
	for _, s := range raw {
		t := strings.TrimSpace(s)
		if t != "" && !strings.HasPrefix(t, "#") {
			n++
		}
	}
	return n
}

// measure splits a physical line into indentation and body, dropping comments.
// ok is false for lines with nothing to parse.
func measure(file string, num int, text string, d *diag.List) (string, int, bool) {
	indent := 0
	for indent < len(text) && text[indent] == ' ' {
		indent++
	}
	if indent < len(text) && text[indent] == '\t' {
		d.Add(diag.Diag{Code: "K001", File: file, Line: num, Col: indent + 1,
			Msg: "tab in indentation", Fix: "indent with two spaces per level"})
		return "", 0, false
	}
	body := stripComment(text[indent:])
	if strings.TrimSpace(body) == "" {
		return "", 0, false
	}
	if indent%IndentWidth != 0 {
		d.Add(diag.Diag{Code: "K002", File: file, Line: num, Col: 1,
			Msg: sprintf("indented %d spaces, which is not a multiple of %d", indent, IndentWidth),
			Fix: sprintf("indent %d or %d spaces", indent-indent%IndentWidth, indent+IndentWidth-indent%IndentWidth)})
		indent -= indent % IndentWidth
	}
	return strings.TrimRight(body, " "), indent, true
}

// stripComment removes a trailing # comment, ignoring a # inside a string.
func stripComment(s string) string {
	inString := false
	for i, r := range s {
		switch {
		case r == '"':
			inString = !inString
		case r == '#' && !inString:
			return s[:i]
		}
	}
	return s
}

// tokenize scans one line's body into tokens. col is 1-based in the original
// line, so indent is added back.
func tokenize(file string, num, indent int, body string, d *diag.List) []Token {
	var out []Token
	r := []rune(body)
	i := 0
	for i < len(r) {
		if r[i] == ' ' {
			i++
			continue
		}
		col := indent + i + 1
		switch {
		case r[i] == '"':
			j := i + 1
			for j < len(r) && r[j] != '"' {
				j++
			}
			if j >= len(r) {
				d.Add(diag.Diag{Code: "K004", File: file, Line: num, Col: col,
					Msg: "unterminated string", Fix: `close it with a " on this line`})
				return out
			}
			out = append(out, Token{String, string(r[i+1 : j]), num, col})
			i = j + 1
		case r[i] == '$':
			j := i + 1
			for j < len(r) && isWord(r[j]) {
				j++
			}
			out = append(out, Token{Var, string(r[i+1 : j]), num, col})
			i = j
		case r[i] == '/' && i+1 < len(r) && isPathRune(r[i+1]):
			j := i
			for j < len(r) && r[j] != ' ' {
				j++
			}
			out = append(out, Token{Path, string(r[i:j]), num, col})
			i = j
		case unicode.IsDigit(r[i]):
			// Sign is not lexed: "-" is always an operator, and the expression
			// parser reads a leading one as negation. That keeps "text -5" and
			// "text a - 5" from depending on spacing to mean different things.
			j := i
			for j < len(r) && (unicode.IsDigit(r[j]) || r[j] == '.') {
				j++
			}
			out = append(out, Token{Number, string(r[i:j]), num, col})
			i = j
		case isWordStart(r[i]):
			j := i
			for j < len(r) && isWord(r[j]) {
				j++
			}
			out = append(out, Token{Ident, string(r[i:j]), num, col})
			i = j
		default:
			if op, n := matchOp(r[i:]); n > 0 {
				out = append(out, Token{Op, op, num, col})
				i += n
				continue
			}
			d.Add(diag.Diag{Code: "K005", File: file, Line: num, Col: col,
				Msg: sprintf("unexpected character %q", string(r[i])),
				Fix: "see `kiln docs --section expr` for the operators Kiln has"})
			i++
		}
	}
	return out
}

func matchOp(r []rune) (string, int) {
	s := string(r)
	for _, op := range operators {
		if strings.HasPrefix(s, op) {
			return op, len(op)
		}
	}
	return "", 0
}

func isWordStart(r rune) bool { return unicode.IsLetter(r) || r == '_' }
func isWord(r rune) bool      { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
func isPathRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == ':' || r == '/' || r == '_' || r == '-'
}

// sprintf is fmt.Sprintf, named short because diagnostics use it constantly.
func sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }
