# Hooks Sync — One Pager

## What are hooks?

Shell commands that platforms execute at specific lifecycle points (before a tool runs, on session start, etc.). Both Claude Code and Gemini CLI support hooks configured in `settings.json` under a `hooks` key.

## Configuration comparison

Both platforms use the same structure: event name → matcher → command array.

```json
{
  "hooks": {
    "EventName": [
      {
        "matcher": "pattern",
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/script.sh",
            "timeout": 5000
          }
        ]
      }
    ]
  }
}
```

Config fields (`type`, `command`, `matcher`, `timeout`) are identical across platforms.

## Event name mapping

### Shared (same name)

| Event          | Claude | Gemini |
| -------------- | ------ | ------ |
| Session start  | `SessionStart` | `SessionStart` |
| Notification   | `Notification` | `Notification` |

### Portable with translation (same concept, different name)

| Concept      | Claude        | Gemini        |
| ------------ | ------------- | ------------- |
| Before tool  | `PreToolUse`  | `BeforeTool`  |
| After tool   | `PostToolUse` | `AfterTool`   |

### Not portable (platform-exclusive)

| Claude-only | Gemini-only    |
| ----------- | -------------- |
| `Stop`      | `SessionEnd`   |
|             | `BeforeAgent`  |
|             | `AfterAgent`   |
|             | `BeforeModel`  |
|             | `AfterModel`   |
|             | `BeforeToolSelection` |
|             | `PreCompress`  |

## What's actually the hard part?

Not much. The config structure is identical — only event names differ, and it's a static lookup table of ~4 entries. Platform-exclusive events are written only to their platform's config. Unknown event names are inert (the platform never fires them, no error).

## Recommended approach: transform event names

Unlike subagents (which can be symlinked as-is), hooks **require transformation** — the event name is the dispatch key and can't be dropped or left ambiguous.

**TOML schema**: a `[hooks]` section using chai-neutral event names that map to both platforms.

```toml
[hooks.before-tool]
matcher = "Edit|Write"
command = "scripts/lint-check.sh"

[hooks.session-start]
command = "scripts/load-context.sh"
```

**Sync strategy**: same "replace key" approach as MCP sync. Chai owns the `hooks` key in each platform's `settings.json`, replaces it wholesale, preserves everything else. Event names are translated per platform via a hardcoded mapping table.

**Platform-exclusive events**: use the platform name as prefix.

```toml
[hooks.claude-stop]
command = "scripts/on-stop.sh"

[hooks.gemini-before-model]
command = "scripts/before-model.sh"
```

These get written only to their respective platform's config.

### Implementation notes

- Same atomic write strategy as MCP sync (read JSON → replace `hooks` key → write temp → rename).
- Chai already does this exact pattern for `mcpServers` — hooks would be a second key in the same files.
- Could share the JSON read/replace/write logic with MCP sync.
