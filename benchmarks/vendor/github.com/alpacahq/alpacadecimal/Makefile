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
	go test -bench=. --cpuprofile profile.out --memprofile memprofile.out

bench-x:
	go test -bench=Mul --cpuprofile profile.out --memprofile memprofile.out -benchtime 10s

view-bench-cpu-result:
	go tool pprof -http=":8000" profile.out

view-bench-mem-result:
	go tool pprof -http=":8000" memprofile.out
