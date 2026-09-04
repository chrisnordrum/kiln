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
