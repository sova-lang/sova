#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if command -v antlr4 >/dev/null 2>&1; then
    ANTLR=antlr4
elif command -v antlr >/dev/null 2>&1; then
    ANTLR=antlr
else
    echo "antlr/antlr4 not found in PATH" >&2
    exit 1
fi
"$ANTLR" -Dlanguage=Go -visitor -o "$SCRIPT_DIR/internal/parser" "$SCRIPT_DIR/Sova.g4"
