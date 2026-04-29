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
    SkillsDir:        filepath.Join(".chatgpt", "skills"),    // where skills are copied
    AgentsDir:        filepath.Join(".chatgpt", "agents"),    // where subagents are copied, if supported
    MCPConfigPath:    filepath.Join(".chatgpt", "mcp.json"),  // config file with MCP entries
    MCPKey:           "mcpServers",                           // config key chai replaces
    MCPFormat:        MCPFormatStandard,                       // on-disk MCP entry shape
},
```

All paths are relative to the user's home directory. The compiler will catch missing fields.

## Design decisions to know about

- **Instructions, skills, and subagents are copied**. Instructions use dirty detection because agents may edit their platform copy; skills/subagents use hash-based ownership so stale chai-managed copies can be removed without touching user-created files.
- **chai owns each platform's MCP key**. It replaces that key wholesale but preserves all other keys.
- **All file writes must be atomic** — write to a `.tmp` file, then `os.Rename`.
- **`chai sync` doesn't touch deps or extensions**. Those are handled by `chai update`.

See `CLAUDE.md` for the full architecture and `docs/design-doc.md` for the spec.
