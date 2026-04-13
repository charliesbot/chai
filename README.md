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

# Clone deps and install extensions
chai update

# Distribute config to all platforms
chai sync
```

`chai init` creates `~/chai.toml` with a starter config. It doesn't create any directories — the paths in the TOML are resolved at sync time.

## Config

Everything lives in `~/chai.toml`:

```toml
instructions = "~/dotfiles/ai/instructions/AGENTS.md"

[deps]
angular-skills = "https://github.com/angular/skills"

[deps.some-tool]
url = "https://github.com/example/tool"
build = "npm install"

[skills]
paths = ["~/dotfiles/ai/skills", "@angular-skills"]

[subagents]
paths = ["~/dotfiles/ai/subagents"]

[mcp.angular-cli]
command = "npx"
args = ["-y", "@angular/cli", "mcp"]

[mcp.gcloud]
command = "npx"
args = ["-y", "@google-cloud/gcloud-mcp"]

[gemini.extensions]
workspace = "https://github.com/gemini-cli-extensions/workspace"
```

### Sections

| Section              | What it does                                                                       |
| -------------------- | ---------------------------------------------------------------------------------- |
| `instructions`       | Path to your AGENTS.md. Copied to `~/.claude/CLAUDE.md` and `~/.gemini/GEMINI.md`. |
| `[deps]`             | Git repos cloned to `~/.chai/deps/`. Referenced as `@name` in other paths.         |
| `[skills]`           | Directories symlinked to each platform's skills folder.                            |
| `[subagents]`        | Directories symlinked to `~/.claude/subagents/` and `~/.gemini/agents/`.           |
| `[mcp.*]`            | MCP server definitions written to each platform's config file.                     |
| `[gemini.extensions]`| Gemini CLI extensions installed via `gemini extensions install`.                    |

### Deps

Simple deps are just a URL:

```toml
[deps]
angular-skills = "https://github.com/angular/skills"
```

Deps that need a build step use a table:

```toml
[deps.some-tool]
url = "https://github.com/example/tool"
build = "npm install"
```

The `build` command runs once on first clone (not on subsequent pulls). Reference any dep in other paths with `@name`.

### Gemini extensions

Some tools only work as Gemini extensions (e.g., they rely on Gemini's OAuth). Declare them under `[gemini.extensions]`:

```toml
[gemini.extensions]
workspace = "https://github.com/gemini-cli-extensions/workspace"
```

`chai update` installs them via `gemini extensions install`. They appear in `chai sync` output with `· ◆` (Gemini-only).

### Path resolution

- `~` expands to your home directory
- `@name` resolves to `~/.chai/deps/<name>/` — works in skill paths, subagent paths, and MCP `args`/`cwd`
- Pointing to a directory auto-expands to its contents (no trailing `/*` needed)

## Commands

### `chai init`

Creates `~/chai.toml` with a starter config. Skips if it already exists.

### `chai sync`

Distributes everything to Claude and Gemini:

```
$ chai sync
 ┌ instructions ──────────────────────────── ● ◆
 │ ~/dotfiles/ai/instructions/AGENTS.md
 │ → ~/.claude/CLAUDE.md
 │ → ~/.gemini/GEMINI.md
 └
 ┌ skills (6) ────────────────────────────── ● ◆
 │ agents-md  android-dev  slidev  web-dev
 │ angular-developer  angular-new-app
 └
 ┌ mcpServers (3) ────────────────────────── ● ◆
 │ angular-cli  gcloud  pencil
 └
 ┌ gemini extensions (1) ─────────────────── · ◆
 │ workspace
 └
```

`●` = Claude, `◆` = Gemini, `·` = not applicable.

| Flag        | Description                                     |
| ----------- | ----------------------------------------------- |
| `--dry-run` | Preview what would happen without writing files |
| `--force`   | Skip dirty detection and overwrite everything   |

### `chai update`

Clones deps, runs builds, and installs Gemini extensions:

```
$ chai update
deps

  ✓ angular-skills  cloned
    https://github.com/angular/skills

gemini extensions

  ✓ workspace  installed
    https://github.com/gemini-cli-extensions/workspace
```

## How it works

| What              | Strategy                         | Why                                               |
| ----------------- | -------------------------------- | ------------------------------------------------- |
| Instructions      | **Copy** with dirty detection    | Agents may edit their platform copy               |
| Skills            | **Symlink**                      | Read-only from the agent's perspective            |
| Subagents         | **Symlink**                      | Read-only from the agent's perspective            |
| MCP servers       | **Replace key** in platform JSON | chai owns `mcpServers`, preserves everything else |
| Gemini extensions | **gemini extensions install**    | Gemini-only, uses Gemini's own auth and runtime   |

### Platform targets

|        | Instructions          | Skills              | Subagents              | MCP config                |
| ------ | --------------------- | ------------------- | ---------------------- | ------------------------- |
| Claude | `~/.claude/CLAUDE.md` | `~/.claude/skills/` | `~/.claude/subagents/` | `~/.claude.json`          |
| Gemini | `~/.gemini/GEMINI.md` | `~/.gemini/skills/` | `~/.gemini/agents/`    | `~/.gemini/settings.json` |

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
```

## License

MIT
