package eval

import (
	"testing"
	"time"

	"github.com/chrisnordrum/kiln/internal/ast"
	"github.com/chrisnordrum/kiln/internal/diag"
	"github.com/chrisnordrum/kiln/internal/parse"
)

const schema = `app t
  title "T"
  session user ref User

table User
  id id
  name text max 80

table Task
  id id
  owner ref User on delete cascade
  title text max 200
  done bool = false
  rank int
  created at now
`

func program(t *testing.T, extra string) *ast.Program {
	t.Helper()
	var d diag.List
	p := parse.Program([]parse.File{{Path: "s.kiln", Src: schema + extra}}, &d)
	if !d.Empty() {
		shown, _ := d.Resolved()
		t.Fatalf("fixture did not parse: %+v", shown)
	}
	return p
}

// evalExpr parses a bare expression by wrapping it in an allow rule.
func evalExpr(t *testing.T, e *Evaluator, env *Env, src string) Value {
	t.Helper()
	var d diag.List
	p := parse.Program([]parse.File{{Path: "e.kiln",
		Src: "action probe\n  allow " + src + "\n  do\n    refresh\n"}}, &d)
	if !d.Empty() {
		shown, _ := d.Resolved()
		t.Fatalf("%q did not parse: %+v", src, shown)
	}
	return e.Eval(p.Actions[0].Allow, env)
}

func TestArithmeticKeepsWholeNumbersWhole(t *testing.T) {
	e := New(program(t, ""))
	env := &Env{Vars: map[string]Value{}}
	cases := []struct {
		src  string
		want Value
	}{
		{"2 + 3", int64(5)},
		{"2 * 3", int64(6)},
		{"7 - 10", int64(-3)},
		{"7 % 3", int64(1)},
		{"7 / 2", 3.5},
		{"2.5 + 1", 3.5},
		{"1 < 2", true},
		{"2 <= 2", true},
		{`"a" + "b"`, "ab"},
		{"not true", false},
		{"true and false", false},
		{"false or true", true},
		{"if 1 < 2 then 10 else 20", int64(10)},
		{"-3 + 1", int64(-2)},
	}
	for _, c := range cases {
		if got := evalExpr(t, e, env, c.src); got != c.want {
			t.Errorf("%s = %v (%T), want %v (%T)", c.src, got, got, c.want, c.want)
		}
	}
}

// Division by zero yields nothing rather than crashing the request.
func TestDivisionByZeroIsNil(t *testing.T) {
	e := New(program(t, ""))
	env := &Env{Vars: map[string]Value{}}
	for _, src := range []string{"1 / 0", "1 % 0"} {
		if got := evalExpr(t, e, env, src); got != nil {
			t.Errorf("%s = %v, want nil", src, got)
		}
	}
}

func TestStdlib(t *testing.T) {
	e := New(program(t, ""))
	e.S.Now = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	env := &Env{Vars: map[string]Value{}}
	cases := []struct {
		src  string
		want Value
	}{
		{`upper("abc")`, "ABC"},
		{`lower("ABC")`, "abc"},
		{`title("hello there")`, "Hello There"},
		{`trim("  x  ")`, "x"},
		{`slug("Hello, World!")`, "hello-world"},
		{`truncate("abcdef", 4)`, "abc…"},
		{`truncate("abc", 10)`, "abc"},
		{`len("abcd")`, int64(4)},
		{`has("abcdef", "cd")`, true},
		{`replace("a-b-c", "-", "+")`, "a+b+c"},
		{`abs(-3)`, int64(3)},
		{`round(2.6)`, int64(3)},
		{`floor(2.9)`, int64(2)},
		{`ceil(2.1)`, int64(3)},
		{`min(3, 5)`, int64(3)},
		{`max(3, 5)`, int64(5)},
		{`money(1234.5)`, "$1,234.50"},
		{`money(-12)`, "-$12.00"},
		{`pct(42.4)`, "42%"},
		{`plural(1, "task", "tasks")`, "task"},
		{`plural(2, "task", "tasks")`, "tasks"},
		{`coalesce(null, "fallback")`, "fallback"},
		{`coalesce("set", "fallback")`, "set"},
		{`date(now(), "Y-m-d")`, "2026-03-10"},
		{`ago(plus_days(now(), -3))`, "3 days ago"},
		{`ago(plus_days(now(), 2))`, "in 2 days"},
		{`days_between(now(), plus_days(now(), 5))`, int64(5)},
	}
	for _, c := range cases {
		if got := evalExpr(t, e, env, c.src); got != c.want {
			t.Errorf("%s = %v (%T), want %v (%T)", c.src, got, got, c.want, c.want)
		}
	}
}

func TestInsertAppliesDefaults(t *testing.T) {
	p := program(t, "")
	e := New(p)
	id, err := e.S.Insert("Task", Row{"title": "Ship it", "rank": int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	row, ok := e.S.Get("Task", id)
	if !ok {
		t.Fatal("inserted row is not retrievable")
	}
	if row["done"] != false {
		t.Errorf("done = %v, want the declared default false", row["done"])
	}
	if _, ok := row["created"].(time.Time); !ok {
		t.Errorf("created = %v, want the fixed clock", row["created"])
	}
	if row["id"] != int64(1) {
		t.Errorf("id = %v, want a generated 1", row["id"])
	}
}

// An explicit id must not collide with a later generated one.
func TestExplicitIDAdvancesTheCounter(t *testing.T) {
	e := New(program(t, ""))
	if _, err := e.S.Insert("Task", Row{"id": int64(7), "title": "a", "rank": int64(0)}); err != nil {
		t.Fatal(err)
	}
	id, err := e.S.Insert("Task", Row{"title": "b", "rank": int64(0)})
	if err != nil {
		t.Fatal(err)
	}
	if id == int64(7) {
		t.Error("a generated id collided with an explicit one")
	}
}

func TestCascadeDelete(t *testing.T) {
	e := New(program(t, ""))
	uid, _ := e.S.Insert("User", Row{"name": "A"})
	e.S.Insert("Task", Row{"owner": uid, "title": "x", "rank": int64(0)})
	e.S.Insert("Task", Row{"owner": uid, "title": "y", "rank": int64(1)})
	other, _ := e.S.Insert("User", Row{"name": "B"})
	e.S.Insert("Task", Row{"owner": other, "title": "z", "rank": int64(0)})

	if err := e.S.Delete("User", uid); err != nil {
		t.Fatal(err)
	}
	left := e.S.All("Task")
	if len(left) != 1 {
		t.Fatalf("want 1 task left after cascade, got %d", len(left))
	}
	if left[0]["title"] != "z" {
		t.Errorf("the wrong task survived: %v", left[0]["title"])
	}
}

func TestFieldWalkFollowsReferences(t *testing.T) {
	p := program(t, "")
	e := New(p)
	uid, _ := e.S.Insert("User", Row{"name": "Ada"})
	tid, _ := e.S.Insert("Task", Row{"owner": uid, "title": "x", "rank": int64(0)})

	env := &Env{Vars: map[string]Value{}, Session: map[string]Value{"user": uid}}
	if got := evalExpr(t, e, env, "Task["+itoa(tid)+"].owner.name"); got != "Ada" {
		t.Errorf("walk through a ref = %v, want Ada", got)
	}
	if got := evalExpr(t, e, env, "session.user == Task["+itoa(tid)+"].owner"); got != true {
		t.Errorf("comparing a session ref with a walked ref = %v, want true", got)
	}
}

func itoa(v Value) string { return Text(v) }

func TestQueryFilterOrderLimit(t *testing.T) {
	p := program(t, "")
	e := New(p)
	uid, _ := e.S.Insert("User", Row{"name": "A"})
	for i, title := range []string{"c", "a", "b", "d"} {
		e.S.Insert("Task", Row{"owner": uid, "title": title, "rank": int64(i)})
	}

	q := findQuery(t, `
route r
  path /r
  data
    tasks many Task where rank < 3 order rank desc limit 2
  view
    page
`)
	got := e.Query(q, &Env{Vars: map[string]Value{}, Params: map[string]Value{}})
	list, ok := got.(List)
	if !ok {
		t.Fatalf("want a List, got %T", got)
	}
	if len(list.Rows) != 2 {
		t.Fatalf("limit 2 yielded %d rows", len(list.Rows))
	}
	if list.Rows[0]["title"] != "b" || list.Rows[1]["title"] != "a" {
		t.Errorf("order rank desc gave %v, %v", list.Rows[0]["title"], list.Rows[1]["title"])
	}
}

// findQuery parses a route and returns its first data key.
func findQuery(t *testing.T, src string) *ast.Query {
	t.Helper()
	var d diag.List
	p := parse.Program([]parse.File{{Path: "q.kiln", Src: schema + src}}, &d)
	if !d.Empty() {
		shown, _ := d.Resolved()
		t.Fatalf("fixture did not parse: %+v", shown)
	}
	return p.Routes[0].Data[0]
}

func TestOneQueryYieldsARecordOrNil(t *testing.T) {
	p := program(t, "")
	e := New(p)
	q := findQuery(t, `
route r
  path /r/:id
  data
    task one Task where id == params.id
  view
    page
`)
	env := &Env{Vars: map[string]Value{}, Params: map[string]Value{"id": "1"}}
	if got := e.Query(q, env); got != nil {
		t.Errorf("an empty table should yield nil, got %v", got)
	}
	e.S.Insert("Task", Row{"title": "x", "rank": int64(0)})
	rec, ok := e.Query(q, env).(Record)
	if !ok {
		t.Fatalf("want a Record once the row exists")
	}
	if rec.Row["title"] != "x" {
		t.Errorf("got %v", rec.Row)
	}
}

// sum used to add every numeric cell of every row, the primary key and the
// foreign keys included, so a two-line expense report totalled $59.75 instead
// of $54.75. join had the same shape: it returned the ids, never the column
// anyone wanted.
func TestAggregatesReadOnlyTheirColumn(t *testing.T) {
	e := New(program(t, ""))
	if _, err := e.S.Insert("User", Row{"name": "Ada"}); err != nil {
		t.Fatal(err)
	}
	for _, task := range []struct {
		title string
		rank  int64
	}{{"first", 10}, {"second", 20}} {
		if _, err := e.S.Insert("Task", Row{
			"owner": int64(1), "title": task.title, "rank": task.rank,
		}); err != nil {
			t.Fatal(err)
		}
	}
	list := List{Table: "Task", Rows: e.S.All("Task")}

	// ranks are 10 and 20; ids are 1 and 2, and must stay out of it.
	if got := e.call("sum", []Value{List{Table: "Task", Rows: list.Rows, Field: "rank"}}); got != int64(30) {
		t.Errorf("sum of the rank column = %v, want 30", got)
	}
	if got := e.call("join", []Value{List{Table: "Task", Rows: list.Rows, Field: "title"}, ", "}); got != "first, second" {
		t.Errorf("join of the title column = %v, want \"first, second\"", got)
	}
	// count is about the rows themselves, so it still takes the whole list.
	if got := e.call("count", []Value{list}); got != int64(2) {
		t.Errorf("count = %v, want 2", got)
	}
}
