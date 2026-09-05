#!/usr/bin/env python3
"""Pull a fresh-session run's evidence out of its transcript.

Round 1 lost task 1's file list and task 2's cycle count because both were
things the operator had to notice and write down. Everything the rubric asks
for is already in the session transcript, so read it from there instead.

    ./eval/record.py            # the newest run in the clean room
    ./eval/record.py --list     # every run, newest first
    ./eval/record.py -s 282b4d2c

What it cannot do is tell a guided cycle from an unguided one. That is a
judgement about whether an edit followed the diagnostic's fix line, so it
prints the two side by side and leaves the call to the reader.
"""

import argparse
import datetime
import json
import pathlib
import re
import sys

CLEAN_ROOM = pathlib.Path.home() / "Developer" / "kiln-eval"
PROJECTS = pathlib.Path.home() / ".claude" / "projects"

# The isolation rule: the session may read the reference, the map, and the
# app's own files. Anything else is a rubric violation; the Go implementation
# is the one that actually poisons the run, so it is called out by name.
APP_FILE = re.compile(r"^(app/|schema/|routes/|actions/|tests/|snap/|[\w*-]+\.(kiln|txt)$)")
# the clean room, however the session spelled it: absolute, or an archived copy
ROOM_PREFIX = re.compile(r"\S*/kiln-eval[^/]*/")
SELF = re.compile(r"^/private/tmp/|^/tmp/|scratchpad/")
IMPL = re.compile(r"/Developer/kiln/|(?<!-)\binternal/|\.go\b|cmd/kiln")
# The clean room ships the compiled binary, and an unstripped Go binary carries
# its own source paths and string table. Task 3 read the whole K-code list and
# every internal/ path out of it without the repo being reachable at all.
MINING = re.compile(r"\b(strings|nm|objdump|otool|xxd|hexdump)\b[^|;]*\bkiln\b")
READS = re.compile(r"\b(cat|sed|head|tail|less|more|grep|rg|awk|wc|diff|find|ls)\b")
WRITES = re.compile(r"(cat\s*>|python3\s*-\s*<<|sed\s+-i|printf.*>|tee\s)")
PATHISH = re.compile(r"[\w*./~-]+\.(?:kiln|txt|md|go|json|sh)\b|\bapp/[\w*./-]+")
TASKS = [
    (re.compile(r"carry a note|fill it in", re.I), "1 — a field, surfaced"),
    (re.compile(r"archive a task|archived tasks", re.I), "2 — an action with a permission rule"),
    (re.compile(r"listing all of a user|at /projects", re.I), "3 — a new route"),
    (re.compile(r"is failing\.\s*fix it", re.I), "4 — repair"),
]


def which_task(prompt):
    """The task is the last thing in the prompt.

    Not the first match: a paste that arrives garbled can carry fragments of
    the previous task ahead of the real one, which is how a task-4 run read as
    task 1. Whichever pattern matches latest is the one that was asked for.
    """
    hits = [(m.start(), name) for pat, name in TASKS for m in [pat.search(prompt)] if m]
    return max(hits)[1] if hits else "unrecognised"


def transcripts():
    """Every session recorded against the clean room, newest last."""
    slug = str(CLEAN_ROOM).replace("/", "-")
    found = []
    for d in PROJECTS.glob(slug + "*"):
        found += list(d.glob("*.jsonl"))
    return sorted(found, key=lambda p: p.stat().st_mtime)


def local(ts):
    return datetime.datetime.fromisoformat(ts.replace("Z", "+00:00")).astimezone()


def parse(path):
    """Split a transcript into segments, one per operator turn.

    The method is one session per task, but nothing enforces it: a run that
    reset the clean room and started task 4 in the session that had just
    finished task 1 puts two tasks in one file. Attributing all of it to the
    first task credits task 1 with the other task's cycles and its reads.
    Every operator message starts a new segment, so a doubled-up session
    reports as what it is.
    """
    segments, pending = [], {}
    for line in path.open():
        try:
            entry = json.loads(line)
        except json.JSONDecodeError:
            continue
        body = (entry.get("message") or {}).get("content")
        if entry.get("type") == "user" and isinstance(body, str) and body.strip():
            segments.append(
                {"prompt": body.strip(), "calls": [], "first": None, "last": None}
            )
        if not segments:
            continue
        seg = segments[-1]
        if entry.get("timestamp"):
            stamp = local(entry["timestamp"])
            seg["first"] = seg["first"] or stamp
            seg["last"] = stamp
        if not isinstance(body, list):
            continue
        for block in body:
            if not isinstance(block, dict):
                continue
            if block.get("type") == "tool_use" and block.get("name") == "Bash":
                call = {"cmd": block["input"].get("command", ""), "out": ""}
                pending[block["id"]] = call
                seg["calls"].append(call)
            elif block.get("type") == "tool_result":
                call = pending.get(block.get("tool_use_id"))
                if call is None:
                    continue
                content = block.get("content")
                if isinstance(content, str):
                    call["out"] = content
                elif isinstance(content, list):
                    call["out"] = "".join(
                        c.get("text", "") for c in content if isinstance(c, dict)
                    )
    for seg in segments:
        seg["task"] = which_task(seg["prompt"])
    return segments


def diagnostics(out):
    """The K-codes a check reported, each with the fix it offered."""
    found = []
    lines = out.splitlines()
    for i, line in enumerate(lines):
        hit = re.search(r"(\S+:\d+): (K\d+): (.+)", line)
        if not hit:
            continue
        fix = ""
        for nxt in lines[i + 1 : i + 3]:
            if "fix:" in nxt:
                fix = nxt.split("fix:", 1)[1].strip()
                break
        found.append((hit.group(2), hit.group(1), hit.group(3).strip(), fix))
    return found


def files_touched(calls):
    """Split every path the session read into app / outside / its own scratch."""
    app, outside, mining = {}, {}, []
    for call in calls:
        cmd = call["cmd"]
        if MINING.search(cmd):
            mining.append(" ".join(cmd.split())[:120])
        if not (READS.search(cmd) or WRITES.search(cmd)):
            continue
        for hit in PATHISH.finditer(cmd):
            path = hit.group(0).strip("'\"")
            # `$S/projects.kiln` is a variable expansion, not a path
            if hit.start() and cmd[hit.start() - 1] in "${":
                continue
            if not path or path.startswith("./kiln") or re.fullmatch(r"\.\w+", path):
                continue
            if SELF.search(path):
                continue  # a temp file it wrote itself is not a read
            # an app file reached by its full path is still an app file
            path = ROOM_PREFIX.sub("", path)
            bucket = app if APP_FILE.match(path) else outside
            bucket[path] = bucket.get(path, 0) + 1
    return app, outside, mining


def path_is_impl(path):
    return bool(IMPL.search(path))


def segment_report(seg, label):
    calls = seg["calls"]
    checks = [c for c in calls if re.search(r"kiln check", c["cmd"])]
    # A check before the session has edited anything is observing the state it
    # was handed, not a cycle it caused. Task 4 opens on a failing check by
    # construction — counting that one means the repair task can never be
    # Clean, which is the outcome it was written to test for.
    first_edit = next((i for i, c in enumerate(calls) if WRITES.search(c["cmd"])), 0)
    cycles = [
        c for c in checks if diagnostics(c["out"]) and calls.index(c) > first_edit
    ]
    inside, outside, mining = files_touched(calls)
    first, last = seg["first"], seg["last"]
    elapsed = (last - first).total_seconds() / 60 if first and last else 0

    print(f"\n{label}")
    print(f"task     {seg['task']}")
    print(f"clock    {first:%b %d %H:%M} -> {last:%H:%M}  ({elapsed:.0f} min)")
    print(f"shell    {len(calls)} commands, {len(checks)} of them a check")
    print(f"cycles   {len(cycles)}   (guided/unguided is yours to judge, below)")

    # Clean means the last check reported nothing, not that its output opened
    # with "ok" — the final check is usually chained behind `kiln fmt`, whose
    # output comes first.
    clean = bool(checks) and not diagnostics(checks[-1]["out"])
    print(f"ended    {'check clean' if clean else 'CHECK NOT CLEAN'}")

    if outside or mining:
        print("\nISOLATION BREACH — read beyond docs, map and app/. Weak on its own.")
        for path_, n in sorted(outside.items(), key=lambda kv: (not path_is_impl(kv[0]), -kv[1])):
            tag = "compiler" if path_is_impl(path_) else "        "
            print(f"  {tag} {n:>3}x  {path_}")
        for cmd in mining:
            print(f"  binary   {cmd}")
        if mining:
            print("  Mining the binary needs no access to the repo at all —")
            print("  build it stripped and trimmed so it carries no source.")

    print(f"\nfiles opened ({len(inside)}):")
    for path_, n in sorted(inside.items(), key=lambda kv: -kv[1]):
        print(f"  {n:>3}x  {path_}")

    print("\ncycles, each beside the edit that followed it:")
    if not cycles:
        print("  none — clean on the first check.")
    for i, cycle in enumerate(cycles, 1):
        print(f"\n  --- cycle {i} " + "-" * 52)
        for code, where, msg, fix in diagnostics(cycle["out"]):
            print(f"  {code} at {where}: {msg}")
            print(f"       fix: {fix or '(none offered)'}")
        after = calls[calls.index(cycle) + 1 :]
        edit = next((c for c in after if WRITES.search(c["cmd"])), None)
        if edit:
            print(f"  next edit: {' '.join(edit['cmd'].split())[:300]}")
        else:
            print("  next edit: none — the run ended here.")


def report(path):
    segments = parse(path)
    print(f"run      {path.stem[:8]}   {path.parent.name}")
    if len(segments) > 1:
        print(f"\nWARNING  {len(segments)} operator turns in one session. The method is one")
        print("         session per task; anything after the first is a different run")
        print("         sharing this transcript, and is reported separately below.")
    for i, seg in enumerate(segments, 1):
        segment_report(seg, f"--- segment {i}/{len(segments)} " + "=" * 46)
    print("\nguided   = the next edit applied the fix line as written")
    print("unguided = anything else. Two or more is Weak.")


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("-s", "--session", help="session id prefix")
    ap.add_argument("-l", "--list", action="store_true", help="list runs, newest first")
    args = ap.parse_args()

    runs = transcripts()
    if not runs:
        sys.exit(f"no transcripts for {CLEAN_ROOM} under {PROJECTS}")
    if args.list:
        for path in reversed(runs):
            segs = parse(path)
            tasks = ", ".join(s["task"].split(" — ")[0] for s in segs)
            n = sum(len(s["calls"]) for s in segs)
            when = segs[0]["first"] if segs else None
            flag = "  (multi-task session)" if len(segs) > 1 else ""
            print(f"{path.stem[:8]}  {when:%b %d %H:%M}  {n:>3} cmds  task {tasks}{flag}")
        return
    if args.session:
        runs = [p for p in runs if p.stem.startswith(args.session)]
        if not runs:
            sys.exit(f"no run matching {args.session}")
    report(runs[-1])


if __name__ == "__main__":
    main()
