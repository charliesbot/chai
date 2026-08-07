# Skill Sources - Design

## Status

Proposed for the next major version. This is a breaking replacement for using
`[deps]` plus `@dep/...` paths to install remote skills.

## Summary

Chai should treat local skill collections and public GitHub repositories as
skill sources. Users add them through a small CLI, and chai records their intent
in a readable `chai.toml` that can be committed to a repository and shared
across machines.

The manifest is not a lock file. It contains source definitions and explicit
remote skill selections. Resolved repository paths, hashes, commits, and cache
locations remain internal chai state.

## Goals

- Make adding a public GitHub skill repository as easy as `chai add owner/repo`.
- Let users select a subset with `chai add owner/repo --skill name ...`.
- Keep `chai.toml` readable, deterministic, and suitable for version control.
- Avoid cloning unrelated repository content and history.
- Keep skill selections stable when an upstream repository moves skill folders.
- Preserve the existing copy, ownership, stale cleanup, and dirty detection
  behavior after sources are resolved.

## Non-goals

The first version does not support:

- Private GitHub repositories.
- GitHub Enterprise, GitLab, or generic Git remotes.
- SSH sources.
- Direct `SKILL.md` or archive download URLs.
- Branch, tag, or commit pinning.
- Per-skill platform selection.
- Wildcards in source paths or persisted remote selections.
- Automatic or assisted migration from the legacy skill schema.

## Design principles

### The manifest records intent

`chai.toml` is a human-authored source of truth, not generated installation
state. Chai must not write resolved paths, repository commits, cache paths, or
content hashes into it.

### The CLI is convenient, the manifest is canonical

The CLI accepts either GitHub shorthand or a full public GitHub URL. The
manifest always stores a canonical full URL.

```text
stablyai/orca
https://github.com/stablyai/orca/
https://github.com/stablyai/orca.git
```

All become:

```text
https://github.com/stablyai/orca
```

GitHub owner and repository components are case-insensitive. Chai lowercases
both components in the canonical manifest URL, uses that lowercase URL as the
logical source identity, and rejects case-insensitive duplicates.

Source classification is deterministic:

- `https://github.com/owner/repo` is a remote source.
- `owner/repo` is public GitHub shorthand.
- `/absolute/path`, `~/path`, `./path`, and `../path` are local sources.
- Any other input is rejected instead of guessed.

Public GitHub URLs must use HTTPS, contain exactly an owner and repository path,
and contain no credentials, query, fragment, or repository subpath. Chai removes
an optional trailing slash and `.git` suffix when writing the canonical URL.

### Local and remote sources have different trust boundaries

A local collection is controlled by the user, so chai may discover newly added
local skill directories automatically. A remote repository is controlled by an
upstream maintainer, so chai persists explicit skill names and never installs a
new upstream skill automatically.

## TOML schema

Dynamic local collections and explicit public GitHub selections use visibly
different structures because they have different update semantics.

```toml
[skills]
local = [
  "~/dotfiles/ai/skills",
]

[[skills.github]]
url = "https://github.com/stablyai/orca"
include = [
  "orca-cli",
  "orchestration",
]

[[skills.github]]
url = "https://github.com/vercel-labs/agent-skills"
include = [
  "frontend-design",
  "skill-creator",
  "source-driven-development",
]
```

| Configuration       | Meaning                                                                         |
| ------------------- | ------------------------------------------------------------------------------- |
| `[skills].local`    | Discover skills from each user-controlled collection or single-skill directory. |
| `[[skills.github]]` | Fetch explicitly named skills from one public GitHub repository.                |

Validation rules:

- `local` is always an array, even when it contains one path.
- Every local path must be non-empty.
- Local paths must not contain glob metacharacters such as `*`, `?`, or `[]`.
- Every GitHub entry must use a canonical `https://github.com/owner/repo` URL.
- Every GitHub entry must contain at least one explicit skill name in `include`.
- `include` must not contain `*`.
- Skill names must use lowercase ASCII letters, digits, and single hyphens. A
  name must start and end with a letter or digit, contain no consecutive
  hyphens, and be at most 64 characters.
- Duplicate local paths, case-insensitively equivalent repository URLs, and
  skill names within one repository are invalid.

The authoritative skill identity is the `name` field in `SKILL.md` frontmatter,
not the containing directory name. The frontmatter name must satisfy the safe
grammar above and must exactly match a requested `include` value. Chai uses the
validated name for destination and cache paths. Directory names may differ and
may move without changing the manifest.

## Local source discovery

Given:

```toml
[skills]
local = ["~/dotfiles/ai/skills"]
```

and:

```text
~/dotfiles/ai/skills/
|-- android-dev/
|   `-- SKILL.md
|-- slidev/
|   `-- SKILL.md
`-- web-dev/
    `-- SKILL.md
```

chai resolves `android-dev`, `slidev`, and `web-dev`.

Discovery is intentionally predictable:

1. Expand `~` and resolve the configured directory.
2. If the directory itself contains `SKILL.md`, treat it as one skill.
3. Otherwise, inspect its immediate child directories.
4. Include each child directory that contains a valid `SKILL.md`.
5. Do not recursively search arbitrary nested directories in the first version.

Adding or removing a child skill directory changes the next sync because the
local collection is user-controlled.

Local paths are cleaned lexically without resolving symlinks. Chai preserves a
literal leading `~` and rewrites an absolute path beneath the current user's
home directory back to `~/...`; this handles shells that expand an unquoted
tilde before chai receives the argument. Chai also removes redundant separators
and trailing slashes and preserves explicit `./` or `../` prefixes. Relative
paths resolve from the directory containing `chai.toml`; absolute paths outside
the current home directory remain absolute.

## Remote source discovery

Remote selections use stable skill names rather than repository-relative paths.

```toml
[[skills.github]]
url = "https://github.com/google-labs-code/stitch-skills"
include = [
  "design-md",
  "enhance-prompt",
]
```

Chai discovers the actual directories containing those skills. It must not
assume every repository uses `/skills`; repositories may use plugin layouts or
move a skill without changing its declared name.

Each requested name must resolve to exactly one `SKILL.md` whose frontmatter
declares that name. Missing, invalid, or ambiguous names fail before chai
changes the manifest or platform directories.

## CLI API

### Inspect a repository

```bash
chai add vercel-labs/agent-skills --list
```

Lists discovered skills without changing `chai.toml`, the source cache, or any
platform directory.

### Add all current skills

```bash
chai add stablyai/orca
```

With no `--skill` option, chai selects every skill currently discovered in the
repository. It expands that selection into explicit names in `include`; it does
not persist `*` or remember an "all future skills" mode.

### Add selected skills

```bash
chai add stablyai/orca --skill orca-cli orchestration
```

`--skill` accepts one or more names and narrows the selection. Repeated options
are not part of the initial API. The add command requires a custom argument
parser: after `--skill`, it consumes names until the next option or the end of
the command. The repository source remains the first positional argument.
`--skill` is valid only for GitHub sources in the first version.

### Add a local collection

```bash
chai add "~/dotfiles/ai/skills"
```

Adds the normalized collection path to the `[skills].local` array. A local path
may identify either one directory containing `SKILL.md` or a collection whose
immediate children contain skills. Local subset selection is out of scope for
the first version; users can list individual skill directories in `local` when
they do not want to track an entire collection.

### Compatibility flags

- `--global`, `-g`: accepted because chai currently manages the global
  `~/chai.toml`; it is otherwise redundant.
- `--yes`, `-y`: skips only the add summary confirmation for scripts and CI. It
  does not bypass dirty detection or imply `--force` during sync.

Per-command `--agent` is intentionally unsupported. Chai syncs to the platforms
declared in `chai.toml`.

## Add flow

`chai add` is a high-level operation, not an imperative copy command.

```text
parse source
    |
    v
discover and validate skills
    |
    v
fetch selected source content
    |
    v
show summary and confirm
    |
    v
promote validated content to source cache
    |
    v
atomically update chai.toml
    |
    v
run the normal sync pipeline
```

Requirements:

- Discovery and fetching must succeed before modifying `chai.toml`.
- Discovery and fetching before confirmation use temporary storage. The summary
  shows the canonical source, selected or tracked skills, manifest changes, and
  destination platforms.
- Declining confirmation removes temporary content and leaves `chai.toml`, the
  persistent source cache, and platform directories unchanged.
- After confirmation, chai promotes the validated temporary content into the
  persistent source cache before writing the manifest. Cache promotion uses a
  staged directory and rename/swap so an existing valid cache is not left
  partially updated.
- If cache promotion fails, chai leaves the manifest and platform directories
  unchanged. If the later manifest write fails, the promoted cache is
  non-authoritative orphan state; chai does not run sync and removes that state
  immediately or during the next cache cleanup.
- The TOML write must be atomic.
- Chai uses the stable TOML decoder and encoder to load, mutate, and rewrite the
  complete manifest in one canonical format. Comments and custom formatting may
  be removed. Comment-preserving edits are deferred until users demonstrate a
  need for them.
- Re-adding an existing source merges and sorts explicit skill names.
- Re-running the same command is idempotent.

The manifest is the desired state and is the transaction boundary. If fetching
or validation fails, chai does not edit the manifest. If the atomic manifest
write fails, chai does not run sync. If sync later fails or a dirty-file prompt
is declined, chai does not roll back the manifest; it exits non-zero, reports
that the source was recorded but sync is incomplete, and tells the user to
resolve the conflict and run `chai sync` again. A partially populated internal
cache may be reused or cleaned up, but it is never authoritative.

## Fetch architecture

Git cannot clone only one directory. Chai should use a shallow partial clone
without an initial checkout to fetch repository metadata without checking out
unrelated file contents, discover skill locations, and then use non-cone sparse
checkout for only the selected skill directories.

Git tree metadata reveals candidate `SKILL.md` paths but not their frontmatter.
During discovery, chai may lazily fetch the candidate `SKILL.md` blobs needed to
validate names and descriptions. It must not check out or eagerly fetch other
candidate skill contents until the user selects them.

Conceptually:

```bash
git clone --depth=1 --filter=blob:none --no-checkout <url> <cache-dir>
git -C <cache-dir> sparse-checkout init --no-cone
# Pass generated patterns through `sparse-checkout set --no-cone --stdin`.
git -C <cache-dir> checkout
```

The snippet is architectural, not a literal shell script. Chai must generate a
recursive non-cone pattern for each selected directory, escape every Git pattern
metacharacter so repository-controlled names are treated literally, reject NUL
and newline path components that cannot be represented safely, and pass the
patterns through standard input rather than shell interpolation. Integration
tests must cover spaces and Git pattern metacharacters in repository paths.

The exact commands may differ, but the behavior must satisfy these constraints:

- Do not fetch full repository history.
- Do not materialize unrelated repository files. Parent directories required
  to reach selected paths may exist, but unrelated files within them must not.
- Fetch every file inside each selected skill directory, including scripts,
  references, templates, and assets.
- Reject selected remote skill trees containing symbolic links, Git submodules
  (gitlinks), device nodes, or other non-regular entries. Executable regular
  files are allowed. Every copied path must remain beneath the selected skill
  directory after lexical and filesystem containment checks.
- Store the partial repository in chai's internal cache, not beside the
  manifest or in platform skill directories.
- On update, fetch the latest tree, resolve selected names again, and update the
  sparse paths if an upstream skill moved.

Selecting every current skill checks out every discovered skill directory, not
the rest of the repository.

### Git requirement

Remote add and update operations require Git 2.37 or newer. This version floor
provides the explicit non-cone sparse-checkout workflow used by chai.

Chai checks the installed Git version before network or cache mutation. Missing
and outdated Git installations produce distinct errors containing the detected
version when available. Chai never falls back to a full clone.

The requirement applies to remote `chai add` and `chai update`. Local add,
`chai clean`, and offline `chai sync` with a complete cache do not require Git.
On macOS, diagnostics may suggest `brew install git`; other platforms receive a
link to `https://git-scm.com/downloads`. Chai does not install or upgrade Git.

## Update and sync behavior

`chai sync` remains the fast, offline operation. It resolves local collections
and cached remote skills, then copies them to configured platforms using the
existing hash ownership and dirty detection behavior.

If a remote source or included skill is missing from the cache, `chai sync`
must not access the network or mutate the manifest. It fails with the missing
source and skill names and instructs the user to run `chai update`. The same
rule applies to a partially populated cache left by an interrupted add or
update.

`chai update` performs network work:

1. Fetch the latest metadata for each remote source.
2. Rediscover each explicitly included skill by name.
3. Update only the selected skill directories.
4. Report newly available upstream skills without installing them.
5. Run sync after successful source updates.

An upstream skill that is not listed in `include` is never installed
automatically. Users add it explicitly:

```bash
chai add owner/repo --skill new-skill
```

## Internal state

Remote source caches use a readable hierarchy derived from the canonical public
GitHub URL:

```text
~/.chai/sources/github.com/<lowercase-owner>/<lowercase-repository>/
```

Owner and repository are validated as separate path components. The lowercase
canonical URL remains the logical state key. Cache updates use sibling staging
directories for atomic rename/swap, and unreferenced cache directories are
eligible for cleanup.

Chai may also store the following under `~/.chai/`:

- Partial source repositories.
- Canonical source identities.
- Resolved skill-name-to-path mappings.
- Last fetched commits.
- Content hashes and destination ownership.

None of this state belongs in `chai.toml`.

## Validation

Chai validates the complete manifest strictly before performing command side
effects. Unknown fields are errors; they are never ignored.

Static validation includes:

- TOML syntax and value types.
- Unknown fields and sections.
- Required fields and supported platform names.
- Local path syntax and duplicate paths.
- Canonical public GitHub URLs and duplicate repositories.
- Skill-name grammar and duplicate `include` names.
- Cross-entry conflicts that can be determined without discovery.

Static validation does not access the network. Operational validation runs only
when a command needs it and includes local path existence, discovered skill
validity, remote repository and included-skill existence, global resolved-name
conflicts, cache completeness, and Git capability.

No manifest, cache, hash database, or platform directory is mutated until the
relevant validation phase succeeds. Errors identify the configuration field or
source involved and should report multiple independent static failures in one
run where practical.

## Name conflicts

Platform skill directories identify skills by name. Every resolved skill name
must therefore be globally unique. Chai must detect duplicates across GitHub
repositories, across local roots, among children of one local collection, and
through overlapping local paths. It fails with every conflicting source
location rather than silently overwriting one skill with another. Aliasing is
out of scope for the first version.

## Relationship to dependencies

Remote skills no longer use `[deps]` or `@dep/...` paths.

`[deps]` may remain for repositories that chai must clone for a real runtime
purpose, such as an MCP working directory or a first-clone build command. Its
continued existence is separate from the skill source design.

In the skills schema:

- `@dep/...` entries are no longer accepted.
- Globs are no longer accepted.
- Local collections use the `[skills].local` array.
- Public GitHub repositories use `[[skills.github]]` entries with `url` plus
  explicit `include` names.

## Breaking release

This schema ships in a major release. The legacy `[skills].paths` field and
dependency-backed `@dep/...` skill references are not supported. Strict schema
validation rejects them as unknown or invalid configuration rather than
silently resolving an empty skill set.

Chai does not provide a migration command, automatic conversion, compatibility
layer, backup flow, or migration-specific diagnostics. The README documents the
current v2 schema and CLI only.
