# Fresh-session test — results

Kiln at commit: 71fb58b (harness at 8a1d4c6)

| Task | Outcome | Cycles | Read beyond docs/map | What went wrong |
|------|---------|--------|----------------------|-----------------|
| 1 field + view | **Weak** | 3 | not recorded | param `?`, no truthiness, depth cap |
| 2 action + control | | | | |
| 3 new route | | | | |
| 4 repair | | | | |

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

## Backlog

### Language

**L1 — action parameters cannot be nullable.** A schema field takes `?`; an
`in` parameter does not (K010: `"?" is not a parameter modifier`). So a
nullable column is fed by a non-nullable parameter, an empty textarea stores
`""` rather than null, and every reader has to handle both. That is the whole
reason the line above exists. The asymmetry is the defect.

**L2 — the depth cap is counting the wrong thing.** An ordinary layout — a
task row with a note beneath it — reached 7 and hit K012. The cap was already
raised from 4 to 6 during the build for exactly this reason; hitting it twice
is a pattern, not bad luck. `route > view > page` spends three levels on
structure before any layout begins, so the cap should count view depth from
`page`, not file depth from the declaration.

### Diagnostics

**D1 — K040's root key is too specific.** One deleted guard produced three
diagnostics, one per action bound from the route. The Root is
`reach:<action>:<route>`, so the cascade machinery cannot collapse them. It
should be `reach:<route>:<session field>`: the missing guard is the root cause,
not each action that trips over it.

**D2 — K040 offers the insecure repair as a coequal option.** Held pending
task 4, which is the test of it.

### Docs

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
