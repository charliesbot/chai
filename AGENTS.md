# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

chai is a Go CLI that keeps AI coding agent configs in sync. It reads a single TOML manifest (`~/chai.toml`) and an `AGENTS.md` file, then distributes them to the right locations for each AI platform (Claude, Gemini, Droid, OpenCode, Codex). Instructions, skills, and subagents are copied with hash-based dirty detection so chai can prompt before overwriting files an agent has edited.

chai is deliberately minimal — it syncs config files, not manages workflows.

## Tech Stack

- **Go** — single binary distribution
- **ffcli** (`github.com/peterbourgon/ff/v3/ffcli`) — CLI framework, uses stdlib `flag` for arg parsing
- **Bubbletea** (`github.com/charmbracelet/bubbletea`) — TUI for interactive prompts (dirty detection, update progress)
- **Lipgloss** (`github.com/charmbracelet/lipgloss`) — terminal styling (colors, bold, box-drawing)
- **go-toml** (`github.com/pelletier/go-toml`) — TOML config parsing

### Why ffcli over Cobra/Kong

chai only has three commands (`init`, `sync`, `update`) and a few flags. ffcli wraps stdlib `flag` with minimal abstraction — no codegen, no hidden behavior, easy to read the entire command wiring at a glance. Bubbletea is launched from within ffcli's `Exec` functions for interactive parts.

Reference project for ffcli patterns: https://github.com/rudrankriyam/App-Store-Connect-CLI

## Build & Run

```bash
go build -o chai .
go run . <command>
```

## Test

```bash
go test ./...               # all tests
go test ./internal/sync     # single package
go test -run TestHashDB ./internal/hash  # single test
```

## Architecture

The CLI has three commands: `chai init` (scaffold config), `chai sync` (distribute files to platforms), and `chai update` (clone/pull deps).

### Sync Flow

1. Read `~/chai.toml`
2. Resolve paths (`~`, `@name` for deps) and expand globs
3. Hash target instructions, skills, and subagents files and compare against `~/.chai/hashes.json` for dirty detection
4. Copy instructions to platform locations (with dirty detection prompts)
5. Copy skills and subagents to platform directories (with dirty detection prompts)
6. Replace `mcpServers` key in platform configs
7. Display Gemini extensions status
8. Update hash DB

Deps and Gemini extensions are managed separately via `chai update` (clone/pull deps, install extensions).

All file writes must be atomic (write to `.tmp`, then `os.Rename`).

### Platform Definitions

Built into source code (not user-configured). Each platform specifies:

| Platform | Instructions target               | Skills directory                   | Subagents directory                | MCP config file                       | MCP strategy  |
|----------|-----------------------------------|------------------------------------|------------------------------------|---------------------------------------|---------------|
| Claude   | `~/.claude/CLAUDE.md`             | `~/.claude/skills/`                | `~/.claude/agents/`                | `~/.claude.json`                      | replace key   |
| Gemini   | `~/.gemini/GEMINI.md`             | `~/.agents/skills/` _(shared)_     | `~/.gemini/agents/`                | `~/.gemini/settings.json`             | replace key   |
| Droid    | `~/.factory/AGENTS.md`            | `~/.factory/skills/`               | `~/.factory/droids/`               | `~/.factory/mcp.json`                 | replace `mcpServers` with Droid stdio entries |
| OpenCode | `~/.config/opencode/AGENTS.md`    | `~/.config/opencode/skills/`       | `~/.config/opencode/agents/`       | `~/.config/opencode/opencode.json`    | replace `mcp` (OpenCode-format entries) |
| Codex    | `~/.codex/AGENTS.md`              | `~/.agents/skills/`                | _none_                             | `~/.codex/config.toml`                | replace `mcp_servers` table (TOML, no cwd) |

### Key Design Decisions

- **Copy with dirty detection for instructions, skills, and subagents** — agents may edit any of these in place, so chai hashes each file on write and compares before the next sync, prompting on conflict rather than clobbering.
- **Hash-based dirty detection** — applies to instructions, skills, and subagents. MCPs are fully owned by chai and replaced on every sync.
- **`mcpServers` ownership** — chai owns the entire `mcpServers` key in platform config files. It replaces the key wholesale but preserves all other keys.
- **Deps are clone-only** — chai clones repos to `~/.chai/deps/<name>/` but does not parse or inspect their contents. Deps with a `build` field run the build command on first clone.
- **Sync doesn't touch deps** — `chai sync` is fast and predictable. `chai update` handles cloning/pulling deps and installing Gemini extensions.
- **Gemini extensions** — some tools only work as Gemini extensions (e.g., they rely on Gemini's OAuth). Declared under `[gemini.extensions]`, installed via `gemini extensions install`.
- **Path resolution** — `~` expands to home dir, `@name` resolves to `~/.chai/deps/<name>/`. Works in skill paths, subagent paths, and MCP `args`/`cwd`.
- **Atomic writes** — all file writes go through a temp file + `os.Rename` to prevent partial writes.

## Implementation Phases

Work is tracked in GitHub issues. Build in this order — each phase produces something that works:

### Phase 1 — Foundation
- #1 Project scaffolding (Go module, ffcli, main.go)
- #2 TOML config parsing
- #3 Path resolution (~, @name, globs)

### Phase 2 — Core Sync
- #4 `chai init` (scaffold config + agents.md)
- #5 Instructions sync (copy agents.md to platform locations)
- #6 MCP sync (write mcpServers to platform config files)

### Phase 3 — Dependencies
- #7 Deps clone/pull

### Phase 4 — Dirty Detection
- #8 Hash DB and dirty detection
- #9 TUI prompts for conflicts (Bubbletea)

### Phase 5 — Fast Follows
- #10 `--dry-run` flag
- #11 SIGINT cleanup

## Design Doc

See `docs/design-doc.md` for the full specification including TOML schema, open questions, and future features.
