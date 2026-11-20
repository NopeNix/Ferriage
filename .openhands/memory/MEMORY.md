# Ferriage memory — index (read this first, then follow links)

> Index only. Keep under ~6000 chars (platform truncates older
> content first). One row per `log/*` file; detail lives there.
> Update this table in the same commit as any new log file.

| Date | File | Topic | Takeaway |
|------|------|-------|----------|
| 2026-11-20 | [001-harness-setup](log/2026-11-20-001-harness-setup.md) | harness | index+single-file memory/handover adopted; monoliths rejected |
| 2026-11-20 | [002-roadmap](log/2026-11-20-002-roadmap.md) | roadmap | phase 0→4 plan; low-record principle |
| 2026-11-20 | [003-phase0-confirmation](log/2026-11-20-003-phase0-confirmation.md) | phase0 | stack confirmed as-is; Docker Hub nopenix/ferriage; No Go on dev host yet |
| 2026-11-20 | [004-phase1-scaffold](log/2026-11-20-004-phase1-scaffold.md) | phase1 | 1.1 peer+healthz green; Go 1.25.5 user-local (no sudo) in ~/bin |
| 2026-11-20 | [005-phase1-2-adopt](log/2026-11-20-005-phase1-2-adopt.md) | phase1.2 | state dir + adopt by compose labels + /stacks; docker deps pinned to Go 1.25; /stacks/ gitignore anchor fix |
| 2026-11-20 | [006-phase1-3-sync](log/2026-11-20-006-phase1-3-sync.md) | phase1.3 | ADR 0001 manifest file-sync (not git); tombstone ledger + ed25519 identity in .ferriage/; go test `-count=1` after edits (cache replays stale failures) |
| 2026-11-20 | [008-phase1-4-finish](log/2026-11-20-008-phase1-4-finish.md) | phase1.4 | Docker attach stdin delivers 0 bytes here; volume data via copy APIs through sleeper container; ImageInspectWithRaw is lint-deprecated |
| 2026-11-20 | [009-phase1-5-deploy](log/2026-11-20-009-phase1-5-deploy.md) | phase1.5 | deploy = docker compose CLI + Engine-API health gate; compose --wait hides per-service state; Health none = healthy-once-running; unhealthy can flip back, only timeout is final |
| 2026-11-20 | [007-phase1-4-snapshot](log/2026-11-20-007-phase1-4-snapshot.md) | phase1.4 | ADR 0002 gzip+tar snapshot; docker volumes are root:700 — capture via helper-container exec; attach-stdin of unstarted container drops silently; exec-attach stdin is raw, stdout framed |
| 2026-11-20 | [010-phase1-6-compose-smoke](log/2026-11-20-010-phase1-6-compose-smoke.md) | phase1.6 | deploy/Dockerfile+compose, layer-2 smoke 16/0; sync is pull-only — write on the peer others pull from; host-network peer stability; kill whole source side during a move |
| 2026-11-20 | [011-phase2-ui-shell](log/2026-11-20-011-phase2-ui-shell.md) | phase2 | web/ Vite+React+Tailwind+shadcn; BFF OIDC gate (ADR 0004), Dex dev IdP (ADR 0003); openid-client v6 method-API; dev ports 18080/19080/18081 (8080=Open WebUI); layer-3 png-diff 19/0 |
| 2026-11-20 | [012-phase3-api-cli](log/2026-11-20-012-phase3-api-cli.md) | phase3 | typed API (internal/api + OpenAPI + drift test), cmd/ferriage CLI twin, StackDetail; full routine 29/0 |
| 2026-11-20 | [013-phase3-logs-stats-exec](log/2026-11-20-013-phase3-logs-stats-exec.md) | phase3 | ADR 0005: logs/stats/exec + WS console (hand-rolled RFC6455) + Monaco vendored offline + BFF raw upgrade pipe; errdefs deprecation; full routine 40/0 |
| 2026-11-20 | [014-phase3-ssh-onboarding](log/2026-11-20-014-phase3-ssh-onboarding.md) | phase3 | ADR 0006: native x/crypto/ssh, TO-FU store (missing/empty ≠ pinned), pull_policy missing; full routine 40/0 |
| 2026-11-20 | [015-phase3-shoutrrr](log/2026-11-20-015-phase3-shoutrrr.md) | phase3 | ADR 0007: Shoutrrr env-only (FERRIAGE_NOTIFY_URLS), async best-effort (never inherit request ctx), redacted GET /notify; full routine 40/0 |
| 2026-11-20 | [016-phase3-audit](log/2026-11-20-016-phase3-audit.md) | phase3 | ADR 0008: audit JSONL in .ferriage/, middleware-recorded mutating actions, actor via X-Ferriage-Actor (BFF strips+re-sets); full routine 46/0 |
| 2026-11-20 | [017-phase3-auto-update](log/2026-11-20-017-phase3-auto-update.md) | phase3 | ADR 0009: per-node digest-poll loop, poll/auto, stateful→poll, snapshot-first, Shoutrrr events; RepoDigests vs ID compare trap; stale go-run peer on :18082; full routine 46/0 |
