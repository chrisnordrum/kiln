// Package docs serves the Kiln language reference.
//
// The reference is the spec: it is written before the implementation and the
// implementation must satisfy it. Rule 1 of the design caps it at 3000
// estimated tokens so an agent can hold the entire language in context.
package docs

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed reference.md
var reference string

// TokenBudget is the hard cap from design rule 1.
const TokenBudget = 3000

// sectionMark introduces an addressable section, as in "## §view".
const sectionMark = "## §"

// Full returns the complete language reference.
func Full() string { return reference }

// Section returns one named section, so an agent can pull just the part it
// needs instead of the whole reference.
func Section(name string) (string, error) {
	want := sectionMark + name
	lines := strings.Split(reference, "\n")
	start := -1
	for i, ln := range lines {
		if ln == want {
			start = i
			break
		}
	}
	if start < 0 {
		return "", fmt.Errorf("no section %q; have: %s", name, strings.Join(Sections(), " "))
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], sectionMark) {
			end = i
			break
		}
	}
	return strings.TrimRight(strings.Join(lines[start:end], "\n"), "\n"), nil
}

// Sections lists every addressable section name.
func Sections() []string {
	var out []string
	for _, ln := range strings.Split(reference, "\n") {
		if strings.HasPrefix(ln, sectionMark) {
			out = append(out, strings.TrimPrefix(ln, sectionMark))
		}
	}
	sort.Strings(out)
	return out
}

// EstimateTokens approximates tokens for budget enforcement. There is no
// tokenizer in the binary, so this uses chars/3.5 — deliberately conservative
// for the code-dense text of the reference, where real tokenizers land nearer
// chars/4.
func EstimateTokens(s string) int { return len(s) * 10 / 35 }
