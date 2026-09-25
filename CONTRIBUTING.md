# Contributing

Thanks for your interest in Witness. This project is a GitOps CLI for bare
servers with observer-driven application reconciliation.

## Reporting Issues

- Search the [issue tracker](https://github.com/lyssar/witness-cli/issues) first
  to avoid duplicates.
- Include the Witness version (`witness version`), your OS, and the exact
  command that failed.
- For security vulnerabilities, follow the process in [SECURITY.md](SECURITY.md)
  instead of opening a public issue.

## Proposing Changes

1. Open an issue or discussion to describe the change before writing code.
2. Fork the repository and create a feature branch.
3. Keep changes focused and small; one logical change per pull request.
4. Follow the existing code style and add tests where behavior changes.
5. Open a pull request and reference the related issue.

## Development Setup

Requirements: Go toolchain and [Task](https://taskfile.dev/).

```bash
task build      # build the witness binary
task validate   # run linting and tests
```

Run the local harness smoke check with `task local:smoke`.

## Review & Commit

Humans review and commit all code. No automated agent commits to this
repository. Expect review feedback on your pull request and be prepared to
iterate.