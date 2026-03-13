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

## Prerequisites

### Firebase CLI

Required for Firebase App Hosting, Cloud Functions, Firestore rules, and emulator.

```sh
brew install firebase-cli
firebase login
```

### gcloud CLI

Required for Cloud DNS (`*.charlies.bot` subdomains) and Cloud Run (if required).

```sh
brew install --cask gcloud-cli
gcloud auth login
gcloud config set project <default-project-id>
```

### Angular CLI

Required by the Angular MCP server.

```sh
npm install -g @angular/cli
```

## MCP Servers

MCP servers give Claude and Gemini direct access to Firebase, gcloud, and Angular tooling.

### Firebase

```sh
# Claude — installed as a plugin
claude plugin marketplace add firebase/firebase-tools
claude plugin install firebase@firebase

# Gemini — installed as an extension
gemini extensions install https://github.com/gemini-cli-extensions/firebase/
```

## Linking the skills

### Gemini

gemini skills link /path/to/chai/skills
