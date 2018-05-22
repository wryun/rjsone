#!/bin/sh

set -eu

TEMPDIR="$(mktemp -d)"
trap "rm -rf $TEMPDIR" EXIT

go build
export PATH="$(readlink -f .):$PATH"

run() {
  local outputdir
  set +e
  outputdir="$(readlink -f "$2")"
  cd "$1"
  "./run.sh" > "$outputdir/stdout" 2> "$outputdir/stderr"
  echo "$?" > "$outputdir/exitcode"
  cd - > /dev/null
  set -e
}

PASS=0
FAIL=0

if [ "$1" = "-g" ]; then
  # generate 'golden' files (i.e. test output)
  shift

  echo "Generating new test outputs - be careful..."
  for f in "$@"; do
    OUTPUTDIR="$f/expected"
    if [ -e $OUTPUTDIR ]; then
      echo "Skipping $OUTPUTDIR since it already exists."
      continue
    fi
    mkdir -p "$OUTPUTDIR"
    run "$f" "$OUTPUTDIR"
    echo "$f exitcode=$(cat "$OUTPUTDIR/exitcode") stderr=$(cat "$OUTPUTDIR/stderr")"
    cat "$OUTPUTDIR/stdout"
  done
else
  for f in "$@"; do
    echo "$f"
    run "$f" "$TEMPDIR"
    if diff "$f"/expected "$TEMPDIR"; then
      PASS="$(expr $PASS + 1)"
    else
      FAIL="$(expr $FAIL + 1)"
    fi
  done

  echo "--- $FAIL failed, $PASS passed ---"
  test "$FAIL" -eq 0
fi
