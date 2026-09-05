# Round 2 — Build Plan

## Goal

Run the fresh-session test a second time, against the language as it stands
now, and record the result in `RESULTS.md`. Done means four valid runs, each
scored under the revised rubric in `tasks.md`, with the evidence pulled from
the transcripts by `record.py` rather than from memory.

The first attempt (Sep 5) produced four runs and all four are void. Fixing the
harness so that cannot happen again is Phase 1, and it is most of the work.

## Why the first attempt was void

Read this before touching anything — it is the whole reason this plan exists.

**The clean room was never reset.** `setup.sh` did not run between tasks, so
`~/Developer/kiln-eval/app` accumulated every task's output on top of round 1's.
It still held `routes/projects.kiln`, which is round-1 task 3's work. Task 4 was
therefore never broken — `break.sh` never ran — and its "0 cycles, check clean"
means a session found a healthy app and correctly did nothing.

**The binary was two days stale.** `~/Developer/kiln-eval/kiln` was built Sep 3
at 23:36, which is round 1's task-3 build. Every fix since was absent, so three
of four sessions spent the run fighting bugs that were already closed:

| Run | Hit | Actually |
|-----|-----|----------|
| 1, cycle 1 | `due at?` → K010 | L1, fixed in P2 |
| 2, both cycles | `expect no text` → K021 | fixed in P1 |
| 3, 12 cycles + binary mining | `"/projects/" + id` → K030 | P0; it is in the shipped example |

The sessions did nothing wrong. Run 3 in particular spent 10 minutes and 53
commands re-deriving a hole that had been closed for two days, then mined the
binary to try to escape it.

Both failures are the same class as H1–H3: the harness depended on the operator
remembering a step. Three tasks in a row skipped the reset, which makes it a
design problem, not a lapse.

## Decisions

Settled. Do not relitigate.

- **The tasks were rewritten.** Round 1's tasks 1 and 3 are now implemented in
  `examples/tasks` — fixing what round 1 found meant landing the fixes in the
  example, and task 3's requirement *became* the P0 commit. A session given
  either would find the feature already there. The new texts are below.
- **Task 2 is kept verbatim** from round 1. It is the one directly comparable
  data point across rounds.
- **Both new tasks were verified expressible** before being written, so neither
  is secretly rigged to fail: `where project.owner == session.user` checks
  clean, and a nullable `at` field, parameter, input and render all work.
- **The rubric counts unguided cycles, not cycles** (`tasks.md`). `record.py`
  cannot make that call and deliberately does not try; it prints each cycle's
  diagnostics beside the edit that followed.
- **Isolation is enforced by absence, not convention.** `setup.sh` copies the
  binary, so the repo does not need to be on disk during a run. Move it aside.
  Stripping the binary was not enough: it removed the source paths, but run 3
  mined the diagnostic string table, which cannot be removed because the tool
  prints it.
- **Preamble and task go in one message.** Run 1 received them as two, which
  starts the session working before it knows the work, and makes `record.py`
  segment the transcript.

## Open questions

**For Chris — answer before Phase 2:**

1. Add a fifth task exercising enum parameters and `select` with no `from=`?
   Both shipped after round 1 and neither has ever been written by a cold
   session. Costs comparability with the four-task shape.
   *Assumption if unanswered: no, keep four.*
2. Should `RESULTS.md` get a round-2 table beside round 1's, or a separate
   document? Round 1's outcomes describe a language that no longer exists, so
   a shared table invites false comparison.
   *Assumption if unanswered: a new section in `RESULTS.md`, own table, with a
   line saying the two rounds are not comparable except on task 2.*
3. Is `mv ~/Developer/kiln` aside acceptable for the duration, or do you want
   the run containerised?
   *Assumption if unanswered: move it aside; `run.sh` will do it and put it
   back.*

## Plan

### Phase 1 — make the harness enforce the method

- [ ] **`record.py`: update the task patterns.** They still match round 1's
      text, so runs 1 and 3 reported `unrecognised`. In `TASKS`, replace
      `carry a note|fill it in` with `carry a due date`, and
      `listing all of a user|at /projects` with `every task across|at /tasks`.
- [ ] **`break.sh`: fix the drift.** It now injects five diagnostics, not four.
      `style=quiet` appears twice in `routes/project_detail.kiln` since the
      example gained a priority badge, so the K024 sed hits both lines. Anchor
      it to one line so the repair task is four defects, one per class, as its
      premise says.
- [ ] **`setup.sh`: stamp the clean room.** Write the repo's `git rev-parse
      HEAD` and the build time into `<DEST>/.kiln-eval-stamp`. This is what
      makes a stale room detectable.
- [ ] **`eval/run.sh <task>`: one command for the whole preflight.** Rebuild
      the binary from HEAD, run `setup.sh` with `REPLACE=1`, verify the stamp
      matches HEAD, move `~/Developer/kiln` aside, and put the task's prompt on
      the clipboard with `pbcopy`. Print the next step. Skipping the reset then
      becomes impossible rather than merely discouraged. It must also restore
      the repo afterwards — a run that leaves the repo moved is worse than the
      problem it solves.
- [ ] **`record.py`: refuse a stale run.** If the transcript's clean room stamp
      does not match the commit the report is generated against, say so loudly
      at the top. A run against the wrong compiler should be unusable, not
      quietly scored.
- [ ] **`tasks.md`: replace the task list** with the four below, and record why
      round 1's tasks were retired.

### Phase 2 — run it, four times

For N in 1..4, and **nothing else in the session**:

- [ ] Task 1 — due date
- [ ] Task 2 — archive
- [ ] Task 3 — the `/tasks` route
- [ ] Task 4 — repair

Each one:

```bash
sh ~/Developer/kiln/eval/run.sh <N>      # resets, verifies, copies the prompt
cd ~/Developer/kiln-eval && claude       # a brand new session
# paste. do not type anything else. do not answer questions about syntax.
# when it stops, close the session, then:
cd ~/Developer/kiln && ./eval/record.py
```

Check the pasted text landed intact before it starts working — two of round 1's
four preambles arrived visibly mangled ("lethey add a task", "the toanguage"),
which means those sessions worked from instructions nobody sent.

### Phase 3 — score and record

- [ ] Score each run: outcome, total cycles split guided/unguided, files
      opened, shell count, any isolation breach. Guided means the next edit
      applied the diagnostic's `fix:` line as written.
- [ ] Sort every mistake into exactly one bucket — docs, diagnostic, or
      language. That list is the backlog.
- [ ] Write the round-2 section of `RESULTS.md`.
- [ ] Update `CLAUDE.md`'s next-steps.
- [ ] Delete this plan file and commit the deletion.

## The four tasks

Paste the preamble and exactly one task, as a single message.

**Preamble (every task):**

    This project is written in Kiln, a language you haven't seen before. Run
    `./kiln docs` for the complete reference — it is short, and it is the whole
    language. `./kiln map app` shows the app's structure, `./kiln where <symbol>
    app` finds a symbol, `./kiln check app` verifies the program, `./kiln test
    app` runs its tests, and `./kiln snap app` updates the rendered snapshots.

    Work in `app/`.

**1 — a field, surfaced.** The easy path: schema to view.

    A task should be able to carry a due date. Add it, show it with the task on
    the project page, and let someone set it when they add a task.

**2 — an action with a permission rule.** The guard/allow relationship.
Unchanged from round 1.

    Add a way to archive a task instead of deleting it. Archived tasks shouldn't
    show in the list. Only the project's owner should be able to archive one.

**3 — a new route.** Does `data`, cardinality and guards land without a model?

    Add a page listing every task across all of a user's projects, at /tasks. It
    should show each task's title as a link to the project it belongs to, with the
    newest first, and only be reachable when signed in.

**4 — repair.** Tests the diagnostics directly, which is half the design.

    `./kiln check app` is failing. Fix it.

## Out of scope / deferred

- **`send` is a no-op.** `effect email via smtp` and `send` parse, pass
  `check`, and do nothing — `internal/runner/runner.go` groups `*ast.Send` with
  the after-statements it ignores, and the server does not handle it either.
  The reference documents effects as the only way out of the language. Real,
  and its own piece of work.
- **Containerised isolation.** Moving the repo aside is enough for now.
- **The `kiln docs` skill** (`CLAUDE.md` next-steps item 2). Separate.
- **`plural(n, ...)` returns only the noun**, so a page reads "report" rather
  than "1 report". Composable on purpose; changing it breaks the other use.
- **`ago()` against the fixed clock** reads "in 8 months" for a past date.
  That is the fixed-clock decision doing what it says.
- **A `select` bound to a record list still renders row ids** as both value and
  label. Pre-existing, unrelated to the enum work, and nothing exercises it.
