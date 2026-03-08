# chai

Charlie's AI configuration repo — shared config for Claude and Gemini.

## Structure

```
AGENTS.md              # shared agent definitions
skills/                # shared skills
mcp/                   # shared MCP server configs
claude/CLAUDE.md       # symlink -> ../AGENTS.md
gemini/GEMINI.md       # symlink -> ../AGENTS.md
install.sh             # symlink installer
```

## Setup

```sh
./install.sh
```
