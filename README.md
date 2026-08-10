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
chai update                     # Refresh remote skills, then sync
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
platforms = ["claude", "antigravity", "droid", "opencode", "codex", "cursor", "pi"]

# Your shared instruction files. Merged in order for each supported platform.
instructions = [
  "~/dotfiles/ai/instructions/AGENTS.md",
  "~/dotfiles/ai/instructions/ADHD.md",
]

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
```

Local skill paths support `~`, absolute paths, and paths relative to
`chai.toml`. Skill names come from `SKILL.md` frontmatter, not directory names.

## Sync strategy

- **Instructions** are merged in declaration order and copied to each supported target. Instructions, skills, and subagents use dirty detection to protect local edits.
- **GitHub skills** are fetched with a shallow partial clone and selected-only sparse checkout under `~/.chai/sources/`. New upstream skills are reported by `chai update` but are never installed automatically.
- **MCP servers** are **merged** into platform config files. chai owns the `mcpServers` key (or `mcp` for OpenCode, `mcp_servers` for Codex) and preserves everything else.

### Supported platforms

| Icon | Platform        | AGENTS.md | MCP | Skills | Subagents |
| ---- | --------------- | --------- | --- | ------ | --------- |
| ●    | Claude          | ✅        | ✅  | ✅     | ✅        |
| ◆    | Antigravity     | ✅        | ✅  | ✅     | ❌        |
| ✦    | Droid           | ✅        | ✅  | ✅     | ✅        |
| ■    | OpenCode        | ✅        | ✅  | ✅     | ✅        |
| ▲    | Codex           | ✅        | ✅  | ✅     | ✅        |
| ◇    | Cursor          | ❌        | ✅  | ✅     | ✅        |
| ○    | Pi              | ✅        | ❌  | ✅     | ❌        |

✅ full · ❌ not supported

## License

MIT
