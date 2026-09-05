package check

import "sort"

// builtin is one stdlib function's signature.
//
// Kiln has no user-defined functions, so this table is the complete set of
// calls a program can make. A name that is not here is always a mistake, which
// is what turns a hallucinated helper into a compile error.
type builtin struct {
	params []Kind // Unknown means any type
	ret    Type
	// variadicFrom, when non-zero, lets the last declared parameter repeat.
	variadicFrom int
	// column, when set, requires the List argument to be narrowed to one
	// column — `sum(items.price)`, not `sum(items)`. Num additionally requires
	// that column to be numeric. Without it, sum read every numeric cell of
	// every row and returned a total nobody asked for.
	column Kind
}

// anyKind marks a parameter that accepts any type.
const anyKind = Unknown

var stdlib = map[string]builtin{
	// text
	"upper":    {params: []Kind{Text}, ret: tText},
	"lower":    {params: []Kind{Text}, ret: tText},
	"title":    {params: []Kind{Text}, ret: tText},
	"trim":     {params: []Kind{Text}, ret: tText},
	"slug":     {params: []Kind{Text}, ret: tText},
	"truncate": {params: []Kind{Text, Int}, ret: tText},
	"len":      {params: []Kind{Text}, ret: tInt},
	"join":     {params: []Kind{List, Text}, ret: tText, column: Text},
	"has":      {params: []Kind{Text, Text}, ret: tBool},
	"replace":  {params: []Kind{Text, Text, Text}, ret: tText},

	// numbers
	"abs":   {params: []Kind{Num}, ret: tNum},
	"round": {params: []Kind{Num}, ret: tInt},
	"floor": {params: []Kind{Num}, ret: tInt},
	"ceil":  {params: []Kind{Num}, ret: tInt},
	"min":   {params: []Kind{Num, Num}, ret: tNum},
	"max":   {params: []Kind{Num, Num}, ret: tNum},
	"sum":   {params: []Kind{List}, ret: tNum, column: Num},
	"count": {params: []Kind{List}, ret: tInt},
	"money": {params: []Kind{Num}, ret: tText},
	"pct":   {params: []Kind{Num}, ret: tText},

	// dates
	"now":          {params: nil, ret: tAt},
	"ago":          {params: []Kind{At}, ret: tText},
	"date":         {params: []Kind{At, Text}, ret: tText},
	"days_between": {params: []Kind{At, At}, ret: tInt},
	"plus_days":    {params: []Kind{At, Int}, ret: tAt},

	// general
	"coalesce": {params: []Kind{anyKind, anyKind}, ret: tUnknown},
	"plural":   {params: []Kind{Int, Text, Text}, ret: tText},
	"default":  {params: []Kind{anyKind, anyKind}, ret: tUnknown},
}

// StdlibNames lists every builtin, for suggestions and for the reference test.
func StdlibNames() []string {
	out := make([]string, 0, len(stdlib))
	for n := range stdlib {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// acceptsKind reports whether an argument of type t satisfies a parameter
// declared as want.
func acceptsKind(want Kind, t Type) bool {
	if want == anyKind || t.Kind == Unknown {
		return true
	}
	switch want {
	case Num:
		return t.numeric()
	case Int:
		return t.Kind == Int || t.Kind == ID
	case Text:
		return t.Kind == Text || t.Kind == Enum
	case List:
		return t.Kind == List
	}
	return t.Kind == want
}
