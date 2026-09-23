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
chai doctor                     # Explain unmanaged skill destinations
chai clean                      # Remove generated outputs and orphan caches
```

`chai sync` supports `--dry-run` to preview changes and `--force` to skip dirty detection.
`chai doctor` is read-only: it lists unmanaged skills left untouched by sync and reports unmanaged destinations that would block a configured skill.
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
# Files copied to each platform that supports subagents.
paths = ["~/dotfiles/ai/subagents/*"]

[mcp.angular-cli]
# MCP server definitions written to each platform's config file.
command = "npx"
args = ["-y", "@angular/cli", "mcp"]

[[droid.custom_models]]
# Optional Droid-only BYOK model written to ~/.factory/settings.json.
model = "openai/gpt-4o-mini"
display_name = "GPT-4o Mini"
base_url = "https://api.openai.com/v1"
api_key = "${OPENAI_API_KEY}"
provider = "generic-chat-completion-api"
max_output_tokens = 4096
```

Local skill paths support `~`, absolute paths, and paths relative to
`chai.toml`. Skill names come from `SKILL.md` frontmatter, not directory names.

## Remote MCP servers and secrets

Use `command` for a local stdio server or `url` for a remote HTTP server, not both.
Chai translates the shared fields into each supported platform's format:

```toml
[mcp.stitch]
url = "https://stitch.googleapis.com/mcp"

[mcp.stitch.headers]
X-Goog-Api-Key = "${STITCH_API_KEY}"
```

Keep actual keys in **`~/.config/chai/.env`**, outside your Git-managed dotfiles:

```dotenv
STITCH_API_KEY='your-real-key'
```

Create the file privately **before adding keys**:

```sh
mkdir -p ~/.config/chai
(umask 077; touch ~/.config/chai/.env)
chmod 600 ~/.config/chai/.env
```

Open that file in your editor, add the key, then run `chai sync --dry-run` followed
by `chai sync`. Never commit the `.env` file or generated configs containing keys.
Single-quote secret values to keep `$` characters literal inside the dotenv file.

Chai loads only this fixed, optional `.env` file. It does not search your current
project or `~/.env`, execute shell commands, or modify the process environment.
Existing environment variables override file values, including empty values.
`${NAME}` references are expanded in MCP `command`, `args`, `cwd`, `url`, `env`
values, and `headers` values. Use `$$` for a literal dollar sign. Other manifest
sections are not expanded. Missing or empty referenced variables abort before
sync writes; `add` and `update` also check them before changing manifests or caches.

Dry runs redact all header and `env` values and any other substituted field.
Generated MCP configs contain **plaintext keys**, written atomically with `0600`
permissions. Rotate a key by updating its source and running `chai sync` again.
This protects the tracked manifest, not against applications running as your user.

Remote entries cannot contain `args`, `env`, or `cwd`; `headers` require `url`.
Configured headers are treated as header-based authentication: Chai disables
OAuth discovery for these entries in OpenCode and Droid. OAuth setup and legacy
SSE-only transport configuration are outside this schema.

## Sync strategy

- **Instructions** are merged in declaration order and copied to each supported target. Instructions, skills, and subagents use dirty detection to protect local edits.
- **GitHub skills** are fetched with a shallow partial clone and selected-only sparse checkout under `~/.chai/sources/`. New upstream skills are reported by `chai update` but are never installed automatically.
- **MCP servers** are **merged** into platform config files. chai owns the `mcpServers` key (or `mcp` for OpenCode, `mcp_servers` for Codex) and preserves everything else.

### Supported platforms

| Icon | Platform        | AGENTS.md | MCP | Skills | Subagents |
| ---- | --------------- | --------- | --- | ------ | --------- |
| ●    | Claude          | ✅        | ✅  | ✅     | ✅        |
| ◆    | Antigravity     | ✅        | ✅  | ✅     | ✅        |
| ✦    | Droid           | ✅        | ✅  | ✅     | ✅        |
| ■    | OpenCode        | ✅        | ✅  | ✅     | ✅        |
| ▲    | Codex           | ✅        | ✅  | ✅     | ✅        |
| ◇    | Cursor          | ❌        | ✅  | ✅     | ✅        |
| ○    | Pi              | ✅        | ❌  | ✅     | ❌        |

✅ full · ❌ not supported

## License

MIT
