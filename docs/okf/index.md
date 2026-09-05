# alpacadecimal — knowledge bundle (OKF)

Generated draft (layered-docs 2026-09) — review required. Concept id = path without `.md`.

- [system](system.md) — System: Go Decimal library compatible with shopspring/decimal, optimised (int64 fixed-point, 12 places) for Alpaca data sets
- [decisions/ADR-0001-int64-fixed-point-with-fallback](decisions/ADR-0001-int64-fixed-point-with-fallback.md) — Architecture Decision: int64 fixed-point with 12 decimal places and shopspring fallback
- [decisions/ADR-0002-shopspring-drop-in-compatibility](decisions/ADR-0002-shopspring-drop-in-compatibility.md) — Architecture Decision: API compatibility with shopspring/decimal as a drop-in replacement
- [decisions/ADR-0003-truncate-noop-at-high-precision](decisions/ADR-0003-truncate-noop-at-high-precision.md) — Architecture Decision: Truncate is a no-op for precision of 12 or more
