# Decision records (ADRs)

One short file per decision: `NNNN-<slug>.md`, numbered
chronologically. This file is the **index** — keep it telegraphic
(number, link, one-line consequence). Detail lives in the files.

## Format

```md
---
status: accepted | superseded | rejected
date: YYYY-MM-DD
---

# NNNN — <title>

## Context
## Decision
## Consequences
```

## Index

| # | Decision | Status | Consequence |
|---|---|---|---|
| 0001 | [State sync: manifest-based file sync](0001-state-sync.md) | accepted | manifest+blob protocol, LWW per file; transport swappable to Iroh later |
