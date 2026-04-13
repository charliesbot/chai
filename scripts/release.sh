#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "Usage: $0 <version>"
  echo "Example: $0 v0.1.0"
  exit 1
fi

VERSION="$1"

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Error: version must match vX.Y.Z (e.g., v0.1.0)"
  exit 1
fi

if git rev-parse "$VERSION" >/dev/null 2>&1; then
  echo "Error: tag $VERSION already exists"
  exit 1
fi

echo "Creating tag $VERSION..."
git tag "$VERSION"
git push origin "$VERSION"
echo "Tag $VERSION pushed. GitHub Actions will build the release."
echo "Watch progress: https://github.com/charliesbot/chai/actions"
