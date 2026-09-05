# Working on Kiln

Kiln is a full-stack web language whose users are LLM coding agents. Two rules
follow from that and outrank ordinary taste:

**The reference is the spec.** `internal/docs/reference.md` is written before
the implementation and the implementation must satisfy it. It is capped at 3,000
estimated tokens so an agent can hold the whole language in context and never
guess an API. When the cap is hit, cut a feature — do not raise the cap. A test
enforces it, and another test asserts the CLI's command table and the
reference's §cli section agree in both directions.

**Diagnostics are an interface, not error strings.** Every diagnostic carries a
`Fix`, an entry in `internal/diag/codes.go` for `kiln explain`, and a `Root`
when it is one of several consequences of the same mistake. Tests assert on
these. A diagnostic that only says what is wrong has not done its job.

## Layout

```
cmd/kiln          entry point
internal/lex      source to an indentation tree of token lines
internal/parse    token lines to a typed AST (shape only, no name resolution)
internal/ast      the tree
internal/check    whole-program name and type resolution
internal/format   canonical form
internal/eval     store, expression evaluation, queries
internal/runner   requests and tests: guards, allow rules, rendering
internal/render   view tree to text (verification) and HTML (browser)
internal/server   HTTP
internal/inspect  kiln map and kiln where
internal/diag     the diagnostic contract
examples/tasks    the program the toolchain is built toward
examples/expenses an expense tracker: enums, money, dates, column totals
```

Parsing is permissive about meaning and strict about shape; every "does this
name exist" question belongs in `check`, so one pass reports structure and the
next reports references.

## Before committing

```
gofmt -w . && go vet ./... && go test ./...
go build -o kiln ./cmd/kiln
for e in examples/*; do ./kiln fmt "$e" && ./kiln check "$e" &&
  ./kiln test "$e" && ./kiln snap --check "$e"; done
```

Every example must stay canonical, check clean, pass its tests and match its
snapshots. `internal/cli/e2e_test.go` asserts all four against every directory
under `examples/`, discovered rather than listed, so adding one puts it under
the same guarantee and a change that breaks any of them fails the suite.

## Adding to the language

1. Write the reference entry first, and confirm the budget test still passes.
2. Extend the AST, then the parser, then the checker, then the formatter — in
   that order. A construct the formatter cannot emit will silently vanish from
   a file on `kiln fmt`.
3. Add a canonical-form test: formatting twice must equal formatting once.
4. Add a negative test. A checker that only proves clean programs clean is
   worth nothing; `internal/check/check_test.go` is the pattern.
5. If it introduces a diagnostic, add its code to `internal/diag/codes.go`.

## Design decisions that look like mistakes

- **Columns are not aligned and attributes sort alphabetically.** Aligning
  would mean renaming one field rewrites every line in the block, which breaks
  the next edit's unique-string match.
- **`id=` is not a view attribute.** It collided with the near-universal action
  parameter of the same name. Region identity is derived, never authored.
- **The lexer never reads a sign into a number.** Otherwise `text -5` and
  `text a -5` would differ on spacing alone.
- **The store's clock is a fixed constant.** A system clock would make every
  snapshot differ from the last.
- **`map` and `where` parse without checking.** Looking around matters most
  when a program does not yet check.
- **A path concatenates with anything.** `"/projects/" + p.id` builds a URL.
  Refusing it — because a `/`-leading literal is typed `Path` and `Path + ID`
  is not text — made a link to a record impossible to write, which is the most
  ordinary thing a web page does.
- **A field reference inside a quoted path is K026,** decided against the
  scope. `to="/projects/p.id"` renders the same dead link on every row *and*
  resolves, because `:id` is a wildcard accepting any text. The scope is what
  keeps the check precise: `p` is bound, `robots` in `/robots.txt` is not.
- **A timestamp is seeded as text.** `at` has no literal form, and every seeded
  row was taking the fixed clock, so no time-ordered query was testable.
- **The view depth cap measures the view tree, not file indentation.**
  `route > view > page` is overhead every route pays before layout begins.
  Counting it rejected a card holding a list of rows. **This limit has produced
  two false positives and no true ones: if it fires again on a reasonable
  layout, delete it rather than raise it a third time.**
- **Persistence is something the caller asks for, not something the store
  does.** A `Store` that loaded itself would make `kiln test` depend on what
  ran before it and `kiln snap` diff against yesterday. `Snapshot`/`Restore`
  are inert until `kiln dev` calls them, which is why the durability work
  changed no test, check or snapshot.
- **The data file carries no type tags.** JSON has one number type and no
  timestamp, so a round trip would turn every id into a float and every `at`
  into a string. Rather than wrap each value in `{"t":...,"v":...}`, the schema
  puts the types back on the way in — it already knows `created` is `at`, and
  it is the spec either way. The file stays something you can open and read,
  and a file that disagrees with the schema loses.
- **An aggregate reads one named column, never a whole row.** `sum(items)`
  added every numeric cell of every row — the primary key and the foreign keys
  with the rest — and `join(items, ", ")` returned the ids. Both answered a
  question nobody asked, checked clean, and rendered plausibly; a two-line
  expense report totalled $59.75 instead of $54.75. `list.field` now yields a
  column and the whole-list form is a K030, because the alternative is a wrong
  number that looks right.
- **An enum value is a quoted string everywhere, the schema default
  included.** It used to have two spellings and no writable one: a bare `todo`
  in a default resolved as a name and reported K021, and `= "todo"` failed a
  membership test that compared the quoted spelling and then suggested the
  bare one. The two diagnostics pointed at each other. `set` and `where` had
  always taken the quoted form, so that is the one that stayed.
- **A fixture is coerced in `Runner.Seed`, not by its caller.** When only the
  test runner coerced, an `at` column held a time under `kiln test` and a raw
  string in the browser, so every date function rendered correctly in the
  snapshot and blank on the page. One seeding path, one set of types.
- **K040 reports once per route and session field,** naming every action
  affected. Keyed per action, one missing guard produced one diagnostic per
  action that tripped over it — the cascade the machinery exists to prevent.

## Where things stand

Working end to end: lexer, parser, canonical formatter, whole-program checker,
evaluator, text and HTML renderers, test runner, dev server, and the three
orientation commands. 187 tests, zero dependencies, reference at 58% of its
3,000-token budget.

A second app was written against the language as a check on the first —
`examples/expenses`, an expense tracker, chosen to exercise what
`examples/tasks` never touched. It found four defects in one sitting: `sum`
totalling every column, enum defaults being unwritable, dev-server seeds
skipping coercion so dates rendered blank in the browser only, and one bad
parameter reporting a diagnostic per enum value. All four are fixed and pinned
by tests, and both examples now carry an enum default and a column total so
they stay fixed. It cost about an hour, which is the argument for a third app.

The store persists on request: `Store.Snapshot` and `Store.Restore` move it to
and from plain JSON, and `kiln dev --data <file>` loads at startup and saves
after every action. `kiln test` and `kiln snap` are untouched by it and still
start empty every time.

The language was tested by running four cold Claude Code sessions against it,
each given only `kiln docs` and a feature request. `eval/` holds the method,
the pre-registered scoring, the transcripts' outcomes and the resulting
backlog; `eval/results/` holds what each session actually wrote. Read
`eval/RESULTS.md` before changing anything — it is the only evidence that
exists for whether this design works, and every item it found is now fixed.

The single most important finding: `kiln check` was green on a page where every
link 404'd. Four days of building never hit it because the example app had one
data route and no navigation between records. **The example is the spec in
practice.** Anything the example does not exercise is unverified, so a language
change lands in an example or it is not done.

### Next, in order

1. **A second eval round.** Re-run the four tasks against the fixed language.
   The rubric is fixed — it now counts *unguided* cycles, where the session did
   something other than apply the diagnostic's fix line, so a checker doing its
   job no longer reads as a session struggling. `eval/record.py` pulls the
   outcome, the cycles, the files opened and any isolation breach out of the
   session transcript, so round 1's habit of losing evidence to memory cannot
   repeat. Read `eval/RESULTS.md` § *Evidence recovered* first: reading the
   round-1 transcripts back found a breach nobody had noticed.
2. **The `kiln docs` skill,** so a session loads the language without being
   told to. Worth more now than before, since the hole it would have taught
   around is closed.

Isolation matters when running the eval: a session that can reach this repo
will infer the language from the compiler instead of from the reference, which
is what happened in the task-3 run. `eval/setup.sh` builds the clean room and
refuses to overwrite one without `REPLACE=1`.
