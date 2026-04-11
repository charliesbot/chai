# chai — design doc

## Goal

Keep AI coding agent configs in sync across tools. One manifest, distributed to every platform that needs it. Nothing more.

chai is deliberately minimal — it syncs config files, not manages workflows. It does one thing well and stays out of the way.

## Problem

Each AI coding agent expects config in different locations and formats. Keeping instructions, MCP servers, and skills consistent across all of them means editing multiple files every time something changes. Symlinks are fragile, non-portable, and can't transform content per platform.

## Solution

chai is a CLI tool that reads a single TOML manifest + an `AGENTS.md` file, and distributes them to the right locations per platform. Instructions are copied (with hash-based dirty detection), while skills and agents are symlinked since they're read-only from the agent's perspective.

## Core Principle

Minimal first. Nail the basics, then think about complexity.

## Core Concepts

- **Manifest** (`~/chai.toml`) — global config file that lives at `~`. Declares instructions path, deps, skills, agents, and MCP servers. All paths are absolute or use `~` / `@name`.
- **Instructions** (`AGENTS.md`) — single source of truth for agent instructions (persistent instructions). Copied to each platform's expected file. Agents may edit their platform copy, so dirty detection protects manual changes.
- **Skills** — read-only prompt files that agents consume but never modify. Symlinked (not copied) to each platform's skills directory. Chai owns these symlinks completely.
- **Agents** — subagent definitions. Same symlink strategy as skills.
- **Dependencies** — external repos that chai clones to `~/.chai/deps/`. Referenced in paths via `@name` prefix. Deps are clone-only — no magic, no manifest parsing. Updated explicitly via `chai update`, not during `chai sync`.
- **Platform definitions** — built into chai. Each definition describes where a platform expects its files. Users don't configure this.
- **Hash DB** — stores hashes of last-synced content per target file. Enables dirty detection before overwriting.

## Supported Platforms

- Claude
- Gemini

## Expected Folder Structure

```
~/chai.toml                  <- global config, always at ~
~/dotfiles/ai/               <- user's AI config (example)
├── instructions/
│   └── AGENTS.md
├── skills/
│   ├── web-dev/
│   ├── android-dev/
│   └── slidev/
└── agents/
    └── code-reviewer/
```

Dependencies are cloned to:

```
~/.chai/
├── hashes.json              <- hash DB for dirty detection
└── deps/
    ├── workspace/
    └── angular-skills/
```

## TOML Schema

```toml
instructions = "~/dotfiles/ai/instructions/AGENTS.md"

[deps]
workspace = "https://github.com/gemini-cli-extensions/workspace"
angular-skills = "https://github.com/angular/skills"

[skills]
paths = [
  "~/dotfiles/ai/skills/*",
  "@workspace/skills/*",
  "@angular-skills/skills/*"
]

[agents]
paths = ["~/dotfiles/ai/agents/*"]

[mcp.context7]
command = "npx"
args = ["-y", "@upstash/context7-mcp", "--api-key", "${CONTEXT7_API_KEY}"]

[mcp.google-workspace]
command = "node"
args = ["scripts/start.js"]
cwd = "@workspace"

[mcp.angular-cli]
command = "npx"
args = ["-y", "@angular/cli", "mcp"]

[mcp.gcloud]
command = "npx"
args = ["-y", "@google-cloud/gcloud-mcp"]

[mcp.pencil]
command = "/Applications/Pencil.app/Contents/Resources/app.asar.unpacked/out/mcp-server-darwin-arm64"
args = ["--app", "desktop"]
```

- `instructions` — path to AGENTS.md. Copied to each platform's expected location.
- `[deps]` — external repos to clone. `name = "url"`. Cloned to `~/.chai/deps/<name>/`. Deps are clone-only — chai doesn't read or parse their contents. Only cloned/pulled via `chai update`, not during `chai sync`.
- `[skills]` — skill directories. Supports globs, external paths (`~/`), and dep references (`@name/`). Symlinked to each platform's skills directory.
- `[agents]` — subagent definitions. Same path resolution and symlink strategy as skills.
- `[mcp.<name>]` — MCP server definitions. `command`, `args`, optional `env` and `cwd`. The section name becomes the key in the platform's `mcpServers` object. Use `@name` in `cwd` to reference a dep's local path. NPX-based MCPs don't need a `[deps]` entry.

### Path Resolution

- All paths are absolute or use `~` (home directory).
- `~` expands to the user's home directory.
- `@name` references a cloned dep (e.g., `@workspace/skills/*` -> `~/.chai/deps/workspace/skills/*`).
- Globs are expanded at sync time.

## Platform Definitions

Defined in chai's source code, not by the user. Each platform specifies where files go and how MCP servers are registered.

| Platform | Instructions destination | Skills directory      | MCP config file              | MCP key        | MCP strategy |
|----------|--------------------------|----------------------|------------------------------|----------------|--------------|
| Claude   | `~/.claude/CLAUDE.md`    | `~/.claude/skills/`  | `~/.claude.json`             | `mcpServers`   | replace key  |
| Gemini   | `~/.gemini/GEMINI.md`    | `~/.gemini/skills/`  | `~/.gemini/settings.json`    | `mcpServers`   | replace key  |

- Instructions are **copied** (agents may edit their platform copy — dirty detection protects manual changes).
- Skills and agents are **symlinked** (read-only from the agent's perspective — one source of truth, no duplication).
- MCPs use the same `mcpServers` structure (`command`, `args`, `env`) on both platforms — no transformation needed.

### MCP Write Strategy

Chai owns the `mcpServers` key completely. The TOML is the source of truth.

1. Read existing config file (if any).
2. Replace the entire `mcpServers` key with all resolved MCP definitions.
3. Preserve all other keys in the file untouched.
4. Write the file back.

## CLI

### `chai init`

Scaffolds a `~/chai.toml` and an AI folder with `instructions/AGENTS.md`, `skills/`, and `agents/` directories. Skips any files or directories that already exist.

### `chai sync`

Distributes config to all platforms. Does **not** touch deps — uses whatever is already cloned.

1. Read `~/chai.toml`.
2. Resolve all paths and expand globs for skills, agents, MCPs.
3. Hash target instructions files and compare against stored hashes for dirty detection.
4. Copy instructions to platform locations (with dirty detection prompts).
5. Symlink skills and agents to platform directories (remove stale symlinks first).
6. Replace `mcpServers` key in platform config files.
7. Update hash DB.

Flags: `--force` (skip dirty checks), `--dry-run` (preview without writing).

### `chai update`

Clones missing deps and pulls existing ones. Shows Bubbletea progress UI with per-dep status.

1. Read `[deps]` from `~/chai.toml`.
2. For each dep: clone if missing, pull if already cloned.
3. Display progress bars and status per dependency.

## Sync Flow

```
~/chai.toml
     |
     v
+-----------+
|   parse   |  read TOML, resolve paths, expand globs
+-----+-----+
     |
     v
+-----------+
|   hash    |  compare source hash vs stored hash for instructions
+-----+-----+
     |
     v
+-----------+
|   write   |  copy instructions, symlink skills/agents, replace mcpServers
+-----+-----+
     |
     v
+-----------+
|   save    |  update hash DB
+-----------+
```

## Update Flow

```
~/chai.toml
     |
     v
+-----------+
|   deps    |  clone missing, pull existing → ~/.chai/deps/
+-----------+
     |
     v
  progress UI (Bubbletea)
```

## Hash / Dirty Detection

- Dirty detection applies only to instructions (`AGENTS.md` → `CLAUDE.md` / `GEMINI.md`). Skills, agents, and MCPs are fully owned by chai and replaced on every sync.
- On every sync, chai hashes the `instructions` file (MD5) and stores it in `~/.chai/hashes.json`.
- Before writing, chai hashes each target file on disk (`CLAUDE.md`, `GEMINI.md`) and compares against the stored hash.
- Match = file untouched since last sync, safe to overwrite.
- Mismatch = file was manually edited, prompt the user via Bubbletea TUI before overwriting.
- Missing hash = first sync for this target, just write.
- `chai sync --force` skips all dirty checks and overwrites everything.

## Tech Stack

- Go
- Bubbletea for TUI (dirty detection prompts, sync output)
- TOML for config (`pelletier/go-toml`)
- Distributed as a single binary (Homebrew, GitHub releases)

## Resolved Questions

- **How do skills map to platforms?** — Symlinked to `~/.claude/skills/<name>` and `~/.gemini/skills/<name>`. Agents use the same pattern.
- **Why symlinks for skills but copies for instructions?** — Instructions are two-way: agents may edit their platform copy (e.g., Claude edits CLAUDE.md based on project context). Skills are read-only from the agent's perspective — they consume them but never modify them. Symlinks give one source of truth with no duplication.
- **Why separate `chai sync` from `chai update`?** — Sync should be fast and predictable. Pulling git repos is slow and network-dependent. Users update deps explicitly when they want to.
- **Do NPX-based MCPs need deps?** — No. NPX fetches the package on the fly. `[deps]` is only needed when you need actual files on disk (for skills, agents, or MCPs that run from a local path).

## Open Questions

- Should `chai update` support pinning deps to a specific commit/tag?

## Future Features

- **`dep = "@name"` shorthand for MCPs** — read dep manifests and extract MCP definitions automatically instead of manual inline config.
- **Hooks** — `[claude.hooks]` and `[gemini.hooks]` sections that write to each platform's `settings.json` under the `hooks` key. Same replace-key strategy as MCPs. Event names differ per platform (`PreToolUse` vs `BeforeTool`, etc.) so no abstraction — platform-specific sections.
- **Project-level config** — a `chai.toml` in a project root for project-scoped instructions, skills, and MCPs. Running `chai sync` from within a project directory detects the local `chai.toml` and generates platform-specific files:

  | Platform | Instructions         | MCP config          | Notes                                      |
  |----------|----------------------|---------------------|--------------------------------------------|
  | Claude   | `CLAUDE.md`          | `.mcp.json`         | Both supported at project level             |
  | Gemini   | `GEMINI.md`          | `.gemini/settings.json` | Needs verification — project-level MCP support unclear |

  The project `chai.toml` uses the same schema as the global one. Global config (`~/chai.toml`) is not merged — project config is standalone.
