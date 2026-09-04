# Kiln

A full-stack web language for LLM coding agents.

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

## The bet

```
kiln docs      # the entire language, ~1400 tokens, read it in one go
kiln check     # proves the whole program before anything runs
kiln snap      # renders a route to text, so UI is verified without a browser
kiln map       # the whole app as ~50 lines of outline
```

A page is a pure function of (params, session, data). An interaction is a named
server action → mutation → re-query → patch. No client state, no effects, no
lifecycle — so the agent never simulates time, and a whole class of invisible
bugs stops existing.

`kiln check` proves, statically: no reference to a field that doesn't exist;
every `do=` names a real action with every param supplied at the right type;
every `allow` rule is satisfiable from its route's guard; every internal link
resolves. React cannot offer these.

## Status

Early. Phase 1 of 7 — the language reference is written and enforced under
budget; the parser, checker, renderer and runtime follow. `examples/tasks` is
the target program those phases build toward.

Human maintainability is explicitly traded away. This is not for marketing
sites (no static export) or canvas/drag UIs (server round-trip per interaction).

## Build

```
go build -o kiln ./cmd/kiln && ./kiln docs
```
