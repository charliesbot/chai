#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
echo "Installing chai symlinks..."

# Global symlinks (so AI tools find shared config)
ln -sf "$SCRIPT_DIR/CHAI_AGENTS.md" "$HOME/.claude/CLAUDE.md"
ln -sf "$SCRIPT_DIR/CHAI_AGENTS.md" "$HOME/.gemini/GEMINI.md"
echo "  Linked CHAI_AGENTS.md → ~/.claude/CLAUDE.md, ~/.gemini/GEMINI.md"

# Skills
SKILLS_DIR="$SCRIPT_DIR/skills"
if [ -d "$SKILLS_DIR" ]; then
  for tool_skills in "$HOME/.claude/skills" "$HOME/.gemini/skills"; do
    mkdir -p "$tool_skills"
    for skill in "$SKILLS_DIR"/*/; do
      skill_name="$(basename "$skill")"
      ln -sf "$skill" "$tool_skills/$skill_name"
    done
  done
  echo "  Linked $(ls "$SKILLS_DIR" | wc -l | tr -d ' ') skills to Claude and Gemini"
fi

echo "Done."
