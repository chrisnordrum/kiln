#!/bin/sh
# Build a clean room for the fresh-session test.
#
# The point is isolation: the session under test must see a Kiln *project* and
# the kiln binary, and never the Go implementation. A session that can read
# internal/check/view.go will infer the language from the compiler instead of
# from the reference, which is the thing being measured.
set -e

REPO=$(cd "$(dirname "$0")/.." && pwd)
DEST=${1:-$HOME/Developer/kiln-eval}
TASK=${2:-1}
REPLACE=${REPLACE:-0}

# Never destroy a previous run. The first run of this script deleted a
# completed task's work with no recovery path, because the clean room is not a
# git repo. Archive instead, and let the operator delete deliberately.
# Archiving is safe for data but not for an open session: moving the directory
# out from under a shell that is already cd'd into it silently relocates that
# session's work, which is how a completed task-2 run ended up in an archive
# while a pristine copy sat at the expected path. Require the intent to be
# explicit, so a reflexive setup cannot move a live directory.
if [ -d "$DEST" ]; then
  if [ "$REPLACE" != "1" ]; then
    echo "$DEST already exists." >&2
    echo "Close any session working there, then re-run with REPLACE=1 to archive it." >&2
    exit 1
  fi
  ARCHIVE="$DEST.$(date +%Y%m%d-%H%M%S)"
  mv "$DEST" "$ARCHIVE"
  echo "previous run archived to $ARCHIVE"
fi
mkdir -p "$DEST"

# Isolation is not just the filesystem. An ordinary Go binary carries its own
# source paths, and the task-3 run read every internal/ path out of this one
# with `strings kiln` before it ever found the repo. -trimpath drops the
# absolute prefix, so the binary stops disclosing where the repo lives; -s -w
# drop the symbol and DWARF tables, about 3MB.
#
# It is a reduction, not a fix. Go keeps a module-relative file table for
# tracebacks, so `kiln/internal/check/route.go` survives and cannot be removed.
# That leaves a map of the implementation, not the implementation: a name is
# only worth something if the file is also reachable. Keeping the repo out of
# reach is still the control that matters, and nothing here enforces it.
( cd "$REPO" && go build -trimpath -ldflags="-s -w" -o "$DEST/kiln" ./cmd/kiln )
cp -r "$REPO/examples/tasks" "$DEST/app"

if [ "$TASK" = "4" ]; then
  sh "$REPO/eval/break.sh" "$DEST/app"
fi

echo "clean room: $DEST"
echo "  ./kiln  the toolchain"
echo "  app/    the project under test"
