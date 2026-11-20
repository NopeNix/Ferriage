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
| 2026-11-20 | [007-phase1-4-snapshot](log/2026-11-20-007-phase1-4-snapshot.md) | phase1.4 | ADR 0002 gzip+tar snapshot; docker volumes are root:700 — capture via helper-container exec; attach-stdin of unstarted container drops silently; exec-attach stdin is raw, stdout framed |
