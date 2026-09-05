---
type: Architecture Decision
title: ADR-0001 int64 fixed-point representation with 12 decimal places and shopspring fallback
description: Represent decimals as an int64 scaled by 1e12 and fall back to shopspring/decimal when the value does not fit.
status: accepted
date: unknown
deciders: []
supersedes: []
affects: [alpacadecimal, alpacadecimal.lib]
allium: []
evidence: [README.md, decimal.go, doc/value-slowness.png]
tags: [alpacahq, adr, generated]
timestamp: 2026-09-05T11:35:41Z
generated_by: claude-fable-5 / layered-docs 2026-09
source_commit: 7ce95bc0f8b064aa0b9b81a4cdf8ef2300fde341
source_branch: docs/layered-2026-09
generated_at: 2026-09-05T11:35:41Z
confidence: high
review_status: draft-needs-review
---

# ADR-0001 int64 fixed-point representation with 12 decimal places and shopspring fallback

## Context
Profiling of Alpaca services showed that `shopspring/decimal` spends significant CPU and memory in `big.Int` operations (SQL serialisation/deserialisation, addition, multiplication) (`README.md`, Key Ideas; `doc/value-slowness.png`). Upstream Go issues on `big.Int.String` slowness and `big.NewInt` allocation are cited as related (`README.md`, Related Issues). Alpaca's data sets are described as 99% within ten million with up to 12 decimal places (`README.md`, Goal).

## Decision
Store most decimals as an `int64` fixed at 12 decimal places (`fixed = 1_230_000_000_000` for 1.23) and keep a `*decimal.Decimal` fallback pointer for values that are too large, too small or too precise (`README.md`, Key Ideas; `decimal.go` `precision = 12`, `scale = 1e12`, `maxInt`, `minInt`). The README states 12 places were chosen because they cover 99% of Alpaca's common cases; the code comment notes the precision is tunable (more precision means a smaller maximum integer).

## Consequences
- Supported range on the fast path is roughly -9,223,372 to 9,223,372 with 12 decimal places; anything else routes to the fallback (`README.md`, `decimal.go`).
- Benchmarks report 5x to 100x speedups over shopspring for the common case (`README.md`, Benchmark).
- `Exponent`, `Coefficient`, `CoefficientInt64` and `NumDigits` return different (still valid) values than shopspring on the optimised path because the exponent is always -12 (`README.md`, Compatibility).
- A cached string table for -1000.00..1000.00 further accelerates `String`/`Value` (`decimal.go` valueCache comment).

## Alternatives considered
The benchmark module compares against `github.com/ericlagergren/decimal` and `github.com/quagmt/udecimal` (`benchmarks/go.mod`, `benchmarks/benchmark_test.go`), which indicates they were evaluated as alternatives; no written rationale for rejecting them exists in the repository.
