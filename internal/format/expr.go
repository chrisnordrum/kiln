package format

import (
	"fmt"
	"strconv"
	"strings"

	"kiln/internal/ast"
	"kiln/internal/parse"
)

// Expr renders an expression in canonical form, adding exactly the parentheses
// needed to preserve meaning on reparse — no more, so the output is stable.
func Expr(e ast.Expr) string { return exprAt(e, 0) }

// exprAt renders e in a context that binds at minPrec.
func exprAt(e ast.Expr, minPrec int) string {
	switch x := e.(type) {
	case *ast.Lit:
		switch x.Kind {
		case "string":
			return strconv.Quote(x.Text)
		default:
			return x.Text
		}
	case *ast.Name:
		return strings.Join(x.Parts, ".")
	case *ast.EventVar:
		return "$" + x.Name
	case *ast.Index:
		s := fmt.Sprintf("%s[%s]", x.Table, exprAt(x.Key, 0))
		for _, p := range x.Path {
			s += "." + p
		}
		return s
	case *ast.Call:
		args := make([]string, len(x.Args))
		for i, a := range x.Args {
			args[i] = exprAt(a, 0)
		}
		return fmt.Sprintf("%s(%s)", x.Fn, strings.Join(args, ", "))
	case *ast.Unary:
		if x.Op == "not" {
			return wrap("not "+exprAt(x.X, parse.NotPrecedence), parse.NotPrecedence, minPrec)
		}
		return "-" + exprAt(x.X, unaryPrec)
	case *ast.Binary:
		prec, ok := parse.Precedence(x.Op)
		if !ok {
			prec = 1
		}
		// Left-associative: the right operand needs one more binding power to
		// stay on the right, so a - (b - c) keeps its parentheses.
		s := exprAt(x.L, prec) + " " + x.Op + " " + exprAt(x.R, prec+1)
		return wrap(s, prec, minPrec)
	case *ast.Cond:
		s := fmt.Sprintf("if %s then %s else %s", exprAt(x.If, 0), exprAt(x.Then, 0), exprAt(x.Else, 0))
		return wrap(s, condPrec, minPrec)
	case nil:
		return ""
	}
	return e.String()
}

// unaryPrec is where a negated operand binds: tighter than any infix operator,
// so -a * b reads as (-a) * b.
const unaryPrec = 6

// condPrec is where an if expression binds: looser than everything, so it is
// parenthesized whenever it appears inside another expression.
const condPrec = 0

func wrap(s string, prec, minPrec int) string {
	if prec < minPrec {
		return "(" + s + ")"
	}
	return s
}
