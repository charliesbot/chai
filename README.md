# chai

Keep AI coding agent configs in sync. One manifest, distributed to every platform.

## Installation

### Homebrew

```bash
brew install charliesbot/tap/chai
```

### Go

```bash
go install github.com/charliesbot/chai@latest
```

### Binary

Download the latest binary from [GitHub Releases](https://github.com/charliesbot/chai/releases/latest).

## Usage

```bash
chai init                       # Scaffold ~/chai.toml
chai add ~/dotfiles/ai/skills   # Track a local skill collection
chai add owner/repo             # Add every current skill from public GitHub
chai add owner/repo --skill one two
chai add owner/repo --list      # Inspect without changing config or caches
chai update                     # Refresh remote skills, deps, and plugins
chai sync                       # Offline distribution to all platforms
chai clean                      # Remove generated outputs and orphan caches
```

`chai sync` supports `--dry-run` to preview changes and `--force` to skip dirty detection.
Remote skill operations require Git 2.37 or newer. `chai sync` is offline; if a
remote cache is missing, run `chai update`.

## Config

Everything lives in `~/chai.toml`:

```toml
# Which platforms to sync to. Only these get touched.
platforms = ["claude", "antigravity", "droid", "opencode", "codex", "cursor"]

# Your shared instructions file. Copied to platforms with a global instructions file.
instructions = ["~/dotfiles/ai/instructions/AGENTS.md"]

[deps]
# Git repos cloned to ~/.chai/deps/. Reference as @name in other paths.
angular-skills = "https://github.com/angular/skills"

# Deps that need a build step use a table. Build runs once on first clone.
[deps.some-tool]
url = "https://github.com/example/tool"
build = "npm install"

[skills]
# Each path is either one skill directory or a collection whose immediate
# child directories contain SKILL.md files.
local = ["~/dotfiles/ai/skills"]

[[skills.github]]
url = "https://github.com/vercel-labs/agent-skills"
include = ["frontend-design", "skill-creator"]

[subagents]
# Files copied to each platform's agents folder.
paths = ["~/dotfiles/ai/subagents/*"]

[mcp.angular-cli]
# MCP server definitions written to each platform's config file.
command = "npx"
args = ["-y", "@angular/cli", "mcp"]

[antigravity.plugins]
# Antigravity plugins installed via 'agy plugin install' on 'chai update'.
workspace = "https://github.com/gemini-cli-extensions/workspace"

[[droid.custom_models]]
# Optional Droid-only BYOK custom model written to ~/.factory/settings.json.
model = "openai/gpt-4o-mini"
display_name = "GPT-4o Mini"
base_url = "https://api.openai.com/v1"
api_key = "${OPENAI_API_KEY}"
provider = "generic-chat-completion-api"
max_output_tokens = 4096
```

Local skill paths support `~`, absolute paths, and paths relative to
`chai.toml`. Skill names come from `SKILL.md` frontmatter, not directory names.
Dependency references (`@name`) remain available for runtime paths such as MCP
`cwd`, but are not accepted as skill sources.

## Sync strategy

- **Instructions and skills** are **copied** with hash-based dirty detection. Instructions are only copied to platforms with a global instructions file. _chai_ detects destination changes and prompts before overwriting. Subagents are copied with ownership tracking for stale cleanup.
- **GitHub skills** are fetched with a shallow partial clone and selected-only sparse checkout under `~/.chai/sources/`. New upstream skills are reported by `chai update` but are never installed automatically.
- **MCP servers** are **merged** into platform config files. chai owns the `mcpServers` key (or `mcp` for OpenCode, `mcp_servers` for Codex) and preserves everything else.
- **Droid custom models** are merged into `~/.factory/settings.json` under `customModels` and preserve unrelated settings.

### Supported platforms

| Icon | Platform        | AGENTS.md | MCP | Skills | Subagents |
| ---- | --------------- | --------- | --- | ------ | --------- |
| ●    | Claude          | ✅        | ✅  | ✅     | ✅        |
| ◆    | Antigravity     | ✅        | ✅  | ✅     | ❌        |
| ✦    | Droid           | ✅        | ✅  | ✅     | ✅        |
| ■    | OpenCode        | ✅        | ✅  | ✅     | ✅        |
| ▲    | Codex           | ✅        | ✅  | ✅     | ✅        |
| ◇    | Cursor          | ❌        | ✅  | ✅     | ✅        |

✅ full · ❌ not supported

`antigravity` syncs the IDE, legacy IDE, and CLI user-level Antigravity directories.

## License

MIT
