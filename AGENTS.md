# chai

Charlie's AI configuration repo — shared config for Claude and Gemini.

## Key Concepts

- `CHAI_AGENTS.md` is the single source of truth for user-level agent instructions. It is symlinked to `~/.claude/CLAUDE.md` and `~/.gemini/GEMINI.md` via `install.sh`.
- `AGENTS.md` in this directory is project-level config for working in this repo only.

## Structure

- `skills/` — reusable skills shared across projects
- `mcp/` — shared MCP server configs
- `claude/` — Claude-specific config (settings, rules)
- `gemini/` — Gemini-specific config (settings)

## Workflow

- Run `./install.sh` after changing config files to update symlinks.
- Do not edit `~/.claude/CLAUDE.md` or `~/.gemini/GEMINI.md` directly — edit `CHAI_AGENTS.md` instead.
