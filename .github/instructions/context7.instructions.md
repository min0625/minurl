---
applyTo: "**"
---
# Context7 MCP Server

Use the Context7 MCP server to fetch up-to-date library documentation and code examples.

## When to Use Context7

ALWAYS use Context7 when the task involves:

- Library or framework API usage, configuration, or setup (e.g., Go standard library, Docker, golangci-lint)
- SDK or CLI tool documentation and commands
- Cloud service integration or configuration
- Version-specific documentation or migration guides
- Any library-related code generation or debugging

Do NOT use for: general programming concepts, business logic, or refactoring tasks unrelated to a specific library.

## Workflow

1. **Resolve the library ID** — call `mcp_context7_resolve-library-id` with the library name and the user's question.
2. **Fetch the docs** — call `mcp_context7_query-docs` with the resolved `libraryId` and the specific query.
3. Use the returned documentation to generate accurate, up-to-date code or answers.
