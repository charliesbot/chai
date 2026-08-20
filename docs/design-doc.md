# chai — design doc

## Goal

Keep AI coding agent configs in sync across tools. One manifest, distributed to every platform that needs it. Nothing more.

chai is deliberately minimal — it syncs config files, not manages workflows. It does one thing well and stays out of the way.

## Problem

Each AI coding agent expects config in different locations and formats. Keeping instructions, MCP servers, and skills consistent across all of them means editing multiple files every time something changes. Symlinks are fragile, non-portable, and can't transform content per platform.

## Solution

chai is a CLI tool that reads a single TOML manifest + an `AGENTS.md` file, and distributes them to the right locations per platform. Instructions, skills, and agents are copied with hash-based tracking so chai can remove stale managed files without touching user-created files.

## Core Principle

Minimal first. Nail the basics, then think about complexity.

## Core Concepts

- **Manifest** (`~/chai.toml`) — global config file that lives at `~`. Declares instructions path, deps, skills, agents, and MCP servers. All paths are absolute or use `~` / `@name`.
- **Instructions** — ordered source files for persistent agent instructions. Merged into one document and copied to each platform with a global instructions file. Agents may edit their platform copy, so dirty detection protects manual changes.
- **Skills** — reusable prompt/capability directories copied to each platform's skills directory. Chai tracks managed copies in the hash DB and leaves user-created files alone.
- **Agents** — subagent definitions. Copied to each platform's markdown subagent directory when supported.
- **Dependencies** — external repos that chai clones to `~/.chai/deps/`. Referenced in paths via `@name` prefix. Deps are clone-only — no magic, no manifest parsing. Updated explicitly via `chai update`, not during `chai sync`.
- **Platform definitions** — built into chai. Each definition describes where a platform expects its files. Users don't configure this.
- **Hash DB** — stores hashes of last-synced content per target file. Enables dirty detection before overwriting.

## Supported Platforms

- Claude
- Antigravity (IDE)
- Droid
- OpenCode
- Codex
- Cursor

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
├── deps/
│   └── workspace/
└── sources/github.com/
    └── owner/repository/    <- sparse public GitHub skill cache
```

## TOML Schema

```toml
instructions = [
  "~/dotfiles/ai/instructions/AGENTS.md",
  "~/dotfiles/ai/instructions/ADHD.md",
]

[deps]
workspace = "https://github.com/gemini-cli-extensions/workspace"
angular-skills = "https://github.com/angular/skills"

[skills]
local = ["~/dotfiles/ai/skills"]

[[skills.github]]
url = "https://github.com/vercel-labs/agent-skills"
include = ["frontend-design", "skill-creator"]

[subagents]
paths = ["~/dotfiles/ai/subagents/*"]

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

- `instructions` — ordered instruction file paths. Their contents are joined with one blank line and copied to each platform with a global instructions file.
- `[deps]` — external repos to clone. `name = "url"`. Cloned to `~/.chai/deps/<name>/`. Deps are clone-only — chai doesn't read or parse their contents. Only cloned/pulled via `chai update`, not during `chai sync`.
- `[skills].local` — local skill directories or collections. Collections discover valid `SKILL.md` files in immediate child directories. Names come from frontmatter; globs and dependency references are rejected.
- `[[skills.github]]` — canonical public GitHub sources with explicit selected skill names. Chai caches only selected trees; `chai sync` reads the cache offline and `chai update` refreshes it.
- `[subagents]` — markdown subagent definitions. Same path resolution as skills; copied to markdown-native platforms and compiled to TOML for Codex.
- `[mcp.<name>]` — MCP server definitions. `command`, `args`, optional `env` and `cwd`. The section name becomes the key in the platform's `mcpServers` object. Use `@name` in `cwd` to reference a dep's local path. NPX-based MCPs don't need a `[deps]` entry.
- `[[droid.custom_models]]` — Droid BYOK model definitions. Written to `~/.factory/settings.json` as `customModels`, preserving unrelated settings.

### Path Resolution

- Local skill paths are absolute, use `~`, or are explicitly relative with `./` or `../`.
- `~` expands to the user's home directory.
- Relative local skill paths resolve from the directory containing `chai.toml`.
- `@name` references remain available for dependency-backed runtime paths such as MCP `cwd`, not for skills.

## Platform Definitions

Defined in chai's source code, not by the user. Each platform specifies where files go and how MCP servers are registered.

| Platform | Instructions destination | Skills directory      | Subagents directory | MCP config file              | MCP key        | MCP strategy |
|----------|--------------------------|----------------------|---------------------|------------------------------|----------------|--------------|
| Claude   | `~/.claude/CLAUDE.md`    | `~/.claude/skills/`  | `~/.claude/agents/` | `~/.claude.json`             | `mcpServers`   | replace key  |
| Antigravity | `~/.gemini/GEMINI.md` | `~/.gemini/config/skills/` | _none_ | `~/.gemini/config/mcp_config.json` | `mcpServers` | replace key |
| Droid    | `~/.factory/AGENTS.md`   | `~/.factory/skills/` | `~/.factory/droids/` | `~/.factory/mcp.json`       | `mcpServers`   | replace key with Droid stdio entries |
| OpenCode | `~/.config/opencode/AGENTS.md` | `~/.config/opencode/skills/` | `~/.config/opencode/agents/` | `~/.config/opencode/opencode.json` | `mcp` | replace key with OpenCode entries |
| Codex    | `~/.codex/AGENTS.md`     | `~/.agents/skills/`  | `~/.codex/agents/` _(compiled TOML)_ | `~/.codex/config.toml` | `mcp_servers` | replace TOML table |
| Cursor   | _none_                   | `~/.cursor/skills/`  | `~/.cursor/agents/` | `~/.cursor/mcp.json` | `mcpServers` | replace key with Cursor stdio entries |

- Instructions are **merged in declaration order** and copied where the platform has a global instructions file (agents may edit their platform copy — dirty detection protects manual changes).
- Skills and agents are **copied** and tracked so stale chai-managed files can be removed without touching user-created files.
- MCPs are transformed per platform when needed. Droid uses `type: "stdio"`, `command`, `args`, `env`, and `disabled: false` in `~/.factory/mcp.json`. Cursor uses `type: "stdio"`, `command`, `args`, and `env` in `~/.cursor/mcp.json`.

### MCP Write Strategy

Chai owns the platform MCP key completely. The TOML is the source of truth.

1. Read existing config file (if any).
2. Replace the entire platform MCP key with all resolved MCP definitions.
3. Preserve all other keys in the file untouched.
4. Write the file back.

## CLI

### `chai init`

Scaffolds a `~/chai.toml` and an AI folder with `instructions/AGENTS.md`, `skills/`, and `agents/` directories. Skips any files or directories that already exist.

### `chai sync`

Distributes config to all platforms. Does **not** touch deps — uses whatever is already cloned.

1. Read `~/chai.toml`.
2. Discover local skills and resolve selected GitHub skills from the offline source cache.
3. Resolve remaining paths and expand subagent globs.
4. Hash managed instructions and skill directories for dirty detection.
5. Copy instructions to platform locations (with dirty detection prompts).
6. Copy skills and agents to platform directories (remove stale chai-managed copies first).
7. Replace the platform MCP key in platform config files.
8. Merge Droid custom models into `~/.factory/settings.json` if configured.
9. Update hash DB.

Flags: `--force` (skip dirty checks), `--dry-run` (preview without writing).

### `chai update`

Refreshes public GitHub skill sources, clones or pulls deps, installs Antigravity plugins, then syncs updated skills.

1. Require Git 2.37 or newer when `[[skills.github]]` is configured.
2. Fetch each remote source, rediscover selected names, and atomically replace its sparse cache.
3. Report newly available upstream skills without selecting them.
4. For each dep: clone if missing, pull if already cloned.
5. For each plugin (when `antigravity` is in `platforms`): run `agy plugin install <url>`; treat `"already installed"` as up to date.
6. Run the normal offline sync after source updates succeed.

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
|   hash    |  compare stored hashes for instructions and skills
+-----+-----+
     |
     v
+-----------+
|   write   |  copy instructions/skills/agents, replace MCP config
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
|  sources  |  refresh selected sparse trees → ~/.chai/sources/
+-----------+
     |
     v
+-----------+
| deps/plugins |  clone, pull, install
+-----------+
     |
     v
+-----------+
|   sync    |  distribute refreshed skills
+-----------+
```

## Hash / Dirty Detection

- Dirty detection applies to instructions and skills. Agents use the hash DB for stale ownership tracking. MCPs are fully owned by chai and replaced on every sync.
- On every sync, chai hashes managed content (MD5) and stores it in `~/.chai/hashes.json`.
- Before replacing instructions or skill directories, chai hashes the managed destination and compares it with the stored hash.
- Match = file untouched since last sync, safe to overwrite.
- Mismatch = file was manually edited, prompt the user via Bubbletea TUI before overwriting.
- Missing hash = first sync for this target, just write.
- `chai sync --force` skips dirty checks for chai-managed targets; it never overwrites an unmanaged same-name skill.

## Tech Stack

- Go
- Bubbletea for TUI (dirty detection prompts, sync output)
- TOML for config (`pelletier/go-toml`)
- Distributed as a single binary (Homebrew, GitHub releases)

## Resolved Questions

- **How do skills map to platforms?** — Copied to each platform's configured skills directory. Codex uses `~/.agents/skills/`. Droid uses `~/.factory/skills/`.
- **Why copies instead of symlinks?** — Copies are portable across platforms and allow chai to use hash-based tracking for stale managed content while leaving user-created files alone.
- **Why separate `chai sync` from `chai update`?** — Sync should be fast and predictable. Pulling git repos is slow and network-dependent. Users update deps explicitly when they want to.
- **Do NPX-based MCPs need deps?** — No. NPX fetches the package on the fly. `[deps]` is only needed when an MCP or another runtime tool needs actual repository files on disk.

## Open Questions

- Should `chai update` support pinning deps to a specific commit/tag?

## Future Features

- **`dep = "@name"` shorthand for MCPs** — read dep manifests and extract MCP definitions automatically instead of manual inline config.
- **Hooks** — `[claude.hooks]` and per-platform sections that write to each platform's `settings.json` under the `hooks` key. Same replace-key strategy as MCPs. Event names differ per platform (`PreToolUse` vs `BeforeTool`, etc.) so no abstraction — platform-specific sections.
- **Project-level config** — a `chai.toml` in a project root for project-scoped instructions, skills, and MCPs. Running `chai sync` from within a project directory detects the local `chai.toml` and generates platform-specific files:

  | Platform | Instructions         | MCP config          | Notes                                      |
  |----------|----------------------|---------------------|--------------------------------------------|
  | Claude   | `CLAUDE.md`          | `.mcp.json`         | Both supported at project level             |
  | Droid    | `.factory/AGENTS.md` | `.factory/mcp.json` | Project-level config is natively supported by Droid |

  The project `chai.toml` uses the same schema as the global one. Global config (`~/chai.toml`) is not merged — project config is standalone.
