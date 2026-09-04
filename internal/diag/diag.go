// Package diag is the checker's output contract.
//
// The diagnostics an agent reads are an interface, not prose. They are JSON,
// root-cause first, deduped so one mistake is reported once rather than once
// per use, capped so a single typo cannot flood a context window, and each
// carries a fix rather than only a diagnosis.
package diag

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Max is how many diagnostics are rendered before the rest are summarized.
// A cascade of a hundred errors costs context and teaches an agent nothing the
// first twenty did not.
const Max = 20

// Diag is one problem with one fix.
type Diag struct {
	Code string   `json:"code"`
	File string   `json:"file"`
	Line int      `json:"line"`
	Col  int      `json:"col,omitempty"`
	Msg  string   `json:"msg"`
	Near []string `json:"near,omitempty"` // spelling suggestions
	Fix  string   `json:"fix,omitempty"`

	// Root names the thing that is wrong (a table, a field, an action). When
	// several diagnostics share a Root, only the first survives: the rest are
	// consequences of the same mistake.
	Root string `json:"-"`
}

// Explain returns the long-form help for a code, for `kiln explain`.
func (d Diag) Explain() string { return explain[d.Code] }

// List accumulates diagnostics.
type List struct {
	items []Diag
}

// Add records a diagnostic.
func (l *List) Add(d Diag) { l.items = append(l.items, d) }

// Addf records a diagnostic with a formatted message.
func (l *List) Addf(code, file string, line int, format string, a ...any) {
	l.Add(Diag{Code: code, File: file, Line: line, Msg: fmt.Sprintf(format, a...)})
}

// Len reports how many diagnostics were recorded, before deduping.
func (l *List) Len() int { return len(l.items) }

// Empty reports whether the program checked clean.
func (l *List) Empty() bool { return len(l.items) == 0 }

// Resolved returns the diagnostics worth showing: deduped, cascades collapsed
// to their root cause, and ordered so the earliest failure in the pipeline
// comes first. Fixing the first one often resolves several below it.
func (l *List) Resolved() (shown []Diag, suppressed int) {
	seen := map[string]bool{}
	roots := map[string]bool{}
	var keep []Diag

	ordered := make([]Diag, len(l.items))
	copy(ordered, l.items)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if class(a.Code) != class(b.Code) {
			// K00 lexical, K01 structural, K02 references, K03 types,
			// K04 permissions — the order the checker discovers them, so the
			// first diagnostic is the one furthest upstream.
			return a.Code < b.Code
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Msg < b.Msg
	})

	for _, d := range ordered {
		key := fmt.Sprintf("%s|%s|%d|%s", d.Code, d.File, d.Line, d.Msg)
		if seen[key] {
			suppressed++
			continue
		}
		seen[key] = true
		if d.Root != "" {
			if roots[d.Root] {
				suppressed++
				continue
			}
			roots[d.Root] = true
		}
		keep = append(keep, d)
	}
	if len(keep) > Max {
		suppressed += len(keep) - Max
		keep = keep[:Max]
	}
	return keep, suppressed
}

// Render writes diagnostics for an agent to read. JSON is the machine path;
// the text path stays parseable as file:line: code: msg.
func (l *List) Render(w io.Writer, asJSON bool) error {
	shown, suppressed := l.Resolved()
	if asJSON {
		payload := struct {
			OK         bool   `json:"ok"`
			Diags      []Diag `json:"diags"`
			Suppressed int    `json:"suppressed"`
		}{OK: len(shown) == 0, Diags: shown, Suppressed: suppressed}
		if payload.Diags == nil {
			payload.Diags = []Diag{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}
	if len(shown) == 0 {
		_, err := fmt.Fprintln(w, "ok")
		return err
	}
	var b strings.Builder
	for _, d := range shown {
		fmt.Fprintf(&b, "%s:%d: %s: %s\n", d.File, d.Line, d.Code, d.Msg)
		if len(d.Near) > 0 {
			fmt.Fprintf(&b, "    did you mean: %s\n", strings.Join(d.Near, ", "))
		}
		if d.Fix != "" {
			fmt.Fprintf(&b, "    fix: %s\n", d.Fix)
		}
	}
	if suppressed > 0 {
		fmt.Fprintf(&b, "\n%d more suppressed (same root cause or past the cap of %d)\n", suppressed, Max)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// class is the code's family — the first three characters, which group codes by
// the checking phase that emits them.
func class(code string) string {
	if len(code) < 3 {
		return code
	}
	return code[:3]
}
