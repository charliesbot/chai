# AGENTS.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

chai is a Go CLI that reads a single TOML manifest (`~/chai.toml`) and an `agents.md` file, then distributes them to the right locations for each AI platform (Claude, Gemini). It copies files (not symlinks) and uses hash-based dirty detection to avoid overwriting manual edits.

## Tech Stack

- Go
- Bubbletea for TUI (dirty detection prompts, sync output)
- TOML config parsing (`pelletier/go-toml`)
- Distributed as a single binary

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

The CLI has two commands: `chai init` (scaffold config) and `chai sync` (distribute files to platforms).

### Sync Flow

1. Read `~/chai.toml`
2. Clone/pull all `[deps]` to `~/.chai/deps/`
3. Resolve paths (`~`, `@name` for deps) and expand globs
4. Hash target files and compare against `~/.chai/hashes.json` for dirty detection
5. Write files: copy instructions to platform locations, replace `mcpServers` key in platform configs

### Platform Definitions

Built into source code (not user-configured). Each platform specifies:

| Platform | Instructions target      | MCP config file           | MCP strategy  |
|----------|--------------------------|---------------------------|---------------|
| Claude   | `~/.claude/CLAUDE.md`    | `~/.claude.json`          | replace key   |
| Gemini   | `~/.gemini/GEMINI.md`    | `~/.gemini/settings.json` | replace key   |

### Key Design Decisions

- **Copy, not symlink** — portable and allows per-platform transformation in the future.
- **Hash-based dirty detection** — only applies to instructions files. Skills and MCPs are fully owned by chai and replaced on every sync.
- **`mcpServers` ownership** — chai owns the entire `mcpServers` key in platform config files. It replaces the key wholesale but preserves all other keys.
- **Deps are clone-only** — chai clones repos to `~/.chai/deps/<name>/` but does not parse or inspect their contents.
- **Path resolution** — `~` expands to home dir, `@name` resolves to `~/.chai/deps/<name>/`.

## Design Doc

See `docs/design-doc.md` for the full specification including TOML schema, open questions, and future features.
