// Package check resolves names and types across a whole Kiln program.
//
// This is where the language earns its keep. A frontend framework can tell you
// a component compiles; it cannot tell you that a button's handler exists,
// takes the arguments you passed, and is reachable by the people the route
// lets in.
package check

import (
	"strings"

	"github.com/chrisnordrum/kiln/internal/ast"
)

// Kind is a value's broad shape.
type Kind uint8

const (
	Unknown Kind = iota
	Text
	Int
	Num
	Bool
	At
	ID
	Enum
	Ref    // a foreign key: the id of a row in Table
	Record // a whole row
	List   // many rows
	Null
	Path  // a route path, checked against declared routes
	Param // a URL path segment, which is text until it is used
)

// Type is a value's type. Table names the table for Ref, Record and List;
// Values holds an enum's permitted values.
//
// Elem is set only on a List that has been narrowed to one column, and holds
// that column's kind. An ordinary list of rows leaves it Unknown, which is
// what lets `sum` refuse a whole list instead of quietly adding up every
// numeric cell in it.
type Type struct {
	Kind   Kind
	Table  string
	Values []string
	Elem   Kind
}

var (
	tText    = Type{Kind: Text}
	tInt     = Type{Kind: Int}
	tNum     = Type{Kind: Num}
	tBool    = Type{Kind: Bool}
	tAt      = Type{Kind: At}
	tNull    = Type{Kind: Null}
	tUnknown = Type{Kind: Unknown}
	tPath    = Type{Kind: Path}
)

func refTo(table string) Type    { return Type{Kind: Ref, Table: table} }
func recordOf(table string) Type { return Type{Kind: Record, Table: table} }
func listOf(table string) Type   { return Type{Kind: List, Table: table} }

// String names a type the way the reference spells it.
func (t Type) String() string {
	switch t.Kind {
	case Text:
		return "text"
	case Int:
		return "int"
	case Num:
		return "num"
	case Bool:
		return "bool"
	case At:
		return "at"
	case ID:
		return "id"
	case Enum:
		return "enum " + strings.Join(t.Values, " ")
	case Ref:
		return "ref " + t.Table
	case Record:
		return t.Table
	case List:
		return "many " + t.Table
	case Null:
		return "null"
	case Path:
		return "path"
	case Param:
		return "url parameter"
	}
	return "unknown"
}

// numeric reports whether arithmetic and ordering apply.
func (t Type) numeric() bool {
	switch t.Kind {
	case Int, Num, ID:
		return true
	}
	// A ref is an id underneath, so it orders and compares like one, and a URL
	// parameter is whatever the field it is compared against needs.
	return t.Kind == Ref || t.Kind == Param
}

// comparable reports whether two types may be compared with == or !=.
//
// Unknown compares with anything: it means an earlier diagnostic already
// explained the problem, and repeating it here would bury the root cause.
func comparable(a, b Type) bool {
	if a.Kind == Unknown || b.Kind == Unknown {
		return true
	}
	if a.Kind == Null || b.Kind == Null {
		return true
	}
	// A URL parameter arrives as text and is coerced by whatever it meets, so
	// it compares with any scalar. Checking it further would mean guessing.
	if a.Kind == Param || b.Kind == Param {
		return true
	}
	if a.Kind == Ref && b.Kind == Ref {
		return a.Table == b.Table
	}
	// An id compares with a ref to any table only when the other side is that
	// table's own id; without that context, allow id against ref.
	if (a.Kind == ID && b.Kind == Ref) || (a.Kind == Ref && b.Kind == ID) {
		return true
	}
	if a.numeric() && b.numeric() {
		return true
	}
	if a.Kind == Enum && b.Kind == Text || a.Kind == Text && b.Kind == Enum {
		return true
	}
	if a.Kind == Path && b.Kind == Text || a.Kind == Text && b.Kind == Path {
		return true
	}
	// A timestamp has no literal form, so a fixture spells one as text. Without
	// this, no test could seed a distinct `at` value and every time-ordered
	// query would be untestable.
	if a.Kind == At && b.Kind == Text || a.Kind == Text && b.Kind == At {
		return true
	}
	return a.Kind == b.Kind
}

// fieldType converts a schema field to a value type.
func fieldType(f *ast.Field) Type {
	switch f.Type {
	case "id":
		return Type{Kind: ID}
	case "text":
		return tText
	case "int":
		return tInt
	case "num":
		return tNum
	case "bool":
		return tBool
	case "at":
		return tAt
	case "enum":
		return Type{Kind: Enum, Values: f.Enum}
	case "ref":
		return refTo(f.Ref)
	}
	return tUnknown
}

// paramType converts an action parameter to a value type.
func paramType(p *ast.Param) Type {
	switch p.Type {
	case "text":
		return tText
	case "int":
		return tInt
	case "num":
		return tNum
	case "bool":
		return tBool
	case "at":
		return tAt
	case "ref":
		return refTo(p.Ref)
	case "enum":
		return Type{Kind: Enum, Values: p.Enum}
	}
	return tUnknown
}

// FieldTypeNames are the types a schema field may declare.
var FieldTypeNames = []string{"id", "text", "int", "num", "bool", "at", "enum", "ref"}

// ParamTypeNames are the types an action parameter may declare.
var ParamTypeNames = []string{"text", "int", "num", "bool", "at", "ref", "enum"}
