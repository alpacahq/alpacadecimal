---
type: Architecture Decision
title: ADR-0003 Truncate returns the value unchanged for precision of 12 or more
description: On the int64 fast path, Truncate with precision at or above 12 returns early instead of indexing the power-of-ten table.
status: accepted
date: 2026-03-16
deciders: []
supersedes: []
affects: [alpacadecimal.lib]
allium: []
evidence: [decimal.go, "git log 7ce95bc0f8b064aa0b9b81a4cdf8ef2300fde341 (CRYP-2334, PR #23)"]
tags: [alpacahq, adr, generated]
timestamp: 2026-09-05T11:35:41Z
generated_by: claude-fable-5 / layered-docs 2026-09
source_commit: 7ce95bc0f8b064aa0b9b81a4cdf8ef2300fde341
source_branch: docs/layered-2026-09
generated_at: 2026-09-05T11:35:41Z
confidence: high
review_status: draft-needs-review
---

# ADR-0003 Truncate returns the value unchanged for precision of 12 or more

## Context
A downstream service calling `RequireFromString("1").Truncate(18)` panicked with "index out of range [-6]": the value stayed on the int64 fast path (no shopspring fallback) and a precision above the fixed 12 places produced a negative index into the power-of-ten table (commit 7ce95bc, "CRYP-2334: Don't panic on high-precision Truncate (#23)").

## Decision
When the requested precision is 12 or higher on the fast path, `Truncate` returns early without truncating, since the stored value already has at most 12 decimal places (commit 7ce95bc message; `decimal.go` `Truncate`).

## Consequences
- `Truncate(n)` for n >= 12 is a no-op on optimised values and no longer panics (commit 7ce95bc).
- Behaviour for values held in the shopspring fallback is unchanged (commit 7ce95bc describes the fast-path case only).
