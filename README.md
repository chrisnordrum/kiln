# Kiln

A full-stack web language whose users are LLM coding agents.

React exists to serve human needs — readability, componentization, a hiring
pool. Agents inherit that surface without needing any of it and pay for it: a
screen's behavior is spread across components, hooks, context, CSS and a server
boundary, so one change costs many file reads; hook rules and effect timing are
statically unverifiable, so those bugs surface as wrong pixels in a browser the
agent never sees.

The fix is not terser syntax. Novel terse syntax is out-of-distribution for the
model and has no redundancy, so one bad token is both unrecoverable and
undetectable. Kiln optimizes two other things: **verifiability per token** and
**context required per edit**.

## A whole screen, in one file

```
route project_detail
  path /projects/:id
  guard session.user else redirect /login
  data
    project one Project where id == params.id
    tasks many Task where project == params.id order rank asc
  view
    page title=project.title
      head 2 "Tasks"
      when count(tasks) == 0
        empty "No tasks yet."
      each tasks as t
        row gap=2
          check value=t.done do=toggle_task id=t.id done=$value
          text t.title
          button "Delete" confirm="Delete task?" do=delete_task id=t.id style=quiet
      form do=add_task project=params.id
        input title text required label="New task" max=200
        submit "Add task"
```

Queries, guard, view and event bindings are all here. Nothing about this screen
lives anywhere else.

## What the checker proves

`kiln check` runs in ~33ms and proves, before anything runs:

- no reference to a field that does not exist anywhere in the app
- every `do=` names a real action, with every parameter supplied at the right type
- every internal link resolves to a real route
- every style token is real — the vocabulary is closed, there is no CSS
- **no control is wired to an action the route's guard can never satisfy**

That last one has no React equivalent. An action allowing on `session.user`,
bound from a route with no guard establishing it, is a button that renders for
anonymous visitors and always denies them.

```
routes/project_detail.kiln:14: K022: no action named toggle_tsak
    did you mean: toggle_task
    fix: declare it in actions/, or bind one that exists
```

Diagnostics are an interface, not error strings: JSON on request, root-cause
first, cascades collapsed so one missing field is reported once, capped so a
single typo cannot flood a context window, and every one carries a fix.

## Verifying UI without a browser

`kiln snap` renders a route against a test's fixtures as stable text — loops
expanded to the rows that exist, conditions resolved to the branch taken, values
substituted, each control's whole behavior on one line:

```
page title="Site"
  head 2 "Tasks"
  row gap=2
    check do=toggle_task(done=$value, id=1) value=false
    text "Ship it"
    button "Delete" do=delete_task(id=1) confirm="Delete task?" style=quiet
  form do=add_task(project="1")
    input title text required label="New task" max=200
    submit "Add task"
```

Snapshots are committed, so a UI change is a reviewable text diff and drift is a
failing check.

## Orientation in one call

`kiln map` prints the whole app — including which actions each route's view can
invoke, which is the link an agent would otherwise read a whole view to find.
`kiln where Task.done` traces a field from its declaration through every action,
view binding and test step, distinguishing definitions from uses.

## The constraint that shapes everything

`kiln docs` emits the complete language in ~1,550 tokens. A test fails the build
if it exceeds 3,000. When the cap is hit the answer is to cut a feature, never to
raise the cap — because the moment the language stops fitting in context, an
agent starts guessing, and guessing is the cost this whole design exists to
remove.

## Runtime

A page is a pure function of (params, session, data). An interaction is a named
server action → mutation → re-query → patch. No client state, no effects, no
lifecycle, no client/server boundary — so the agent never simulates time, and
those bug classes stop existing rather than being caught.

## Status

Working end to end: lexer, parser, canonical formatter, whole-program checker,
evaluator, text and HTML renderers, test runner and dev server. 152 tests.
~7,500 lines of Go and **zero dependencies** — the standard library covers it.

Not yet done: persistence (the store is in memory, a backend swap behind the
existing interface), and `kiln docs` is not yet shipped as a Claude Code skill.

Human maintainability is explicitly traded away. Not for marketing sites (no
static export) or canvas/drag UIs (a server round-trip per interaction).

```
go build -o kiln ./cmd/kiln
./kiln docs                    # the whole language
./kiln map examples/tasks      # the whole app
./kiln check examples/tasks
./kiln test examples/tasks
./kiln dev examples/tasks      # http://localhost:7777/projects/1
```
