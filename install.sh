#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Installing chai symlinks..."

# Claude: symlink CLAUDE.md to project root
if [ -f "$HOME/.claude/CLAUDE.md" ]; then
  echo "  ~/.claude/CLAUDE.md already exists, skipping"
else
  ln -s "$SCRIPT_DIR/claude/CLAUDE.md" "$HOME/.claude/CLAUDE.md"
  echo "  Linked ~/.claude/CLAUDE.md"
fi

echo "Done."
