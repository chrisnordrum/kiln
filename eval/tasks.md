# The fresh-session test

Everything built so far is an argument for the design. This is the only part
that is evidence. It answers one question: **can an agent that has never seen
Kiln write correct Kiln from the reference alone?**

## Method

Run each task in its own new session, so nothing carries over. Before each:

    REPLACE=1 sh ~/Developer/kiln/eval/setup.sh ~/Developer/kiln-eval <task-number>

(the full path matters — the clean room has no copy of this script in it.
Close any session working there first: the reset moves that directory, and a
shell already inside it will keep writing to the moved copy.)

then open the session in `~/Developer/kiln-eval` — never in this repo. A session
with access to `internal/` will infer the language from the compiler rather than
from the reference, which is exactly what must not happen.

Paste the preamble and one task. Nothing else. Do not answer questions about
syntax: if the session cannot get there from `kiln docs`, that is the result.

## Preamble (paste before every task)

> This project is written in Kiln, a language you haven't seen before. Run
> `./kiln docs` for the complete reference — it is short, and it is the whole
> language. `./kiln map app` shows the app's structure, `./kiln where <symbol>
> app` finds a symbol, `./kiln check app` verifies the program, `./kiln test
> app` runs its tests, and `./kiln snap app` updates the rendered snapshots.
>
> Work in `app/`.

## Tasks

**1 — Add a field and surface it.** The easy path: schema to view.

> A task should be able to carry a note. Add it, show it under the task title
> on the project page, and let someone fill it in when they add a task.

**2 — Add an action and wire a control.** The guard/allow relationship.

> Add a way to archive a task instead of deleting it. Archived tasks shouldn't
> show in the list. Only the project's owner should be able to archive one.

**3 — Add a route.** Does `data`, cardinality and guards land without a model?

> Add a page listing all of a user's projects, at /projects. It should show
> each project's title as a link to that project, with the newest first, and
> only be reachable when signed in.

**4 — Repair.** Tests the diagnostics directly, which is half the design.

> `./kiln check app` is failing. Fix it.

## Scoring, decided before running

Per task:

- **Clean** — correct on the first `kiln check`, no fix cycles.
- **Pass** — correct within two check-fix cycles.
- **Weak** — three or more cycles, or it read something beyond `kiln docs`,
  `kiln map` and the app's own files.
- **Fail** — never landed, or landed wrong: check passes but the feature does
  not do what was asked.

Record for each: the outcome, the number of cycles, every file it opened, and
what it got wrong. Every mistake sorts into exactly one bucket — a docs fix, a
diagnostic fix, or a language fix — and that list is the real backlog.

## What to watch on task 4

The K040 diagnostic currently reads:

    fix: add `guard session.user else redirect /login` to the route, or widen
         the allow rule

It offers the insecure repair as an equal alternative. Widening the allow rule
makes `check` pass and silently removes the permission. If the session takes
that branch, the diagnostic is at fault, not the session — and that is a more
valuable result than a clean pass. Leaving the wording as it is on purpose:
fixing it first would be tuning the tool to the test.
