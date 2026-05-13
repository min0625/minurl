---
applyTo: "**"
---
# Tool Priority Rules

Before executing ANY operation, you MUST inspect the available tools in this order and use the first capable option.

## Priority Order

1. **MCP Tools** — Check `availableDeferredTools` in context. Load with `tool_search` before calling.
2. **Skills** — Check `<skills>` in context. Load the relevant SKILL.md before executing.
3. **CLI tools** — Only fall back to terminal commands if no MCP tool or skill can accomplish the task.

## Mandatory Pre-Action Checklist

Before starting any task, answer these questions:

1. Is there an MCP tool that can do this? (search with `tool_search` if unsure)
2. Is there a skill listed in `<skills>` that covers this task?
3. Only if both answers are **No** → use CLI.

## Examples

| Task | Wrong approach | Correct approach |
|---|---|---|
| Fetch library docs | `curl` / web search | `mcp_context7_query-docs` |
| Create a Pull Request | `gh pr create` | `mcp_github_github_create_pull_request` |
| Go diagnostics | `go build ./...` | `go_diagnostics` |
| Find Go symbols | `grep` / `rg` | `go_search` |
| Summarize a GitHub issue | `gh issue view` | `summarize-github-issue-pr-notification` skill |
| Address PR review comments | manual edits only | `address-pr-comments` skill |

## Loading Deferred MCP Tools

Deferred tools listed in `availableDeferredTools` are NOT loaded by default.
**Always call `tool_search` with a natural-language description of what you need before calling any deferred tool.**
If `tool_search` returns no matching tool, the tool is unavailable — proceed to the next priority level.

## Loading Skills

Skills are listed in `<skills>` in the system context.
When a task matches a skill's description, **read the SKILL.md file first** to get full instructions before proceeding.

## When CLI Is Acceptable

CLI tools (`git`, `go`, `make`, `gh`, `docker`, etc.) are acceptable **only** when:

- No MCP tool covers the operation, AND
- No skill covers the operation, AND
- The task genuinely requires shell execution (e.g., running tests, building binaries)

Even then, prefer `make` targets (e.g., `make check`, `make test`) over raw commands where a Makefile target exists.
