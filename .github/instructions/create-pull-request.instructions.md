---
applyTo: "**"
---
# Creating a Pull Request on GitHub

This document provides step-by-step instructions for AI agents to create a Pull Request (PR) on GitHub. Follow every step in order. Do NOT skip any step.

---

## Step 1: Inspect the Current Repository State

Before doing anything else, gather full context about the repository state.

```bash
# 1a. Check overall git status (shows staged, unstaged, untracked files)
git status

# 1b. Identify the current branch
git branch --show-current

# 1c. List all local and remote branches
git branch -a

# 1d. Confirm the remote URL (must be a GitHub repository)
git remote -v

# 1e. Check recent commits to understand the history
git log --oneline -10

# 1f. Check if the local branch is behind or ahead of the remote
git fetch --prune
git status
```

**Decision point after Step 1:**
- If you are already on a feature branch (not `main` / `master` / `develop`) with unpushed commits → skip to Step 4.
- If there are no uncommitted changes and nothing to push → stop. There is nothing to create a PR for.
- Otherwise, continue to Step 2.

---

## Step 2: Determine the Branch Name

Choose a branch name that reflects the purpose of the change. Always use **lowercase kebab-case** and a conventional prefix.

### Conventional Prefix Table

| Prefix | Use case |
|---|---|
| `feat/` | A new feature or capability |
| `fix/` | A bug fix |
| `docs/` | Documentation-only changes |
| `chore/` | Maintenance, tooling, dependency updates |
| `refactor/` | Code restructuring without behavior change |
| `test/` | Adding or improving tests |
| `ci/` | CI/CD pipeline changes |
| `build/` | Build system or Dockerfile changes |
| `perf/` | Performance improvements |
| `style/` | Formatting, lint fixes (no logic changes) |
| `revert/` | Reverting a previous commit |

### Branch Naming Rules

- Format: `<prefix>/<short-description>` (e.g., `feat/add-greeting-flag`)
- Use only lowercase letters, numbers, and hyphens (`-`).
- Keep it concise: 3–6 words maximum.
- Do NOT include issue numbers unless explicitly asked.
- Do NOT use underscores (`_`) or slashes within the description part.

**Good examples:**
- `feat/add-json-output`
- `fix/nil-pointer-on-empty-input`
- `chore/upgrade-golangci-lint`
- `docs/update-readme-docker`

**Bad examples:**
- `my_branch` (underscores, no prefix)
- `feature-branch` (no prefix)
- `fix` (no description)
- `FEAT/Add-JSON` (uppercase)

---

## Step 3: Create and Switch to the New Branch

Only create a new branch if you are currently on a protected/base branch (e.g., `main`, `master`, `develop`).

```bash
# Ensure you start from the latest state of the base branch
git checkout main          # or master / develop — use the default branch
git pull origin main       # sync with remote

# Create and switch to the new branch
git checkout -b feat/your-description   # replace with the actual branch name

# Verify you are on the new branch
git branch --show-current
```

If you are already on an appropriate feature branch, simply pull to make sure it is up to date:

```bash
git pull origin <current-branch> --rebase
```

---

## Step 4: Stage the Changes

Review and selectively stage only the relevant files.

```bash
# Review the diff of all changes
git diff

# Review only staged changes
git diff --staged

# Stage specific files (preferred over staging everything blindly)
git add path/to/file1 path/to/file2

# Or stage all tracked changes if all changes are intentional
git add -A

# Confirm what is staged
git status
git diff --staged
```

Ensure no unintended files are staged (e.g., editor configs, generated artifacts, secrets).

---

## Step 5: Write the Commit Message

Use the **Conventional Commits** format. The commit message must be in **English**.

### Format

```
<type>(<optional scope>): <short description in present tense, imperative mood>

<optional body: explain WHY, not WHAT, wrapped at 72 chars>

<optional footer: BREAKING CHANGE or issue references>
```

### Commit Message Rules

- **Subject line**: maximum **72 characters**.
- **Type**: same prefixes as branches but without the slash (`feat`, `fix`, `docs`, etc.).
- **Description**: use imperative present tense — "add", "fix", "update", NOT "added", "fixed", "updating".
- **Scope** (optional): a noun describing the section of the codebase (e.g., `feat(api): ...`).
- **Body** (optional): explain the motivation for the change. Use blank line to separate from subject.
- No period at the end of the subject line.

**Good examples:**
```
feat: add JSON output flag to CLI
fix(config): prevent nil pointer on missing env var
chore: upgrade golangci-lint to 2.11.2
docs: add Docker usage section to README
refactor(handler): extract greeting logic into separate function
```

**Bad examples:**
```
updated stuff          # vague, not conventional
Fix bug.               # period, no type
feat: Added JSON flag  # past tense
```

### Commit the Changes

```bash
git commit -m "feat: add JSON output flag to CLI"

# For multi-line messages, use the editor:
git commit
# Then write the full message in the editor.
```

---

## Step 6: Push the Branch to Remote

```bash
# Push and set the upstream tracking branch
git push -u origin <branch-name>

# Example:
git push -u origin feat/add-json-output
```

If the push is rejected because the remote branch already exists and has diverged:

```bash
# Only use --force-with-lease (safer than --force); NEVER use --force on shared branches
git push --force-with-lease origin <branch-name>
```

Verify the push succeeded:

```bash
git log origin/<branch-name> --oneline -5
```

---

## Step 7: Create the Pull Request

### 7a. Using the GitHub MCP Tool (Preferred)

Use the `mcp_github_create_pull_request` tool. This is the **preferred method** when an MCP-capable agent is running, as it does not require the `gh` CLI to be installed.

Required parameters:

| Parameter | Value |
|---|---|
| `owner` | Repository owner (e.g., `min0625`) |
| `repo` | Repository name (e.g., `minurl`) |
| `title` | PR title (see naming rules below) |
| `body` | PR description (use the template below) |
| `head` | Your feature branch name |
| `base` | Target branch (usually `main`) |

### 7b. Fallback: GitHub CLI (`gh`)

Only use `gh` if the MCP tool is unavailable:

```bash
gh pr create --base main --head <branch-name> \
  --title "<PR title>" \
  --body "<PR body>"
```

### 7c. PR Title

The PR title MUST follow the same Conventional Commits format as the commit message.

- Format: `<type>(<optional scope>): <short description>`
- Maximum **72 characters**.
- Imperative present tense.
- No period at the end.

If there is a single commit, the PR title should match the commit message exactly.
If there are multiple commits, write a summary title that covers the overall change.

**Good PR titles:**
- `feat: add JSON output flag to CLI`
- `fix(config): prevent nil pointer on missing env var`
- `chore: upgrade golangci-lint to 2.11.2`

### 7d. PR Description Template

Use the following template for the PR body (in Markdown):

```markdown
## Summary

<!-- One paragraph describing WHAT was changed and WHY. -->

## Changes

<!-- Bullet list of the key changes made. -->
-
-

## How to Test

<!-- Step-by-step instructions for a reviewer to verify the change. -->
1.
2.

## Checklist

- [ ] No unintended files are included
- [ ] Commit message follows Conventional Commits format
```

### 7e. Full Example

Using the MCP tool (preferred):

```
mcp_github_github_create_pull_request(
  owner = "min0625",
  repo  = "minurl",
  title = "feat: add JSON output flag to CLI",
  head  = "feat/add-json-output",
  base  = "main",
  body  = "## Summary

Add a `--json` flag that outputs the greeting in JSON format instead of plain text.

## Changes

- Add `--json` boolean flag to the root command
- Implement JSON marshalling of the greeting struct
- Add unit tests for JSON output mode

## How to Test

1. Run `go run . --json`

## Checklist

- [x] No unintended files are included
- [x] Commit message follows Conventional Commits format"
)
```

---

## Step 8: Verify the Pull Request

After creation, confirm the PR exists and is correct.

**Using the MCP tool (preferred):**

```
mcp_github_github_pull_request_read(
  owner      = "min0625",
  repo       = "minurl",
  pullNumber = <PR number returned from Step 7>
)
```

**Fallback — GitHub CLI:**

```bash
gh pr list
gh pr view <PR-number>
```

Check the following in the PR:
- Title is correct and follows conventions.
- Base branch is `main` (or the intended target).
- Head branch is your feature branch.
- Description is complete.
- No unintended files or changes are included.

---

## Step 9: Handle Post-Creation Tasks (if applicable)

```bash
# If CI fails, fix the issue, commit, and push again (PR updates automatically)
git add -A
git commit -m "fix: address CI failure in JSON output test"
git push

# If reviewers request changes, make the edits, then:
git add -A
git commit -m "refactor: apply code review feedback"
git push

# Do NOT force-push to a PR branch unless absolutely necessary.
```

---

## Naming Convention Quick Reference

| Artifact | Format | Example |
|---|---|---|
| Branch | `<type>/<kebab-description>` | `feat/add-json-output` |
| Commit | `<type>(<scope>): <description>` | `feat: add JSON output flag` |
| PR title | Same as commit | `feat: add JSON output flag` |
| PR body | Markdown with Summary / Changes / How to Test | See template above |

---

## Common Mistakes to Avoid

- **Do not** commit directly to `main` / `master` / `develop`.
- **Do not** push with `--force` (use `--force-with-lease` if needed).
- **Do not** include secrets, credentials, or large binary files.
- **Do not** use vague branch names or commit messages.
- **Do not** mix unrelated changes in a single PR. Keep each PR focused on one concern.
