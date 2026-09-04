package diag

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCascadesCollapseToTheirRoot(t *testing.T) {
	var l List
	// One missing field, used in five places. The agent should be told once.
	for i := range 5 {
		l.Add(Diag{Code: "K021", File: "r.kiln", Line: 10 + i,
			Msg: "Task has no field 'complete'", Root: "Task.complete"})
	}
	shown, suppressed := l.Resolved()
	if len(shown) != 1 {
		t.Errorf("want 1 diagnostic for 1 root cause, got %d", len(shown))
	}
	if suppressed != 4 {
		t.Errorf("want 4 suppressed, got %d", suppressed)
	}
}

func TestEarlierPhasesSortFirst(t *testing.T) {
	var l List
	l.Add(Diag{Code: "K021", File: "b.kiln", Line: 2, Msg: "unknown field"})
	l.Add(Diag{Code: "K002", File: "z.kiln", Line: 9, Msg: "bad indent"})
	shown, _ := l.Resolved()
	if shown[0].Code != "K002" {
		t.Errorf("syntax errors should come before reference errors, got %s first", shown[0].Code)
	}
}

func TestCapWithSuppressedCount(t *testing.T) {
	var l List
	for i := range Max + 7 {
		l.Addf("K021", "r.kiln", i, "field %d missing", i)
	}
	shown, suppressed := l.Resolved()
	if len(shown) != Max {
		t.Errorf("want %d shown, got %d", Max, len(shown))
	}
	if suppressed != 7 {
		t.Errorf("want 7 suppressed, got %d", suppressed)
	}
}

func TestJSONShape(t *testing.T) {
	var l List
	l.Add(Diag{Code: "K021", File: "r.kiln", Line: 18, Msg: "Task has no field 'complete'",
		Near: []string{"done"}, Fix: "set t.done"})
	var buf bytes.Buffer
	if err := l.Render(&buf, true); err != nil {
		t.Fatal(err)
	}
	var got struct {
		OK    bool `json:"ok"`
		Diags []struct {
			Code, File, Msg, Fix string
			Line                 int
			Near                 []string
		}
		Suppressed int
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if got.OK {
		t.Error("ok should be false when there are diagnostics")
	}
	d := got.Diags[0]
	if d.Code != "K021" || d.Line != 18 || d.Fix == "" || len(d.Near) != 1 {
		t.Errorf("lost a field in the round trip: %+v", d)
	}
}

func TestCleanProgramIsOK(t *testing.T) {
	var l List
	var buf bytes.Buffer
	if err := l.Render(&buf, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"ok": true`) {
		t.Errorf("want ok:true, got %s", buf.String())
	}
	buf.Reset()
	if err := l.Render(&buf, false); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(buf.String()) != "ok" {
		t.Errorf("want plain ok, got %q", buf.String())
	}
}

func TestTextFormatIsParseable(t *testing.T) {
	var l List
	l.Add(Diag{Code: "K021", File: "routes/r.kiln", Line: 18, Msg: "no field", Fix: "use done"})
	var buf bytes.Buffer
	if err := l.Render(&buf, false); err != nil {
		t.Fatal(err)
	}
	first := strings.SplitN(buf.String(), "\n", 2)[0]
	if !strings.HasPrefix(first, "routes/r.kiln:18: K021: ") {
		t.Errorf("want file:line: code: msg, got %q", first)
	}
}

func TestSuggest(t *testing.T) {
	fields := []string{"done", "title", "rank", "project"}
	if got := Suggest("complete", fields); len(got) != 0 {
		t.Errorf("a genuinely different word should not be guessed at, got %v", got)
	}
	got := Suggest("titel", fields)
	if len(got) == 0 || got[0] != "title" {
		t.Errorf("want title suggested for titel, got %v", got)
	}
	if got := Suggest("dome", fields); len(got) == 0 || got[0] != "done" {
		t.Errorf("want done suggested for dome, got %v", got)
	}
}

// Every code the checker can emit needs help behind `kiln explain`.
func TestEveryCodeHasExplanation(t *testing.T) {
	for _, c := range Codes() {
		if text, ok := Lookup(c); !ok || strings.TrimSpace(text) == "" {
			t.Errorf("code %s has no explanation", c)
		}
	}
}
