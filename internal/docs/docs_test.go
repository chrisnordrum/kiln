package docs

import (
	"strings"
	"testing"
)

// Design rule 1: the whole language fits in an agent's context. If this fails,
// the fix is to cut a feature, not to raise the budget.
func TestReferenceWithinTokenBudget(t *testing.T) {
	got := EstimateTokens(Full())
	if got > TokenBudget {
		t.Fatalf("reference is ~%d tokens, budget is %d: cut a feature", got, TokenBudget)
	}
	t.Logf("reference ~%d tokens, %d%% of budget", got, got*100/TokenBudget)
}

func TestSectionsResolve(t *testing.T) {
	names := Sections()
	if len(names) == 0 {
		t.Fatal("reference declares no sections")
	}
	for _, name := range names {
		body, err := Section(name)
		if err != nil {
			t.Errorf("Section(%q): %v", name, err)
			continue
		}
		if len(strings.TrimSpace(body)) <= len(sectionMark+name) {
			t.Errorf("Section(%q) is empty", name)
		}
	}
}

func TestUnknownSectionListsChoices(t *testing.T) {
	_, err := Section("nope")
	if err == nil {
		t.Fatal("want an error for an unknown section")
	}
	if !strings.Contains(err.Error(), "view") {
		t.Errorf("error should list real sections, got: %v", err)
	}
}
