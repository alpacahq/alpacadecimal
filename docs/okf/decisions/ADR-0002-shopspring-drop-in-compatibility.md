---
type: Architecture Decision
title: ADR-0002 API compatibility with shopspring/decimal as a drop-in replacement
description: Keep the public API compatible with shopspring/decimal so existing usage can switch by changing the import.
status: accepted
date: unknown
deciders: []
supersedes: []
affects: [alpacadecimal.lib]
allium: []
evidence: [README.md, go.mod, decimal.go, decimal_test.go]
tags: [alpacahq, adr, generated]
timestamp: 2026-09-05T11:35:41Z
generated_by: claude-fable-5 / layered-docs 2026-09
source_commit: 7ce95bc0f8b064aa0b9b81a4cdf8ef2300fde341
source_branch: docs/layered-2026-09
generated_at: 2026-09-05T11:35:41Z
confidence: high
review_status: draft-needs-review
---

# ADR-0002 API compatibility with shopspring/decimal as a drop-in replacement

## Context
Alpaca's Go services already use `decimal.Decimal` from `shopspring/decimal`; the performance problem (ADR-0001) had to be solved without a rewrite of every call site (`README.md`, Goal: "drop-in replacement for current decimal.Decimal usage").

## Decision
Mirror the `shopspring/decimal` API (constructors, arithmetic, marshalling, `driver.Valuer`/`sql.Scanner`) and use `shopspring/decimal` itself as the fallback implementation so that behaviour is identical outside the optimised range (`README.md`, Compatibility; `go.mod` dependency; `decimal.go`). Documented exceptions are limited to `Exponent`, `Coefficient`, `CoefficientInt64` and `NumDigits`.

## Consequences
- `shopspring/decimal` remains a hard dependency of the package (`go.mod`).
- Consumers must accept the documented differences in exponent/coefficient reporting (`README.md`, Compatibility).
- Tests compare behaviour against shopspring using testify (`decimal_test.go`, `go.mod`).
