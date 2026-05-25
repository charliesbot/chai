# Antigravity Support — One Pager

## What is Antigravity?

Google's agentic coding IDE, released late 2025. Runs on Gemini models and competes with Claude Code / Cursor. Ships with a fixed set of built-in agents ("Mission Control"); users can't define their own yet.

## Path comparison

|               | Claude Code                     | Gemini CLI                               | Antigravity                                            |
| ------------- | ------------------------------- | ---------------------------------------- | ------------------------------------------------------ |
| Instructions  | `~/.claude/CLAUDE.md`           | `~/.gemini/GEMINI.md`                    | `~/.gemini/GEMINI.md` **(shared w/ Gemini)**           |
| Skills dir    | `~/.claude/skills/`             | `~/.gemini/skills/`                      | `~/.gemini/antigravity-ide/skills/`, `~/.gemini/antigravity/skills/`, `~/.gemini/antigravity-cli/skills/` |
| Subagents dir | `~/.claude/agents/`             | `~/.gemini/agents/`                      | **not supported**                                      |
| MCP config    | `~/.claude.json` (`mcpServers`) | `~/.gemini/settings.json` (`mcpServers`) | matching `mcp_config.json` files under each Antigravity directory |

Sources: [antigravity.google/docs/mcp](https://antigravity.google/docs/mcp), [Antigravity Skills codelab](https://codelabs.developers.google.com/getting-started-with-antigravity-skills), [Google AI forum — custom subagents](https://discuss.ai.google.dev/t/antigravity-sub-agents/114381) (Google confirmed March 2026 that user-defined subagents are escalated as a feature request — not shipped).

## Per-area sync plan

### Instructions

Antigravity reads `~/.gemini/GEMINI.md`. Chai writes this file once even though the `antigravity` key expands to multiple Antigravity targets.

### Skills

Copy to `~/.gemini/antigravity-ide/skills/`, `~/.gemini/antigravity/skills/`, and `~/.gemini/antigravity-cli/skills/`. Same strategy as existing platforms: copy with hash-based dirty detection, chai owns only files it hashed.

Antigravity skill format is a _folder_ with `SKILL.md` (+ optional `scripts/`, `templates/`, `resources/`). This matches Claude Code's skill format (also a folder with `SKILL.md`), so no transformation — same copy strategy as `skills` today.

### Subagents

No-op. Antigravity has no user-defined subagents as of 2026-04. When Google ships the feature, add the destination dir. Document the skip in `chai sync` output ("antigravity: subagents not supported, skipping").

### MCP

Write `mcpServers` key to matching `mcp_config.json` files under `~/.gemini/antigravity-ide/`, `~/.gemini/antigravity/`, and `~/.gemini/antigravity-cli/`. Same `replace key` strategy chai uses for Claude/Gemini. The files may not exist on first sync — create them.

## The overlap problem

The Antigravity IDE and CLI targets all share `~/.gemini/GEMINI.md` for instructions, so one config key expands to multiple targets that write the same file.

**Options:**

1. **Detect and write once.** Sync logic dedupes destinations: if two platforms share a target, write once. Cleanest, but adds a special case.
2. **Make Antigravity a modifier, not a platform.** Treat `antigravity` as an extension of `gemini` — e.g. `gemini` platform gains Antigravity paths automatically. Smaller change, but surprising semantics (user enables `gemini`, gets Antigravity writes too).
3. **Let it double-write.** Second write is idempotent (same content). Hash DB handles it. Simplest, but dirty-detection prompts may fire twice for the same file.

**Recommendation: option 1.** Dedup destinations at sync time keyed by absolute path. The behavior is obvious ("same file, written once") and it generalizes if future platforms also share files. Implementation: after building the full `(platform, asset, dest)` list, group by `dest` and skip duplicates (warn in `--dry-run`).

## Platform definition

```go
{
    Key:              "antigravity",
    Name:             "Antigravity IDE",
    InstructionsPath: filepath.Join(".gemini", "GEMINI.md"),
    SkillsDir:        filepath.Join(".gemini", "antigravity-ide", "skills"),
    AgentsDir:        "", // not supported; sync skips empty dirs
    MCPConfigPath:    filepath.Join(".gemini", "antigravity-ide", "mcp_config.json"),
    MCPKey:           "mcpServers",
}
```

`AgentsDir: ""` needs a sentinel. Options: empty string = skip (current logic would need a guard), or a new `SupportsSubagents bool` field. Lean toward empty string + guard — one less field to plumb.

## Open questions

1. **Instructions file location** — is `GEMINI.md` really the right target, or should chai write both `GEMINI.md` and `AGENTS.md`? Antigravity v1.20.3+ reads both with `GEMINI.md` taking precedence. Picking one (GEMINI.md) is simpler. Defer `AGENTS.md` until someone asks.
2. **Project-level paths** — Antigravity also reads `<workspace>/.agent/skills/` and `<workspace>/GEMINI.md`. Out of scope for chai (which is user-level only today) but worth noting if chai ever adds project-level sync.
3. ~~**mcp_config.json may not exist** — first sync must create it.~~ Resolved: `mergeMCPIntoFile` already treats `os.IsNotExist` as "start with an empty map", and `atomicWrite` creates parent dirs. No work needed.

## Recommendation

Add Antigravity as a first-class platform key that expands to all known Antigravity user-level targets. The shared `GEMINI.md` is the only real design question — dedup by destination path and move on. Subagents are a future todo; skip with a log line for now.
