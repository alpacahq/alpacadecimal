---
type: System
title: alpacadecimal
description: Go library providing a Decimal type compatible with shopspring/decimal but optimised (int64 fixed-point, 12 decimal places) for Alpaca data sets; used by Alpaca Go services.
resource: likec4://alpacahq/alpacadecimal
tags: [alpacahq, library, decimal, go, generated]
status: active
owners: []
timestamp: 2026-09-05T11:35:41Z
generated_by: claude-fable-5 / layered-docs 2026-09
source_commit: 7ce95bc0f8b064aa0b9b81a4cdf8ef2300fde341
source_branch: docs/layered-2026-09
generated_at: 2026-09-05T11:35:41Z
confidence: high
review_status: draft-needs-review
links:
  repository: https://github.com/alpacahq/alpacadecimal
  architecture: ../architecture/alpacadecimal.c4
---

# alpacadecimal

## Purpose
alpacadecimal is a Go library that offers a `Decimal` type "similar and compatible with" `shopspring/decimal` but optimised for Alpaca's data sets, where 99% of decimals are within ten million with up to 12 decimal places (`README.md`, Goal). The upstream package's `big.Int` operations (SQL serialisation, addition, multiplication) were a CPU and memory bottleneck in Alpaca's profiling (`README.md`, Key Ideas; `doc/value-slowness.png`). The library therefore stores most values as an `int64` fixed at 12 decimal places and falls back to a `*decimal.Decimal` when the value is too large, too small or too precise (`README.md`; `decimal.go` constants `precision`, `scale`, `maxInt`).

It is meant to be a drop-in replacement for existing `decimal.Decimal` usage in Alpaca's Go services (`README.md`, Goal). It is a library only: there is no server process, container image or deployment.

## Capabilities
- Construction from int, float, string, shopspring decimal (`New`, `NewFromInt`, `NewFromString`, `RequireFromString`, `NewFromDecimal`) — evidence: `decimal.go`, `README.md` (Benchmark section)
- Arithmetic (Add, Sub, Mul, Div), comparison, rounding and truncation on the int64 fast path with shopspring fallback — evidence: `decimal.go`, `decimal_test.go`
- String formatting with a value cache for -1000.00..1000.00 — evidence: `decimal.go` (valueCache comment), `README.md` (Cached_Case benchmark)
- JSON, Text, Binary and Gob marshalling/unmarshalling — evidence: `decimal.go` (`MarshalJSON`, `MarshalText`, `MarshalBinary`, `GobEncode` and counterparts)
- database/sql integration via `driver.Valuer` and `sql.Scanner` — evidence: `decimal.go` (`Value`, `Scan`; import `database/sql/driver`)
- Documented behavioural differences from shopspring for `Exponent`, `Coefficient`, `CoefficientInt64`, `NumDigits` (exponent is always -12 on the optimised path) — evidence: `README.md` (Compatibility)

## Interfaces
**Inbound:** Go package API of `github.com/alpacahq/alpacadecimal` (`go.mod`, `decimal.go`). No network, CLI or UI interface.
**Outbound:** none at runtime. The package calls into the `github.com/shopspring/decimal` library for fallback cases (`decimal.go` import; `go.mod`).

## Dependencies
- `github.com/shopspring/decimal` v1.4.0 — fallback representation and API compatibility target — evidence `go.mod`, `decimal.go`
- `github.com/stretchr/testify` v1.10.0 — tests only — evidence `go.mod`, `decimal_test.go`
- Benchmark-only (separate module): `github.com/ericlagergren/decimal`, `github.com/quagmt/udecimal`, `github.com/shopspring/decimal` — evidence `benchmarks/go.mod`

No external systems (cloud, datastore, messaging, vendor APIs) are referenced by the repository; the org-level `ext_*` dictionary is therefore not used by this model.

## Data & storage
None. The library holds values in memory (`Decimal{fixed int64; fallback *decimal.Decimal}`, `README.md`, `decimal.go`). It serialises to/from SQL driver values, JSON, text, binary and gob but owns no store.

## Operations
- Build/test: `make test` runs `go test .`; `make bench` delegates to `benchmarks/Makefile` (`go test -bench=.` with cpu/mem profiles viewable via `go tool pprof`) — evidence `Makefile`, `benchmarks/Makefile`
- CI: GitHub Actions workflow on every push, matrix Go 1.18 and 1.19, steps build / vet / test — evidence `.github/workflows/go.yml`
- Release: consumed as a versioned Go module (benchmarks pin `v0.0.8`; commit 7ce95bc quotes a consumer on `alpacadecimal@v0.0.8`) — evidence `benchmarks/go.mod`, git log
- No Dockerfile, compose, Kubernetes or Terraform in the repository (`git ls-files`), so no deployment model is provided.

## Behaviour (Allium)
- none — the repository contains no `.allium` specification and no accepted spec exists for it (`git ls-files`).

## Decisions
- [ADR-0001 int64 fixed-point representation with 12 decimal places](decisions/ADR-0001-int64-fixed-point-with-fallback.md)
- [ADR-0002 API compatibility with shopspring/decimal as a drop-in replacement](decisions/ADR-0002-shopspring-drop-in-compatibility.md)
- [ADR-0003 Truncate is a no-op for precision of 12 or more](decisions/ADR-0003-truncate-noop-at-high-precision.md)

## Evidence
- `README.md` — goal, key ideas, compatibility notes, benchmark output
- `go.mod`, `benchmarks/go.mod` — module identity and dependencies
- `decimal.go`, `decimal_test.go` — implementation and tests
- `Makefile`, `benchmarks/Makefile` — build, test and benchmark targets
- `.github/workflows/go.yml` — CI
- `doc/value-slowness.png` — profiling result motivating the library
- git log (commit 7ce95bc, "CRYP-2334: Don't panic on high-precision Truncate (#23)")

## Open questions
- Owners: no CODEOWNERS or maintainer statement in the repository; `owners` left empty (unverified).
- Consumers: the README states the library is a drop-in replacement for Alpaca services and commit 7ce95bc references one downstream service, but the full list of consuming repositories is not evidenced here.
- Status: marked `active` because the latest commit is dated 2026-03-16 (git log); no deprecation notice exists. Unverified beyond that.
- The CI matrix tests Go 1.18/1.19 while `benchmarks/go.mod` requires Go 1.23; whether newer Go versions are officially supported is not stated.
