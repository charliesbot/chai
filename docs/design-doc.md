# chai — design doc

## Problem

AI coding agents (Claude, Gemini, etc.) each expect config in different locations and formats. Maintaining parallel files manually is tedious and error-prone. Symlinks are fragile, non-portable, and can't transform content per platform.

## Solution

chai is a CLI tool that reads a single TOML manifest + an `agents.md` file, and distributes them to the right locations per platform. It copies files (not symlinks) and uses hash-based dirty detection to avoid overwriting manual edits.

## Core Principle

Minimal first. Nail the basics, then think about complexity.

## Core Concepts

- **Manifest** (`~/chai.toml`) — global config file that lives at `~`. Declares instructions path, deps, skills, agents, and MCP servers. All paths are absolute or use `~` / `@name`.
- **Instructions** (`agents.md`) — single source of truth for agent instructions (persistent instructions). Copied to each platform's expected file.
- **Dependencies** — external repos that chai clones to `~/.chai/deps/`. Referenced in paths via `@name` prefix. Deps are clone-only — no magic, no manifest parsing.
- **Platform definitions** — built into chai. Each definition describes where a platform expects its files. Users don't configure this.
- **Hash DB** — stores hashes of last-synced content per target file. Enables dirty detection before overwriting.

## Supported Platforms

- Claude
- Gemini

## Expected Folder Structure

```
~/chai.toml                  <- global config, always at ~
~/dotfiles/ai/               <- user's AI config (example)
├── agents.md
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
instructions = "~/dotfiles/ai/agents.md"

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

- `instructions` — path to the agents.md file. Copied to each platform's expected location.
- `[deps]` — external repos to clone. `name = "url"`. Cloned to `~/.chai/deps/<name>/`. Deps are clone-only — chai doesn't read or parse their contents.
- `[skills]` — skill directories. Supports globs, external paths (`~/`), and dep references (`@name/`).
- `[agents]` — subagent definitions. Same path resolution as skills.
- `[mcp.<name>]` — MCP server definitions. `command`, `args`, optional `env` and `cwd`. The section name becomes the key in the platform's `mcpServers` object. Use `@name` in `cwd` to reference a dep's local path.

### Path Resolution

- All paths are absolute or use `~` (home directory).
- `~` expands to the user's home directory.
- `@name` references a cloned dep (e.g., `@workspace/skills/*` -> `~/.chai/deps/workspace/skills/*`).
- Globs are expanded at sync time.

## Platform Definitions

Defined in chai's source code, not by the user. Each platform specifies where files go and how MCP servers are registered.

| Platform | Instructions destination | MCP config file              | MCP key        | MCP strategy |
|----------|--------------------------|------------------------------|----------------|--------------|
| Claude   | `~/.claude/CLAUDE.md`    | `~/.claude.json`             | `mcpServers`   | replace key  |
| Gemini   | `~/.gemini/GEMINI.md`    | `~/.gemini/settings.json`    | `mcpServers`   | replace key  |

Both platforms use the same `mcpServers` structure (`command`, `args`, `env`), so chai writes the same MCP data to both — no transformation needed.

### MCP Write Strategy

Chai owns the `mcpServers` key completely. The TOML is the source of truth.

1. Read existing config file (if any).
2. Replace the entire `mcpServers` key with all resolved MCP definitions.
3. Preserve all other keys in the file untouched.
4. Write the file back.

## CLI

### `chai init`

Scaffolds a `~/chai.toml` and an AI folder with `agents.md` in the specified path.

### `chai sync`

1. Read `~/chai.toml`.
2. Clone or pull all `[deps]` to `~/.chai/deps/`.
3. Resolve all paths and expand globs for skills, agents, MCPs.
4. For each target file: hash the content that would be written.
5. Compare against stored hash from last sync.
6. If target hash != stored hash, the file was manually edited — warn the user.
7. Delete existing target files.
8. Copy fresh files to target paths.
9. Replace `mcpServers` key in platform config files.
10. Update hash DB.

## Sync Flow

```
~/chai.toml
     |
     v
+-----------+
|   deps    |  clone/pull repos to ~/.chai/deps/
+-----+-----+
     |
     v
+-----------+
|   parse   |  read TOML, resolve paths, expand globs
+-----+-----+
     |
     v
+-----------+
|   hash    |  compare source hash vs stored hash per target
+-----+-----+
     |
     v
+-----------+
|   write   |  delete old files, copy fresh, replace mcpServers, update hash DB
+-----------+
```

## Hash / Dirty Detection

- Dirty detection applies only to instructions (`agents.md` → `CLAUDE.md` / `GEMINI.md`). Skills and MCPs are fully owned by chai and replaced on every sync.
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

## Open Questions

- How exactly do skills map to each platform's expected directory structure?
- Where do skills get copied to per platform? (`~/.claude/skills/`? `~/.gemini/skills/`?)

- Should `chai sync` run `npm install` or setup steps for deps that need it (e.g., workspace extension uses `node scripts/start.js`)?

## Future Features

- **`dep = "@name"` shorthand for MCPs** — read dep manifests and extract MCP definitions automatically instead of manual inline config.
- **Hooks** — `[claude.hooks]` and `[gemini.hooks]` sections that write to each platform's `settings.json` under the `hooks` key. Same replace-key strategy as MCPs. Event names differ per platform (`PreToolUse` vs `BeforeTool`, etc.) so no abstraction — platform-specific sections.
- **Project-level config** — a `chai.project.toml` in a project root for project-scoped instructions, skills, and MCPs. `chai sync --project` from within a project directory generates `.mcp.json` (Claude), `.gemini/settings.json` (Gemini), project-level `CLAUDE.md` / `GEMINI.md`, and `.claude/skills/` per project.
