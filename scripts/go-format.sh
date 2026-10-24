#!/usr/bin/env bash
# Include new source files without staging them, while respecting Git ignores.
set -euo pipefail
cd "$(dirname "$0")/.."
mapfile -d '' sources < <(git ls-files --cached --others --exclude-standard -z -- '*.go')
if [[ ${#sources[@]} -eq 0 ]]; then exit 0; fi
case "${1:---check}" in
  --write) gofmt -w "${sources[@]}" ;;
  --check)
    unformatted="$(gofmt -l "${sources[@]}")"
    if [[ -n "$unformatted" ]]; then
      printf '%s\nRun make fmt to format these files.\n' "$unformatted" >&2
      exit 1
    fi
    ;;
  *) echo 'Usage: scripts/go-format.sh [--check|--write]' >&2; exit 2 ;;
esac
