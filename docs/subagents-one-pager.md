# Subagents Sync — One Pager

## What is a subagent?

A markdown file with YAML frontmatter (config) and a body (system prompt). Both Claude Code and Gemini CLI use the same format. The file lives in a platform-specific directory and the platform loads it at session start.

|                   | Claude Code         | Gemini CLI          |
| ----------------- | ------------------- | ------------------- |
| User-level dir    | `~/.claude/agents/` | `~/.gemini/agents/` |
| Project-level dir | `.claude/agents/`   | `.gemini/agents/`   |

## Frontmatter field comparison

### Portable (same semantics, same or trivially different syntax)

| Field       | Claude        | Gemini        | Notes                               |
| ----------- | ------------- | ------------- | ----------------------------------- |
| name        | `name`        | `name`        | Identical                           |
| description | `description` | `description` | Identical                           |
| MCP servers | `mcpServers`  | `mcpServers`  | Both support inline MCP definitions |

### Portable with translation (same concept, different values)

| Field      | Claude                    | Gemini                                  | Translation needed                       |
| ---------- | ------------------------- | --------------------------------------- | ---------------------------------------- |
| model      | `model: sonnet`           | `model: gemini-3-flash`                 | Model name mapping                       |
| turn limit | `maxTurns: 10`            | `max_turns: 10`                         | Field name + casing                      |
| tools      | `tools: Read, Grep, Glob` | `tools: [read_file, grep_search, glob]` | Tool name mapping + format (CSV vs list) |

### Not portable (platform-exclusive)

| Claude-only                         | Gemini-only           |
| ----------------------------------- | --------------------- |
| `effort` (low/medium/high/max)      | `temperature` (float) |
| `isolation` (worktree)              | `kind` (local)        |
| `color` (display color)             |                       |
| `memory` (user/project/local)       |                       |
| `hooks` (lifecycle hooks)           |                       |
| `permissionMode`                    |                       |
| `background` (run in background)    |                       |
| `skills` (load skills into context) |                       |
| `initialPrompt`                     |                       |
| `disallowedTools`                   |                       |

### The system prompt (markdown body)

Fully portable. This is where the agent's behavior, persona, and instructions live. It's plain text — no platform-specific syntax.

## What's actually the hard part?

The system prompt is portable. The metadata fields are either trivially portable or platform-exclusive. The real friction is **tools**:

1. **Built-in tools have different names.** `Read` vs `read_file`, `Bash` vs `run_shell_command`. A finite mapping (~10 entries) covers this.
2. **MCP tools use the same names on both platforms** — no translation needed.
3. **Some tools exist on one platform but not the other.** `Agent` (spawn subagents) is Claude-specific. Gemini has tools Claude doesn't.

Tools are the only field where "just drop it" means the agent is broken, not just degraded. An agent without its tools can't do its job.

## Recommended approach: minimal frontmatter + behavioral prompts

Real-world validation: [superpowers](https://github.com/obra/superpowers) ships agents across 6+ platforms (Claude, Gemini, Cursor, Codex, OpenCode, Copilot) using this exact strategy.

**Frontmatter**: only `name`, `description`, and `model: inherit`. Nothing platform-specific.

**Tool restrictions**: expressed as behavioral instructions in the system prompt body ("You will review code, not edit it") rather than frontmatter `tools` fields. Both platforms follow in-prompt constraints.

**Platform-exclusive fields** (e.g. `isolation`, `temperature`): omitted. Platforms use their defaults. These can be layered later via platform-specific TOML sections if needed.

**Tradeoff**: tool restrictions become soft constraints (the agent *probably won't* use blocked tools) instead of hard enforcement (the platform *prevents* it). For advisory agents (reviewers, researchers, planners) this is fine. For operational agents that write/deploy, the risk is higher — but acceptable as a v1.

### Sync strategy

- **Symlink** (same as skills) — no transformation needed since files are portable as-is.
- Subagents are **individual `.md` files**, not directories. The current `resolvePatterns` filters out non-directories and needs to be updated to accept `.md` files.
- Platform-exclusive frontmatter fields are inert on the other platform (unknown keys are ignored).

### Implementation delta from current code

1. **Fix `resolvePatterns`** (`internal/sync/skills.go`) — accept `.md` files, not just directories.
2. **Fix Claude platform path** (`internal/platform/platform.go`) — change `.claude/subagents` to `.claude/agents`.
3. **`syncSymlinks`** works as-is for files — `os.Symlink` doesn't care if the target is a file or directory.
