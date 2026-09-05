// Package eval runs a checked Kiln program: it holds the data, executes
// queries, evaluates expressions and applies actions.
//
// The store is in memory. The evaluator is the hard part and the part the
// language's semantics live in; persistence is a backend swap behind the same
// interface, so it is deliberately not the thing built first.
package eval

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Value is a runtime value: nil, bool, int64, float64, string, time.Time,
// Record or List.
type Value any

// Row is one stored record.
type Row map[string]Value

// Record is a row together with the table it came from, so a reference can be
// followed without guessing.
type Record struct {
	Table string
	Row   Row
}

// List is an ordered set of rows from one table.
//
// Field, when set, means the list has been narrowed to one column: `sum` and
// `join` then read that column instead of guessing which one was meant. They
// used to guess — sum added every numeric cell in every row, the primary key
// included, and join returned the ids — and both produced a plausible answer
// to a question nobody asked.
type List struct {
	Table string
	Rows  []Row
	Field string
}

// Column reports the values a projected list holds.
func (l List) Column() []Value {
	out := make([]Value, 0, len(l.Rows))
	for _, r := range l.Rows {
		out = append(out, r[l.Field])
	}
	return out
}

// Text renders a value the way it appears on a page.
func Text(v Value) string {
	switch x := v.(type) {
	case nil:
		return ""
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		return x
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	case Record:
		return fmt.Sprintf("%s[%v]", x.Table, x.Row["id"])
	case List:
		if x.Field != "" {
			parts := make([]string, 0, len(x.Rows))
			for _, v := range x.Column() {
				parts = append(parts, Text(v))
			}
			return strings.Join(parts, ", ")
		}
		return fmt.Sprintf("%d %s", len(x.Rows), x.Table)
	}
	return fmt.Sprint(v)
}

// truthy reports whether a value counts as true in a condition.
func truthy(v Value) bool {
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
	case List:
		return len(x.Rows) > 0
	}
	return true
}

// number coerces to float64 for arithmetic.
func number(v Value) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}

// isInt reports whether a value is a whole number, so arithmetic on two
// integers stays integral.
func isInt(v Value) bool {
	_, ok := v.(int64)
	return ok
}

// equal compares two values, coercing across the numeric kinds so an id from a
// URL compares with an id from the store.
func equal(a, b Value) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if ar, ok := a.(Record); ok {
		a = ar.Row["id"]
	}
	if br, ok := b.(Record); ok {
		b = br.Row["id"]
	}
	if at, ok := a.(time.Time); ok {
		bt, ok := b.(time.Time)
		return ok && at.Equal(bt)
	}
	an, aok := number(a)
	bn, bok := number(b)
	if aok && bok {
		// Two strings that both parse as numbers still compare as text, so
		// "01" and "1" stay distinct identifiers.
		_, aStr := a.(string)
		_, bStr := b.(string)
		if !(aStr && bStr) {
			return an == bn
		}
	}
	return Text(a) == Text(b)
}

// compare orders two values, reporting -1, 0 or 1 and whether they are
// orderable at all.
func compare(a, b Value) (int, bool) {
	if at, ok := a.(time.Time); ok {
		if bt, ok := b.(time.Time); ok {
			switch {
			case at.Before(bt):
				return -1, true
			case at.After(bt):
				return 1, true
			}
			return 0, true
		}
	}
	an, aok := number(a)
	bn, bok := number(b)
	if aok && bok {
		switch {
		case an < bn:
			return -1, true
		case an > bn:
			return 1, true
		}
		return 0, true
	}
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		return strings.Compare(as, bs), true
	}
	return 0, false
}

// sortRows orders rows by a sequence of keys.
func sortRows(rows []Row, keys []SortKey) {
	sort.SliceStable(rows, func(i, j int) bool {
		for _, k := range keys {
			c, ok := compare(rows[i][k.Field], rows[j][k.Field])
			if !ok || c == 0 {
				continue
			}
			if k.Desc {
				return c > 0
			}
			return c < 0
		}
		return false
	})
}

// SortKey is one ordering term.
type SortKey struct {
	Field string
	Desc  bool
}
