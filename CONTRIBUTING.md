# Contributing

## Build & test

```bash
go build -o chai .
go test ./...
```

## Adding a new platform

This is the most likely contribution. All platform definitions live in `internal/platform/platform.go` as struct literals. To add one, copy an existing block and fill in the fields:

```go
{
    Name:             "ChatGPT",                              // display name in sync output
    InstructionsPath: filepath.Join(".chatgpt", "AGENTS.md"), // relative to ~
    SkillsDir:        filepath.Join(".chatgpt", "skills"),    // where skills are symlinked
    AgentsDir:        filepath.Join(".chatgpt", "agents"),    // where subagents are symlinked
    MCPConfigPath:    filepath.Join(".chatgpt", "mcp.json"),  // JSON file with MCP config
    MCPKey:           "mcpServers",                           // JSON key chai replaces
},
```

All paths are relative to the user's home directory. The compiler will catch missing fields.

## Design decisions to know about

- **Instructions are copied**, skills and subagents are **symlinked**. Instructions are two-way (agents may edit their copy), so chai uses hash-based dirty detection. Skills and subagents are read-only from the agent's perspective.
- **chai owns the entire `mcpServers` key** in each platform's config file. It replaces the key wholesale but preserves all other keys.
- **All file writes must be atomic** — write to a `.tmp` file, then `os.Rename`.
- **`chai sync` doesn't touch deps or extensions**. Those are handled by `chai update`.

See `CLAUDE.md` for the full architecture and `docs/design-doc.md` for the spec.
