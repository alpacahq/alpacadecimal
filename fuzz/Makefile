FUZZ_TIME ?= 10s
FUZZ_TIMEOUT ?= 60s

FUZZ_TARGETS = \
	FuzzArithmetic \
	FuzzRounding \
	FuzzStringFixed \
	FuzzIntrospection \
	FuzzConstructors \
	FuzzComparisons \
	FuzzCeilFloor \
	FuzzNewFromInt \
	FuzzNewFromFloat \
	FuzzNewFromFloat32 \
	FuzzShift \
	FuzzPow \
	FuzzDivRound \
	FuzzQuoRem \
	FuzzAggregates \
	FuzzExponentCoefficient \
	FuzzBigIntBigFloatRat \
	FuzzStringFormats \
	FuzzRoundCash \
	FuzzNewFromFloatWithExponent \
	FuzzNewFromBigInt \
	FuzzSerialization \
	FuzzNullDecimal \
	FuzzRequireFromString \
	FuzzGetFixedIsOptimized \
	FuzzNewFromFormattedString \
	FuzzScanTypes

.PHONY: fuzz fuzz-seeds fuzz-all clean-corpus

# Run all fuzz target seeds as regular tests (fast sanity check)
fuzz-seeds:
	go test -run "Fuzz" -count=1 -timeout $(FUZZ_TIMEOUT)

# Fuzz all targets sequentially for FUZZ_TIME each
fuzz-all:
	@for target in $(FUZZ_TARGETS); do \
		echo "=== Fuzzing $$target for $(FUZZ_TIME) ==="; \
		go test -fuzz="^$${target}$$" -fuzztime=$(FUZZ_TIME) -timeout $(FUZZ_TIMEOUT) || exit 1; \
	done

# Alias: run seeds then fuzz all
fuzz: fuzz-seeds fuzz-all

# Remove cached fuzz corpus
clean-corpus:
	-rm -rf testdata/fuzz
