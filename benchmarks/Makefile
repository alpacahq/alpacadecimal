.PHONY: bench bench-x view-bench-cpu-result view-bench-mem-result clean-up

bench:
	go test -bench=. --cpuprofile profile.out --memprofile memprofile.out

bench-x:
	go test -bench=Mul --cpuprofile profile.out --memprofile memprofile.out -benchtime 10s

view-bench-cpu-result:
	go tool pprof -http=":8000" profile.out

view-bench-mem-result:
	go tool pprof -http=":8000" memprofile.out

clean-up:
	-rm memprofile.out
	-rm profile.out
	-rm benchmarks.test