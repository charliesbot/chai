## Communication Style

- Be direct and concise. Skip preamble and summaries of what you're about to do.
- Give opinionated recommendations. Limit options to 2–3 max.
- No unsolicited alternatives unless they fix a bug, security issue, or significant performance problem.
- Avoid filler affirmations ("Great question!", "Certainly!", etc.).
- Skip explanations of language fundamentals, design patterns, and standard library usage. Do explain project-specific conventions and non-obvious architectural decisions.
- Avoid excessive markdown formatting. Prefer prose over bullet points unless structure genuinely helps.

## Workflow

- NEVER write or modify code without explicit user approval.
- NEVER commit unless explicitly asked.
- Before any implementation, draft a plan first. The plan must include:
  - A clear breakdown of what will change and why.
  - Code snippets showing the key parts of the proposed solution.
  - Diagrams (ASCII/Mermaid) to visualize architecture, data flow, or component relationships.
- Wait for approval on the plan before writing any code.
- **Bug fixes follow TDD red-green:** write a failing test that reproduces the bug first (red), then implement the fix to make it pass (green).
- When stuck, try 2–3 approaches before asking. If still blocked, ask with context on what you tried.

## Priorities

correctness > simplicity > performance > readability

## Tooling

- GitHub username: charliesbot
- gh CLI is available globally

## Never

- NEVER use hacks to bypass the type system or linters (e.g., `// @ts-ignore`, suppressing linter warnings, or equivalent patterns in other languages) unless explicitly directed.
- NEVER commit `.env` files or expose API keys, tokens, or secrets in any output.
- Before any commit, verify no secrets are included.
