# Claude Code Hooks

Automated quality enforcement hooks for Claude Code sessions.

## Hooks Overview

| Hook | Trigger | Purpose |
|------|---------|---------|
| `session-start-prek-setup.sh` | Session start | Ensures prek is installed and git hooks are wired up |
| `pre-edit.sh` | Before any file edit | Blocks edits to generated/vendored files, warns on high-risk files |
| `stop-prek-validation.sh` | Session stop | Runs prek validation on changed files before stopping |

## Hook Details

### session-start-prek-setup.sh

Runs asynchronously at the start of each Claude Code session.

**Behavior:**
- If prek is not installed: prints guidance to stderr, does not block
- If prek is installed but hooks not wired: runs `prek install`
- If hooks already configured: prints confirmation

**Worktree support:** Uses `git rev-parse --git-path hooks` which works correctly in both regular repos and git worktrees (where `.git` is a file, not a directory).

### pre-edit.sh

Runs before every file edit (Edit, Write, MultiEdit tools).

**Blocks:**
- `zz_generated.*.go` - controller-gen output
- `pkg/pagerduty/mock_service.go`, `pkg/pagerduty/service_mock.go` - mockgen output
- `vendor/*` - go module vendor directory

**Warns (requires confirmation):**
- `deploy/crds/*.yaml` - generated CRD manifests
- `go.sum` - direct edits
- `boilerplate/*` - upstream-managed files
- RBAC files, Tekton pipelines, Dockerfile

### stop-prek-validation.sh

Runs when Claude Code stops with uncommitted changes.

**Default mode:** Runs only when there are uncommitted changes.

**Strict mode:** Set `export CLAUDE_LINT_ON_STOP=true` to always run.

**Performance:** Validates only changed files using `hack/prek.ci.toml` (skips network-dependent hooks).

## Configuring Hooks

Hooks are wired up in `.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Edit|Write|MultiEdit",
        "hooks": [{ "type": "command", "command": "bash ...pre-edit.sh..." }]
      }
    ],
    "SessionStart": [
      {
        "hooks": [{ "type": "command", "command": "bash ...session-start-prek-setup.sh...", "async": true }]
      }
    ],
    "Stop": [
      {
        "hooks": [{ "type": "command", "command": "bash ...stop-prek-validation.sh..." }]
      }
    ]
  }
}
```
