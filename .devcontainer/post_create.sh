#!/bin/bash

set -euo pipefail

# note that bash will read from ~/.profile or ~/.bash_profile if the latter exists
# ergo, you may want to check to see which is defined on your system and only append to the existing file
echo 'eval "$(mise activate bash --shims)"' >>~/.bash_profile # this sets up non-interactive sessions
echo 'eval "$(mise activate bash)"' >>~/.bashrc               # this sets up interactive sessions

mise trust .

mise install

mise exec -- go mod download

mise exec -- go install -v golang.org/x/tools/gopls@latest
mise exec -- go install -v github.com/go-delve/delve/cmd/dlv@latest

# --overwrite replaces a shim left by a previous `pre-commit install`; without it
# prek keeps the old hook and runs it too, and upstream pre-commit cannot parse
# this config (`repo: builtin` has no `rev`), so every commit would fail.
# --prepare-hooks builds the gitleaks environment now instead of on first commit.
mise exec -- prek install --overwrite --prepare-hooks

if [[ -f ".devcontainer/post_create.local.sh" ]]; then
    source ".devcontainer/post_create.local.sh"
fi
