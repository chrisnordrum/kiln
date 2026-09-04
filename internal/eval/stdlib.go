package eval

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
)

// call runs one stdlib function. The checker has already verified the name and
// the argument count, so this handles shape rather than validating it.
func (e *Evaluator) call(name string, a []Value) Value {
	arg := func(i int) Value {
		if i < len(a) {
			return a[i]
		}
		return nil
	}
	str := func(i int) string { return Text(arg(i)) }
	num := func(i int) float64 {
		f, _ := number(arg(i))
		return f
	}
	whole := func(i int) int {
		return int(num(i))
	}

	switch name {
	// text
	case "upper":
		return strings.ToUpper(str(0))
	case "lower":
		return strings.ToLower(str(0))
	case "title":
		return titleCase(str(0))
	case "trim":
		return strings.TrimSpace(str(0))
	case "slug":
		return slug(str(0))
	case "truncate":
		return truncate(str(0), whole(1))
	case "len":
		return int64(len([]rune(str(0))))
	case "join":
		return joinList(arg(0), str(1))
	case "has":
		return strings.Contains(str(0), str(1))
	case "replace":
		return strings.ReplaceAll(str(0), str(1), str(2))

	// numbers
	case "abs":
		return keepKind(arg(0), math.Abs(num(0)))
	case "round":
		return int64(math.Round(num(0)))
	case "floor":
		return int64(math.Floor(num(0)))
	case "ceil":
		return int64(math.Ceil(num(0)))
	case "min":
		return pickNumber(arg(0), arg(1), true)
	case "max":
		return pickNumber(arg(0), arg(1), false)
	case "sum":
		return sumList(arg(0))
	case "count":
		if l, ok := arg(0).(List); ok {
			return int64(len(l.Rows))
		}
		return int64(0)
	case "money":
		return money(num(0))
	case "pct":
		return fmt.Sprintf("%d%%", int64(math.Round(num(0))))

	// dates
	case "now":
		return e.S.Now
	case "ago":
		return ago(e.S.Now, arg(0))
	case "date":
		return formatDate(arg(0), str(1))
	case "days_between":
		return daysBetween(arg(0), arg(1))
	case "plus_days":
		if t, ok := arg(0).(time.Time); ok {
			return t.AddDate(0, 0, whole(1))
		}
		return nil

	// general
	case "coalesce", "default":
		if arg(0) != nil && Text(arg(0)) != "" {
			return arg(0)
		}
		return arg(1)
	case "plural":
		if whole(0) == 1 {
			return str(1)
		}
		return str(2)
	}
	return nil
}

// keepKind returns a whole number when the input was one, so abs(-2) is 2 and
// not 2.0.
func keepKind(in Value, f float64) Value {
	if isInt(in) {
		return int64(f)
	}
	return f
}

func pickNumber(a, b Value, wantMin bool) Value {
	af, aok := number(a)
	bf, bok := number(b)
	if !aok {
		return b
	}
	if !bok {
		return a
	}
	if (af < bf) == wantMin {
		return a
	}
	return b
}

func titleCase(s string) string {
	prev := ' '
	return strings.Map(func(r rune) rune {
		out := r
		if !unicode.IsLetter(prev) {
			out = unicode.ToUpper(r)
		} else {
			out = unicode.ToLower(r)
		}
		prev = r
		return out
	}, s)
}

func slug(s string) string {
	var b strings.Builder
	lastDash := true // suppress a leading dash
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if n <= 0 || len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func joinList(v Value, sep string) string {
	l, ok := v.(List)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(l.Rows))
	for _, r := range l.Rows {
		parts = append(parts, Text(r["id"]))
	}
	return strings.Join(parts, sep)
}

func sumList(v Value) Value {
	l, ok := v.(List)
	if !ok {
		return int64(0)
	}
	var total float64
	whole := true
	for _, r := range l.Rows {
		for _, cell := range r {
			if f, ok := number(cell); ok {
				total += f
				if !isInt(cell) {
					whole = false
				}
			}
		}
	}
	if whole {
		return int64(total)
	}
	return total
}

func money(f float64) string {
	neg := f < 0
	if neg {
		f = -f
	}
	s := fmt.Sprintf("%.2f", f)
	dot := strings.IndexByte(s, '.')
	whole, frac := s[:dot], s[dot:]
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := "$" + b.String() + frac
	if neg {
		return "-" + out
	}
	return out
}

// ago renders a timestamp relative to the store's fixed clock, so snapshots
// stay reproducible.
func ago(now time.Time, v Value) string {
	t, ok := v.(time.Time)
	if !ok {
		return ""
	}
	d := now.Sub(t)
	future := d < 0
	if future {
		d = -d
	}
	var unit string
	var n int
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		n, unit = int(d/time.Minute), "minute"
	case d < 24*time.Hour:
		n, unit = int(d/time.Hour), "hour"
	case d < 30*24*time.Hour:
		n, unit = int(d/(24*time.Hour)), "day"
	case d < 365*24*time.Hour:
		n, unit = int(d/(30*24*time.Hour)), "month"
	default:
		n, unit = int(d/(365*24*time.Hour)), "year"
	}
	if n != 1 {
		unit += "s"
	}
	if future {
		return fmt.Sprintf("in %d %s", n, unit)
	}
	return fmt.Sprintf("%d %s ago", n, unit)
}

// formatDate renders a timestamp with a small, explicit format vocabulary:
// Y year, m month, d day, H hour, M minute, S second.
func formatDate(v Value, format string) string {
	t, ok := v.(time.Time)
	if !ok {
		return ""
	}
	t = t.UTC()
	var b strings.Builder
	for _, r := range format {
		switch r {
		case 'Y':
			fmt.Fprintf(&b, "%04d", t.Year())
		case 'm':
			fmt.Fprintf(&b, "%02d", int(t.Month()))
		case 'd':
			fmt.Fprintf(&b, "%02d", t.Day())
		case 'H':
			fmt.Fprintf(&b, "%02d", t.Hour())
		case 'M':
			fmt.Fprintf(&b, "%02d", t.Minute())
		case 'S':
			fmt.Fprintf(&b, "%02d", t.Second())
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func daysBetween(a, b Value) Value {
	at, aok := a.(time.Time)
	bt, bok := b.(time.Time)
	if !aok || !bok {
		return int64(0)
	}
	return int64(math.Round(bt.Sub(at).Hours() / 24))
}
