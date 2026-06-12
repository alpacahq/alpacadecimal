.PHONY: bench bench-x bench-pgo view-bench-cpu-result view-bench-mem-result clean-up

update-vendor:
	go mod tidy
	go mod vendor

bench: update-vendor
	go test -benchtime 1s -count 10 -bench=. --cpuprofile profile.out --memprofile memprofile.out

bench-x:
	go test -bench=Mul --cpuprofile profile.out --memprofile memprofile.out -benchtime 10s

# compare benchmarks built without and with the shipped PGO profile
# (requires benchstat: go install golang.org/x/perf/cmd/benchstat@latest)
bench-pgo:
	go test -pgo=off -bench=. -benchtime 0.1s -count 6 | tee pgo-off.txt
	go test -pgo=../default.pgo -bench=. -benchtime 0.1s -count 6 | tee pgo-on.txt
	benchstat pgo-off.txt pgo-on.txt

view-bench-cpu-result:
	go tool pprof -http=":8000" profile.out

view-bench-mem-result:
	go tool pprof -http=":8000" memprofile.out

clean-up:
	-rm memprofile.out
	-rm profile.out
	-rm benchmarks.test
