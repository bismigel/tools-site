#!/usr/bin/env bash
# make dev — one process group, Ctrl-C stops both.
set -euo pipefail
cd "$(dirname "$0")/.."
trap 'kill 0' EXIT
make css-watch &
watchexec -r -e go -- make run
