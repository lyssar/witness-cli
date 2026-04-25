# skuld-cli — Agent Rules

> ⚠️ **CRITICAL INFRASTRUCTURE** — Full pipeline mandatory for every change. No shortcuts.

Human reviews and commits all code. Quality over speed.

---

## Project Context

- **Type:** application
- **Stack:** go
- **Purpose:** GitOps CLI for bare servers with observer-driven application reconciliation
- **Downstream Consumers:** no
- **CI/CD:** none yet / local build-test-doc workflow

---

## Agent Team & Pipelines

Agents are global (`~/.config/opencode/agents/`). The **orchestrator** is the default primary agent.

### Canonical Agent IDs

Use agent IDs exactly as written below in generated files. Do not expand or rename them.

- Core pipeline agents: `@product-owner` (Product Owner), `@product-manager` (Product Manager), `@platform-lead` (Platform Lead), `@platform-engineer` (Platform Engineer), `@code-analyst` (Code Analyst), `@security-analyst` (Security Analyst)
- Coordination / domain agents: `@orchestrator`, `@go`, `@ebitengine`, `@infra-engineer`, `@minecraft-modder`, `@blender-minecraft`, `@fantasy-writer`

### Default: Just describe what you want
Project-level `opencode.json` cannot force a non-built-in global agent as the default. Open OpenCode and switch to `@orchestrator` manually when you want the full pipeline experience.

### Full Pipeline — Significant / Architectural Changes
```
@product-owner → @product-manager → @platform-lead → @go → @code-analyst → @security-analyst → HUMAN COMMIT
```

### Fast-Track — Trivial Changes
```
@go → @code-analyst → @security-analyst → HUMAN COMMIT
```

### Direct Agent Invocation
| What you want | What to write |
|---|---|
| Architecture review | `@platform-lead review approach for: [describe change]` |
| Jump to implementation | `@go implement: [task description]` |
| Code review only | `@code-analyst review all changes` |
| Security check only | `@security-analyst security review current changes` |

---

## Shared Rules

- **NO agent commits code** — human always commits
- **References:** `file:line` format, evidence-based, actionable
- **Severity:** Critical / High / Medium / Low
- **Output:** Summary → Findings → Deliverables → Handoff → Blockers
