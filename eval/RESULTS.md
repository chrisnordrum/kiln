# Fresh-session test — results

Kiln at commit: 71fb58b (harness at 8a1d4c6)

| Task | Outcome | Cycles | Read beyond docs/map | What went wrong |
|------|---------|--------|----------------------|-----------------|
| 1 field + view | **Weak** | 3 | not recorded | param `?`, no truthiness, depth cap |
| 2 action + control | **Pass** (cycles unverified) | ? | 1 file + 2 searches | nothing wrong |
| 3 new route | **Fail** | 35 shell cmds | **read the compiler** | the language cannot link to a record |
| 4 repair | **Clean** | 0 | docs, map, app only | nothing |

## Task 1 — a field, surfaced

Landed correctly in 1m53s. Verified independently from this repo: canonical,
`check` clean, 4 tests pass, snapshots match. The output is kept at
`eval/results/task1-app/` as the evidence.

The code is idiomatic — close to what the language's author would write. The
one wart is forced by the language, not chosen:

    when len(coalesce(t.notes, "")) > 0
      text t.notes style=quiet

Scored **Weak** against the pre-registered bar, which puts three or more
check-fix cycles there. Worth recording the tension rather than rescoring after
the fact: all three corrections were the checker teaching the language, and the
result was correct, canonical and tested. The rubric conflates "the checker
caught it" with "the session struggled", and those are not the same thing. Fix
the rubric for later runs; do not move this score.

Not recorded this run: which files it opened. That is half of what the "one
screen, one read" claim needs, so record it from task 2 on.

## Task 4 — repair

Verified independently: the repaired app is **byte-identical to the pristine
original**. All four defects fixed, no collateral changes, canonical form
intact. Zero check-fix cycles.

**The snapshot turned out to be a specification, not just a regression check.**
Two of the four defects were underdetermined by the checker alone — K031 admits
`done=$value`, `done=true` or `done=t.done`, and K024 admits any of six style
tokens. All of those pass `check`. The committed snapshot pinned both, because
it records `check do=toggle_task(done=$value, id=1)` and `style=quiet`. That
benefit was not designed for: snapshots were built as a regression channel and
are also acting as an executable spec for anything the type system leaves open.
It is the most valuable result of the run.

**D2 did not fire.** The session added the guard rather than widening the allow
rule. That is one favourable data point, not a clearance: three K040s all named
the same missing guard, and the snapshot showed the app as it ought to look.
The wording defect stands on its own terms — two repairs with different
security consequences are still presented as coequal alternatives, and the
session got it right despite the message rather than because of it. Downgraded
in urgency, not resolved.

**D1 needs a refinement.** The triplicate K040 helped here: three diagnostics
naming one missing guard made the root cause obvious. Collapsing them is still
right, but the collapsed message has to carry the scope — "route
project_detail does not guard session.user, which 3 bound actions require" —
rather than silently dropping two of them.

## Task 2 — an action with a permission rule

Verified independently: canonical, `check` clean, 4 tests passing, snapshots
matching. Kept at `eval/results/task2-app/`.

The implementation is correct and the choices are the right ones:

    archived bool = false                                    # schema
    where project == params.id and archived == false         # query, not view
    allow session.user == Task[id].project.owner             # mirrors delete_task

**My pre-registered prediction was wrong.** I predicted K053 would bite — that
a new non-nullable `archived bool` would break `add_task` in a different file,
exercising the cross-file consequence React cannot catch. The session added
`archived bool = false` with a default, so `add_task` needed no change at all
and K053 never fired. The situation I wanted to observe was avoided rather than
survived. Recording it as a miss: the schema's default mechanism steered the
session right without a diagnostic being needed, which is prevention rather
than detection, and I did not anticipate it.

The second prediction held: it filtered in the query rather than wrapping the
view in a `when`. The view version would also have passed `check` and `test`.

**A new language gap, found by the session:** there is no negative text
assertion. `expect text "..."` exists; nothing expresses "this must not
render". Wanting to assert that an archived task is absent, it fell back on the
snapshot.

That is the **second independent time** a snapshot covered a hole the type and
test systems leave open — task 4 used it to disambiguate repairs the checker
accepts several answers for, task 2 to assert absence. Two sessions, two
different holes, the same fallback. The signal cuts both ways: the verification
channel is load-bearing in ways it was not designed for, and the primitives it
is standing in for are genuinely missing.

**Evidence gap:** the transcript was truncated before the end, so the check-fix
cycle count is unknown and the Pass is provisional. Files opened were captured
this time — one file and two searches, which is the first real evidence for the
"one screen, one read" claim.

## Task 3 — a new route

**Fail**, and the failure is the language's, not the session's. The route,
guard, scoping, ordering, index and tests are all correct and land clean. The
requirement it could not meet is "show each project's title as a link to that
project", because **Kiln cannot express a link to a specific record**.

The session was right to escalate rather than ship it. Every claim verified
independently:

    link p.title to="/projects/p.id"     → check: ok, renders the literal
    "/projects/" + p.id                  → K030: cannot apply + to path and id
    "/projects/" + p.title               → check: ok

The rendered snapshot for two different projects:

    link "Site"  to="/projects/p.id"
    link "Other" to="/projects/p.id"

`kiln check` is green on a page where every link 404s. That is precisely the
failure mode this language exists to prevent, and it is the most serious
finding of the four runs.

Three defects are stacked here:

**R1 — the Path type is over-eager.** Any string literal starting with `/` is
typed Path. That was to enable link validation, and it poisons concatenation:
Path + ID is neither Text nor numeric, so it errors. Building a URL from a
title works; building one from an id does not.

**R2 — checkPath matches a literal against the wildcard.**
`pathMatches("/projects/:id", "/projects/p.id")` is true, because the pattern
segment is a wildcard that accepts any text. So the broken literal validates.
This is the silent failure itself.

**R3 — there is no interpolation and no id-to-text builtin,** so the thing
cannot be expressed at all.

A fourth, smaller: K030's fix line here reads "arithmetic needs numbers" for
what is plainly string concatenation. That sends a reader the wrong way.

**A second gap the session found: a guard redirect cannot be tested.** `visit`
fails when the guard redirects, and `expect` has no form for it, so guard
behaviour is statically proven by K040 and completely uncovered by tests.

**Rubric violation, reported by the session:** it read the compiler at
`~/Developer/kiln` because the reference does not say either way. That alone
scores Weak. The deeper problem is that the clean room was isolated by
convention rather than enforcement — under a skill-based deployment the
compiler would not be there, and the run would have shipped the broken literal
link with a green check.

It took 35 shell commands, against 8 for task 4.

## Backlog

### Language

*(all resolved: L1 in P2, the task-3 link gap in P0, L2 in P3.)*

**L1 — FIXED (P2).** Action parameters could not be nullable. A schema field takes `?`; an
`in` parameter does not (K010: `"?" is not a parameter modifier`). So a
nullable column is fed by a non-nullable parameter, an empty textarea stores
`""` rather than null, and every reader has to handle both. That is the whole
reason the line above exists. The asymmetry is the defect.

**L2 — FIXED (P3).** The depth cap was counting the wrong thing. An ordinary layout — a
task row with a note beneath it — reached 7 and hit K012. The cap was already
raised from 4 to 6 during the build for exactly this reason; hitting it twice
is a pattern, not bad luck. `route > view > page` spends three levels on
structure before any layout begins, so the cap should count view depth from
`page`, not file depth from the declaration.

### Diagnostics

*(all resolved: the task-3 silent failure as K026 in P0, D1 and D2 in P4.)*

**D1 — FIXED (P4).** K040's root key was too specific. One deleted guard produced three
diagnostics, one per action bound from the route. The Root is
`reach:<action>:<route>`, so the cascade machinery cannot collapse them. It
should be `reach:<route>:<session field>`: the missing guard is the root cause,
not each action that trips over it.

**D2 — FIXED (P4).** K040 offered the insecure repair as a coequal option.
Task 4 chose correctly despite the wording; the wording was still wrong.

### Docs

*(Doc1 partly addressed: nullability and dynamic links are now documented.)*

**Doc1 — two rules cost a cycle each and are not written down:** that `in`
parameters take no `?`, and that a condition needs a real bool (there is no
truthiness, so `when t.notes` is rejected). Roughly 15 tokens would prevent
both, against 49% unused budget.

### Harness

**H1 — fixed.** `setup.sh` destroyed the completed task-1 work with `rm -rf`
and no recovery path, since the clean room is not a git repo. It now archives
to a timestamped directory instead.

**H2 — fixed.** The reset instruction was written relative to the repo, which
invites running it from the clean room where it does not exist. It now gives
the full path.

**H3 — fixed.** Archiving protects data but not an open session: moving the
directory out from under a shell already inside it silently relocates that
session's work. A completed task-2 run landed in an archive while a pristine
copy sat at the expected path. The reset now refuses unless `REPLACE=1` is
set, so a reflexive setup cannot move a live directory. The deeper fix is
behavioural — do not run the reset proactively; the operator runs it when no
session is open.

## Closing state

Every item found by the four runs is fixed. The requirement task 3 could not
meet is now in the example and covered by a test:

    each projects as p
      row gap=2
        link p.title to="/projects/" + p.id

What the experiment cost: four sessions. What it bought: one silent failure in
the core claim, two missing test primitives, one wrong type rule, one cap
measuring the wrong thing, and one diagnostic burying its own cause. None of it
surfaced in four days of building the language, because the example app was too
thin to exercise it.

The rubric needs one change before a second round. It scored task 1 Weak for
three check-fix cycles, but all three were the checker teaching the language to
a session that then produced correct, canonical, tested code in under two
minutes. "The checker caught it" and "the session struggled" are different
outcomes and the bar cannot currently tell them apart.
