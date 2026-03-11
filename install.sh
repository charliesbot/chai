#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Installing chai symlinks..."

# In-repo symlinks (so AI tools find their config files)
ln -sf "$SCRIPT_DIR/_AGENTS.md" "$SCRIPT_DIR/CLAUDE.md"
ln -sf "$SCRIPT_DIR/_AGENTS.md" "$SCRIPT_DIR/GEMINI.md"
ln -sf "$SCRIPT_DIR/_AGENTS.md" "$SCRIPT_DIR/claude/_CLAUDE.md"
ln -sf "$SCRIPT_DIR/_AGENTS.md" "$SCRIPT_DIR/gemini/_GEMINI.md"
echo "  Linked in-repo CLAUDE.md, GEMINI.md, claude/_CLAUDE.md, gemini/_GEMINI.md"

# Global Claude config
if [ -f "$HOME/.claude/CLAUDE.md" ]; then
  echo "  ~/.claude/CLAUDE.md already exists, skipping"
else
  ln -s "$SCRIPT_DIR/claude/_CLAUDE.md" "$HOME/.claude/CLAUDE.md"
  echo "  Linked ~/.claude/CLAUDE.md"
fi

echo "Done."
