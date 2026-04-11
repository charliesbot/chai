# chai

Keep AI coding agent configs in sync. One manifest, distributed to every platform.

chai reads a single TOML file (`~/chai.toml`) and distributes instructions, skills, subagents, and MCP servers to Claude and Gemini. It copies what needs copying, symlinks what doesn't, and stays out of the way.

## Install

```bash
go install github.com/charliesbot/chai@latest
```

## Quick start

```bash
# Scaffold config
chai init

# Edit ~/chai.toml to your liking, then:
chai sync
```

`chai init` creates `~/chai.toml` and a starter directory with `instructions/AGENTS.md`, `skills/`, and `subagents/`.

## Config

Everything lives in `~/chai.toml`:

```toml
instructions = "~/dotfiles/ai/instructions/AGENTS.md"

[deps]
angular-skills = "https://github.com/angular/skills"

[deps.google-workspace]
url = "https://github.com/gemini-cli-extensions/workspace"
build = "npm install"

[skills]
paths = ["~/dotfiles/ai/skills", "@angular-skills", "@google-workspace/skills"]

[subagents]
paths = ["~/dotfiles/ai/subagents"]

[mcp.angular-cli]
command = "npx"
args = ["-y", "@angular/cli", "mcp"]

[mcp.gcloud]
command = "npx"
args = ["-y", "@google-cloud/gcloud-mcp"]

[mcp.google-workspace]
command = "node"
args = ["scripts/start.js"]
cwd = "@google-workspace"
```

### Sections

| Section | What it does |
|---------|-------------|
| `instructions` | Path to your AGENTS.md. Copied to `~/.claude/CLAUDE.md` and `~/.gemini/GEMINI.md`. |
| `[deps]` | Git repos cloned to `~/.chai/deps/`. Referenced as `@name` in other paths. |
| `[skills]` | Directories symlinked to each platform's skills folder. |
| `[subagents]` | Directories symlinked to `~/.claude/subagents/` and `~/.gemini/agents/`. |
| `[mcp.*]` | MCP server definitions written to each platform's config file. |

### Deps

Simple deps are just a URL:

```toml
[deps]
angular-skills = "https://github.com/angular/skills"
```

Deps that need a build step use a table:

```toml
[deps.google-workspace]
url = "https://github.com/gemini-cli-extensions/workspace"
build = "npm install"
```

The `build` command runs once on first clone (not on subsequent pulls). Reference any dep in other paths with `@name`.

### Path resolution

- `~` expands to your home directory
- `@name` resolves to `~/.chai/deps/<name>/`
- Pointing to a directory auto-expands to its contents (no trailing `/*` needed)

## Commands

### `chai init`

Scaffolds `~/chai.toml` and a starter directory structure. Skips files that already exist.

### `chai sync`

Distributes everything to Claude and Gemini:

```
$ chai sync
instructions  ● ◆
skills (6)  ● ◆
  agents-md, android-dev, slidev, web-dev, angular-developer, angular-new-app
mcpServers (3)  ● ◆
  angular-cli, gcloud, pencil
```

`●` = Claude, `◆` = Gemini.

| Flag | Description |
|------|-------------|
| `--dry-run` | Preview what would happen without writing files |
| `--force` | Skip dirty detection and overwrite everything |

### `chai update`

Clones new deps and pulls existing ones with a progress UI:

```
$ chai update
updating deps

  ✓ angular-skills  cloned
  ✓ google-workspace  cloned + built
```

## How it works

| What | Strategy | Why |
|------|----------|-----|
| Instructions | **Copy** with dirty detection | Agents may edit their platform copy |
| Skills | **Symlink** | Read-only from the agent's perspective |
| Subagents | **Symlink** | Read-only from the agent's perspective |
| MCP servers | **Replace key** in platform JSON | chai owns `mcpServers`, preserves everything else |

### Platform targets

| | Instructions | Skills | Subagents | MCP config |
|--|-------------|--------|-----------|------------|
| Claude | `~/.claude/CLAUDE.md` | `~/.claude/skills/` | `~/.claude/subagents/` | `~/.claude.json` |
| Gemini | `~/.gemini/GEMINI.md` | `~/.gemini/skills/` | `~/.gemini/agents/` | `~/.gemini/settings.json` |

## File structure

```
~/chai.toml                          <- your config
~/dotfiles/ai/                       <- your AI config (example)
  instructions/AGENTS.md
  skills/
    web-dev/
    android-dev/
  subagents/
    code-reviewer/

~/.chai/                             <- managed by chai
  hashes.json                        <- dirty detection
  deps/
    angular-skills/                  <- cloned repo
    google-workspace/                <- cloned repo (with npm install)
```

## License

MIT
