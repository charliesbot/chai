# chai

Charlie's AI configuration repo — shared config for Claude and Gemini.

## Structure

```
_AGENTS.md             # shared agent definitions
skills/                # shared skills
mcp/                   # shared MCP server configs
install.sh             # creates symlinks (CLAUDE.md, GEMINI.md, etc.)
```

## Setup

```sh
./install.sh
```

## Expected MCPs

### Firebase

```sh
# Gemini
gemini extensions install https://github.com/gemini-cli-extensions/firebase/

# Claude
claude plugin marketplace add firebase/firebase-tools
claude plugin install firebase@firebase
claude plugin marketplace list
```

### Angular

```sh
npm install -g @angular/cli
```

```json
{
  "mcpServers": {
    "angular-cli": {
      "command": "npx",
      "args": ["-y", "@angular/cli", "mcp"]
    }
  }
}
```

###
