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
```

Parsing is permissive about meaning and strict about shape; every "does this
name exist" question belongs in `check`, so one pass reports structure and the
next reports references.

## Before committing

```
gofmt -w . && go vet ./... && go test ./...
go build -o kiln ./cmd/kiln && ./kiln fmt examples/tasks && ./kiln check examples/tasks
./kiln test examples/tasks && ./kiln snap --check examples/tasks
```

The example must stay canonical, check clean, pass its tests and match its
snapshots. `internal/cli/e2e_test.go` asserts all four, so a change that breaks
any of them fails the suite.

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
- **K040 reports once per route and session field,** naming every action
  affected. Keyed per action, one missing guard produced one diagnostic per
  action that tripped over it — the cascade the machinery exists to prevent.

## Where things stand

Working end to end: lexer, parser, canonical formatter, whole-program checker,
evaluator, text and HTML renderers, test runner, dev server, and the three
orientation commands. 164 tests, zero dependencies, reference at 56% of its
3,000-token budget.

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
change lands in `examples/tasks` or it is not done.

### Next, in order

1. **A second eval round.** Re-run the four tasks against the fixed language.
   Fix the rubric first: it scored task 1 Weak for three check-fix cycles, but
   all three were the checker teaching the language to a session that then
   produced correct, tested code in under two minutes. "The checker caught it"
   and "the session struggled" are different outcomes and the bar cannot tell
   them apart. Also record which files each session opens — that is the
   evidence for "one screen, one read" and only one run captured it.
2. **Persistence.** The store is in memory behind `eval.Store`; nothing else
   depends on how it persists. No test, check or snapshot needs it, which is
   why it has stayed deferred.
3. **The `kiln docs` skill,** so a session loads the language without being
   told to. Worth more now than before, since the hole it would have taught
   around is closed.

Isolation matters when running the eval: a session that can reach this repo
will infer the language from the compiler instead of from the reference, which
is what happened in the task-3 run. `eval/setup.sh` builds the clean room and
refuses to overwrite one without `REPLACE=1`.
