# chai

Keep AI coding agent configs in sync. One manifest, distributed to every platform.

## Install

```bash
brew install charliesbot/tap/chai        # Homebrew (macOS and Linux)
go install github.com/charliesbot/chai@latest  # From source
```

Or grab a binary from [GitHub Releases](https://github.com/charliesbot/chai/releases/latest).

## Usage

```bash
chai init    # Scaffold ~/chai.toml
chai update  # Clone deps and install extensions
chai sync    # Distribute config to all platforms
```

`chai sync` supports `--dry-run` to preview changes and `--force` to skip dirty detection.

## Config

Everything lives in `~/chai.toml`:

```toml
# Which platforms to sync to. Only these get touched.
platforms = ["claude", "gemini"]

# Your shared instructions file. Copied to each platform with dirty detection.
instructions = "~/dotfiles/ai/instructions/AGENTS.md"

[deps]
# Git repos cloned to ~/.chai/deps/. Reference as @name in other paths.
angular-skills = "https://github.com/angular/skills"

# Deps that need a build step use a table. Build runs once on first clone.
[deps.some-tool]
url = "https://github.com/example/tool"
build = "npm install"

[skills]
# Directories symlinked to each platform's skills folder.
paths = ["~/dotfiles/ai/skills", "@angular-skills"]

[subagents]
# Directories symlinked to each platform's agents folder.
paths = ["~/dotfiles/ai/subagents"]

[mcp.angular-cli]
# MCP server definitions written to each platform's config file.
command = "npx"
args = ["-y", "@angular/cli", "mcp"]

[gemini.extensions]
# Gemini-only extensions installed via 'gemini extensions install'.
workspace = "https://github.com/gemini-cli-extensions/workspace"
```

Paths support `~` (home directory) and `@name` (resolves to `~/.chai/deps/<name>/`).

## Sync strategy

- **Instructions** are **copied** with hash-based dirty detection. Agents may edit their copy. _chai_ detects changes and prompts before overwriting.
- **Skills and subagents** are **symlinked**. One source of truth, read only from the agent's perspective.
- **MCP servers** are **merged** into platform config files. chai owns the `mcpServers` key and preserves everything else.AA

## License

MIT
