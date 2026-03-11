#!/usr/bin/env bash
set -euo pipefail

# Scaffold a new Angular + Firebase side project with all conventions pre-configured.
#
# Usage:
#   ./new-project.sh <project-name>
#
# Example:
#   ./new-project.sh my-app
#
# Creates:
#   <project-name>/  — Angular project with CSS reset and git initialized.
#
# Firebase, Firestore, auth, environments, etc. are added on demand
# as the project needs them — guided by the web-dev skill.

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <project-name>"
    echo "  e.g. $0 my-app"
    exit 1
fi

PROJECT_NAME="$1"

# Validate: lowercase letters, numbers, and hyphens only
if [[ ! "$PROJECT_NAME" =~ ^[a-z][a-z0-9-]*$ ]]; then
    echo "Error: Project name must start with a lowercase letter and contain only lowercase letters, numbers, and hyphens."
    exit 1
fi

if [[ -d "$PROJECT_NAME" ]]; then
    echo "Error: Directory '$PROJECT_NAME' already exists."
    exit 1
fi

echo "==> Creating Angular project: $PROJECT_NAME (this may take a minute...)"
ng new "$PROJECT_NAME" --style=css --routing --ssr --skip-git --skip-install --defaults --zoneless > /dev/null

cd "$PROJECT_NAME"

# --- CSS reset + custom properties ---
echo "==> Writing CSS reset and custom properties"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cp "$SCRIPT_DIR/../assets/reset.css" src/styles.css

# --- Install dependencies ---
echo "==> Installing dependencies"
npm install

# --- Git init ---
echo "==> Initializing git repository"
git init
git add -A
git commit -m "Initial project setup

Scaffolded with new-project.sh. Angular with CSS reset and routing."

echo ""
echo "==> Done! Project '$PROJECT_NAME' is ready."
echo ""
echo "Next steps:"
echo "  1. cd $PROJECT_NAME"
echo "  2. ng serve — start building"
echo ""
echo "The web-dev skill will guide you when you need Firebase, Firestore,"
echo "auth, environments, subdomain setup, or deployment."
