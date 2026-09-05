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

Round 1's bar counted cycles. It scored task 1 Weak for three, but all three
were the checker naming a fix the session then applied verbatim, landing
correct, canonical and tested in under two minutes. Counting cycles measures
how often the checker spoke, not whether the session was lost — and a
diagnostic doing its job is the design working, not the session failing. The
bar below was revised before round 2 and after round 1 was scored; round 1's
recorded outcomes stand as they were pre-registered.

A **cycle** is one `kiln check` that reports at least one diagnostic, after
the session's first edit. A check before that is observing the state it was
handed: task 4 opens on a failing check by construction, and counting that
one would mean the repair task can never score Clean.

A cycle is **guided** when the next edit applies the diagnostic's `fix:` line
as written, or its plainly equivalent form. It is **unguided** when the session
does anything else:

- edits to a form the `fix:` line does not name,
- re-reads the reference or searches before editing,
- tries more than one candidate for the same diagnostic,
- or trips the same code at the same site twice.

Guided cycles are the checker doing its job. An unguided cycle means something
failed, but not necessarily the session: task 2's single unguided cycle was the
language having no way to assert absence, so the session deleted the assertion
rather than follow a fix line that pointed nowhere useful. The count scores the
run; the backlog bucket underneath it says whose fault the run was.

Per task:

- **Clean** — correct on the first `kiln check`, no fix cycles.
- **Pass** — correct, and every cycle was guided.
- **Weak** — two or more unguided cycles, or it read something beyond
  `kiln docs`, `kiln map` and the app's own files. **The binary counts.**
  `strings kiln` recovers the diagnostic table and the implementation's own
  file names, and two of the four round-1 runs did it — task 3, which
  self-reported, and task 2, which nobody noticed until the transcripts were
  read back. It is reading the implementation by another route.
- **Fail** — never landed, or landed wrong: check passes but the feature does
  not do what was asked.

Record for each: the outcome, total cycles split guided/unguided, every file it
opened, the shell command count, and what it got wrong. `eval/record.py` pulls
all of it from the session transcript — run it after every task, because round
1 lost task 1's file list and task 2's cycle count to nobody writing them down.
It cannot judge guided from unguided; it prints each cycle's diagnostics beside
the edit that followed so that call is made against the evidence.

Every mistake sorts into exactly one bucket — a docs fix, a diagnostic fix, or
a language fix — and that list is the real backlog.

## What to watch on task 4

The K040 diagnostic currently reads:

    fix: add `guard session.user else redirect /login` to the route, or widen
         the allow rule

It offers the insecure repair as an equal alternative. Widening the allow rule
makes `check` pass and silently removes the permission. If the session takes
that branch, the diagnostic is at fault, not the session — and that is a more
valuable result than a clean pass. Leaving the wording as it is on purpose:
fixing it first would be tuning the tool to the test.
