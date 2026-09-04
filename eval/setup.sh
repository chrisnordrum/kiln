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

rm -rf "$DEST"
mkdir -p "$DEST"

( cd "$REPO" && go build -o "$DEST/kiln" ./cmd/kiln )
cp -r "$REPO/examples/tasks" "$DEST/app"

if [ "$TASK" = "4" ]; then
  sh "$REPO/eval/break.sh" "$DEST/app"
fi

echo "clean room: $DEST"
echo "  ./kiln  the toolchain"
echo "  app/    the project under test"
