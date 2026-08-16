---
name: chai
description: Manage AI coding-agent instructions, skills, subagents, and MCP servers through Chai. Use when adding, removing, editing, syncing, or troubleshooting agent configuration on a system with ~/chai.toml, or whenever the user mentions Chai-managed agent configuration.
---

# Chai

Treat `~/chai.toml` and the source paths it references as authoritative. Treat files copied into platform directories as generated outputs.

## Work from the source

1. Confirm that `chai` is available and read `~/chai.toml`.
2. Identify which manifest entry owns the requested resource.
3. Modify the canonical source or the manifest, not a synchronized copy.
4. Run `chai sync --dry-run` and inspect the proposed changes.
5. Run `chai sync` only when the preview matches the request.

Use `chai <command> --help` when exact flags or current command behavior are needed.

## Locate the canonical resource

- **Instructions:** Edit the files listed by `instructions`, preserving their declared merge order.
- **Local skills:** Resolve entries under `[skills].local`. A path can name one skill or a collection whose immediate child directories are skills. Match skills using the `name` in `SKILL.md` frontmatter rather than the directory name.
- **GitHub skills:** Treat entries under `[[skills.github]]` as upstream-managed. Do not edit Chai's cached checkout. Modify the upstream repository, use a fork, or replace the entry with a local source.
- **Subagents:** Edit the files matched by `[subagents].paths`.
- **MCP servers:** Edit their definitions in `~/chai.toml`.
- **Platforms:** Edit the top-level `platforms` list.

Resolve `~` from the user's home directory and relative paths from the directory containing `chai.toml`.

## Use the CLI deliberately

| Intent | Command |
| --- | --- |
| Add local or public GitHub skills | `chai add <source>` |
| Inspect a GitHub source | `chai add <source> --list` |
| Preview distribution | `chai sync --dry-run` |
| Distribute current configuration | `chai sync` |
| Refresh remote sources, then sync | `chai update` |
| Preview generated-output cleanup | `chai clean --dry-run` |

Edit `~/chai.toml` directly for configuration changes without a dedicated command. Preserve its existing formatting and unrelated content.

## Protect managed files

Do not directly edit:

- Chai's remote cache under `~/.chai/sources/`.
- Generated skill, instruction, subagent, or MCP files inside platform configuration directories.

Do not use `chai sync --force` merely to resolve uncertainty. It bypasses dirty-file protection; use it only when the user explicitly intends to overwrite locally modified generated files.

Before running `chai clean`, preview it and obtain confirmation because cleanup removes generated outputs. If Chai reports a dirty-file conflict, identify the canonical source and compare the files before choosing whether to overwrite anything.
