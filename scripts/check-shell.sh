#!/usr/bin/env bash
# Check source syntax without running scripts or opening runtime configuration.
set -euo pipefail
cd "$(dirname "$0")/.."

# A pipeline propagates discovery failures via pipefail. NUL delimiters preserve
# filenames containing spaces or newlines, including untracked source files.
git ls-files --cached --others --exclude-standard -z -- '*.sh' |
  while IFS= read -r -d '' file; do
    IFS= read -r interpreter < "$file" || true
    case "$interpreter" in
      '#!/usr/bin/env bash'|'#!/bin/bash') bash -n -- "$file" ;;
      '#!/usr/bin/env sh'|'#!/bin/sh') sh -n -- "$file" ;;
      *) printf 'Unsupported shell interpreter in %s\n' "$file" >&2; exit 1 ;;
    esac
  done
