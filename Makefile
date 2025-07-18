.PHONY: all clean gen-asm clear-asm test bench bench-x view-bench-cpu-result view-bench-mem-result clean-up

gen-asm:
	go build -x -n -v *.go 2>&1 | sed -n "/^# import config/,/EOF$$/p" |grep -v EOF > importcfg
	go tool compile -importcfg importcfg -S decimal.go > decimal.s

clear-asm:
	rm decimal.o
	rm decimal.s
	rm importcfg

test:
	go test .

bench:
	$(MAKE) -C benchmarks bench

bench-x:
	$(MAKE) -C benchmarks bench-x

view-bench-cpu-result:
	$(MAKE) -C benchmarks view-bench-cpu-result

view-bench-mem-result:
	$(MAKE) -C benchmarks view-bench-mem-result

clean-up:
	$(MAKE) -C benchmarks clean-up
