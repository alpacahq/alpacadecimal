.PHONY: all clean gen-asm clear-asm test bench bench-x bench-pgo pgo-profile view-bench-cpu-result view-bench-mem-result clean-up fuzz fuzz-seeds fuzz-all clean-corpus

gen-asm:
	go build -x -n -v *.go 2>&1 | sed -n "/^# import config/,/EOF$$/p" |grep -v EOF > importcfg
	go tool compile -importcfg importcfg -S $$(ls *.go | grep -v _test.go) > decimal.s

clear-asm:
	rm -f *.o
	rm -f decimal.s
	rm -f importcfg

test:
	go test .

bench:
	$(MAKE) -C benchmarks bench

bench-x:
	$(MAKE) -C benchmarks bench-x

# regenerate the default.pgo profile shipped at the repo root
pgo-profile:
	go run ./internal/pgogen -o default.pgo -d 25s

# measure the effect of the shipped profile on the benchmark suite
bench-pgo:
	$(MAKE) -C benchmarks bench-pgo

view-bench-cpu-result:
	$(MAKE) -C benchmarks view-bench-cpu-result

view-bench-mem-result:
	$(MAKE) -C benchmarks view-bench-mem-result

clean-up:
	$(MAKE) -C benchmarks clean-up

fuzz:
	$(MAKE) -C fuzz fuzz

fuzz-seeds:
	$(MAKE) -C fuzz fuzz-seeds

fuzz-all:
	$(MAKE) -C fuzz fuzz-all

clean-corpus:
	$(MAKE) -C fuzz clean-corpus
