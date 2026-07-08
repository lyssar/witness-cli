---
name: skuld-cli
description: GitOps CLI for bare servers with observer-driven application reconciliation — Go standards, linting commands, and project conventions
---

## Project Context

- **Type:** application
- **Purpose:** GitOps CLI for bare servers with observer-driven application reconciliation
- **Go Version:** 1.25.5
- **Downstream Consumers:** no
- **CI/CD:** CircleCI runs `task validate` and a reduced-scope local harness smoke path; local build-test-doc workflow remains available

## Go Standards (All Agents)

- Follow `gofmt` formatting — no exceptions
- `golangci-lint` for static analysis
- Error handling: always check errors, never use `_` on error returns
- Interfaces defined at point of use (consumer side)
- Exported types and functions must have godoc comments

## CI/CD Commands (for Platform Engineer and Code Analyst)

```bash
task validate
task local:smoke
```

## Code Analyst Checklist Additions

- [ ] `go vet` passes
- [ ] `golangci-lint` passes
- [ ] All tests pass with `-race` flag
- [ ] No ignored errors (`_ = someFunc()` without comment)
- [ ] Exported symbols have godoc comments

## Security Analyst Additions

- [ ] No hardcoded credentials or tokens
- [ ] `go list -m -json all | nancy` or `govulncheck` for known vulnerabilities (advisory)
- [ ] User input sanitized before use in `exec.Command` or SQL queries
- [ ] Sensitive config loaded from environment, not source files
