# Kiln

[![ci](https://github.com/chrisnordrum/kiln/actions/workflows/ci.yml/badge.svg)](https://github.com/chrisnordrum/kiln/actions/workflows/ci.yml)

A full-stack web language whose users are LLM coding agents.

Modern frontend frameworks exist to serve human needs — readability,
componentization, a hiring pool. Agents inherit that surface without needing any
of it and pay for it: a screen's behavior is spread across components, hooks,
state, CSS and a server boundary, so one change costs many file reads; effect
timing and render lifecycles are statically unverifiable, so those bugs surface
as wrong pixels in a browser the agent never sees.

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
      text plural(count(tasks), "task", "tasks")
      text sum(tasks.estimate)
      when count(tasks) == 0
        empty "No tasks yet."
      each tasks as t
        col gap=0
          row gap=2
            check do=toggle_task done=$value id=t.id value=t.done
            text t.title
            badge t.priority
            button "Delete" confirm="Delete task?" do=delete_task id=t.id style=quiet
          when t.notes != null
            text t.notes style=quiet
      form do=add_task project=params.id
        input title text required label="New task" max=200
        area notes label="Notes" rows=2
        input estimate int label="Estimate"
        select priority label="Priority"
        submit "Add task"
```

Queries, guard, view and event bindings are all here. Nothing about this screen
lives anywhere else. This is `examples/tasks/routes/project_detail.kiln`
verbatim, and a test fails if the two drift.

## What the checker proves

`kiln check` runs in milliseconds and proves, before anything runs:

- no reference to a field that does not exist anywhere in the app
- every `do=` names a real action, with every parameter supplied at the right type
- every internal link resolves to a real route
- every style token is real — the vocabulary is closed, there is no CSS
- **no control is wired to an action the route's guard can never satisfy**

No mainstream framework checks that last one. An action allowing on `session.user`,
bound from a route with no guard establishing it, is a button that renders for
anonymous visitors and always denies them.

```
routes/project_detail.kiln:17: K022: no action named toggle_tsak
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
  text "task"
  text "0"
  col gap=0
    row gap=2
      check do=toggle_task(done=$value, id=1) value=false
      text "Ship it"
      badge "normal"
      button "Delete" do=delete_task(id=1) confirm="Delete task?" style=quiet
  form do=add_task(project="1")
    input title text required label="New task" max=200
    area notes label="Notes" rows=2
    input estimate int label="Estimate"
    select priority label="Priority" options="low, normal, high"
    submit "Add task"
```

Snapshots are committed, so a UI change is a reviewable text diff and drift is a
failing check. The `options=` line is worth a look: the view never lists the
priorities, so the snapshot prints the ones the action will accept.

## Orientation in one call

`kiln map` prints the whole app — including which actions each route's view can
invoke, which is the link an agent would otherwise read a whole view to find.
`kiln where Task.done` traces a field from its declaration through every action,
view binding and test step, distinguishing definitions from uses.

## The constraint that shapes everything

`kiln docs` emits the complete language in ~1,800 tokens. A test fails the build
if it exceeds 3,000. When the cap is hit the answer is to cut a feature, never to
raise the cap — because the moment the language stops fitting in context, an
agent starts guessing, and guessing is the cost this whole design exists to
remove.

## Runtime

A page is a pure function of (params, session, data). An interaction is a named
server action → mutation → re-query → patch. No client state, no effects, no
lifecycle, no client/server boundary — so the agent never simulates time, and
those bug classes stop existing rather than being caught.

## Try it

Needs Go 1.27 or later.

```
go install github.com/chrisnordrum/kiln/cmd/kiln@latest
```

Or from a clone, which is where the examples are:

```
go build -o kiln ./cmd/kiln
./kiln docs                    # the whole language
./kiln map examples/tasks      # the whole app
./kiln check examples/tasks
./kiln test examples/tasks
./kiln dev examples/tasks      # http://localhost:7777/projects/1
```

`kiln dev --data app.json` keeps the store across restarts. Without it the
store is in memory, and `kiln test` and `kiln snap` always start empty either
way, so a test never depends on what ran before it.

## Status

An experiment, not a product. Working end to end: lexer, parser, canonical
formatter, whole-program checker, evaluator, text and HTML renderers, test
runner and dev server, with two example apps held to the same four guarantees.
About 8,300 lines of Go plus 3,700 of tests, and **zero dependencies** — the
standard library covers it.

Whether the design works is a question for agents, not for its author, so it
was tested that way: four cold Claude Code sessions, each given only `kiln docs`
and a feature request. [`eval/RESULTS.md`](eval/RESULTS.md) has the method, the
pre-registered scoring and what went wrong, including a task the language could
not express at all. Everything it found is fixed; a second round is next.

Not yet done: `kiln docs` is not shipped as a Claude Code skill, so a session
has to be told to read it.

Human maintainability is explicitly traded away. Not for marketing sites (no
static export) or canvas/drag UIs (a server round-trip per interaction).

[CLAUDE.md](CLAUDE.md) is the contributor guide — for agents and people alike —
including the design decisions that look like mistakes and why they are not.

## License

MIT
