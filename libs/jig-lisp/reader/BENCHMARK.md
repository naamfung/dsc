Pre removal of regexp, on a MacBook Pro M1:

```bash
❯ go test -bench=. ./... -benchtime=10s
goos: darwin
goarch: arm64
pkg: github.com/jig/lisp
BenchmarkLoadSymbols-10                          	  297692	     38451 ns/op
BenchmarkMAL1-10                                 	  217472	     54719 ns/op
BenchmarkMAL2-10                                 	 1615966	      7416 ns/op
BenchmarkParallelREAD-10                         	 3756685	      3244 ns/op
BenchmarkParallelREP-10                          	 2838428	      4265 ns/op
BenchmarkREP-10                                  	 1597930	      7488 ns/op
BenchmarkFibonacci-10                            	   30750	    388149 ns/op
BenchmarkParallelFibonacci-10                    	   58689	    203702 ns/op
BenchmarkAtomParallel-10                         	 4544056	      2621 ns/op
BenchmarkAddPreamble-10                          	12085546	       989.0 ns/op
BenchmarkAddPreambleAlternative-10               	 3712512	      3228 ns/op
BenchmarkREADWithPreamble-10                     	  459700	     25936 ns/op
BenchmarkNewEnv-10                               	 8883668	      1328 ns/op
BenchmarkCompleteSendingWithPreamble-10          	  127327	     94265 ns/op
BenchmarkCompleteSendingWithPreambleSolved-10    	   91657	    130333 ns/op
PASS
```

After removal of the main regexp, on a MacBook Pro M1:

```bash
❯ go test -bench=. ./... -benchtime=10s
goos: darwin
goarch: arm64
pkg: github.com/jig/lisp
BenchmarkLoadSymbols-10                               302422         38421 ns/op
BenchmarkMAL1-10                                      359725         32829 ns/op
BenchmarkMAL2-10                                     2706368          4423 ns/op
BenchmarkParallelREAD-10                             4663735          2565 ns/op
BenchmarkParallelREP-10                              3700264          3086 ns/op
BenchmarkREP-10                                      2638774          4507 ns/op
BenchmarkFibonacci-10                                  32196        373477 ns/op
BenchmarkParallelFibonacci-10                          61792        213475 ns/op
BenchmarkAtomParallel-10                             4321690          2785 ns/op
BenchmarkAddPreamble-10                             12032408           991.9 ns/op
BenchmarkAddPreambleAlternative-10                   3557404          3225 ns/op
BenchmarkREADWithPreamble-10                          781752         14996 ns/op
BenchmarkNewEnv-10                                   9081198          1321 ns/op
BenchmarkCompleteSendingWithPreamble-10               155209         76409 ns/op
BenchmarkCompleteSendingWithPreambleSolved-10         122361         97650 ns/op
PASS
```

After removal of all regexp, on a MacBook Pro M1:

```bash
❯ go test -bench=. ./... -benchtime=10s
goos: darwin
goarch: arm64
pkg: github.com/jig/lisp
BenchmarkLoadSymbols-10                               300728         38209 ns/op
BenchmarkMAL1-10                                      423998         27683 ns/op
BenchmarkMAL2-10                                     3133608          3820 ns/op
BenchmarkParallelREAD-10                             4903840          2381 ns/op
BenchmarkParallelREP-10                              4721929          2533 ns/op
BenchmarkREP-10                                      3055911          3884 ns/op
BenchmarkFibonacci-10                                  32557        368213 ns/op
BenchmarkParallelFibonacci-10                          54900        218110 ns/op
BenchmarkAtomParallel-10                             4663047          2552 ns/op
BenchmarkAddPreamble-10                             12042734           982.6 ns/op
BenchmarkAddPreambleAlternative-10                   3715250          3210 ns/op
BenchmarkREADWithPreamble-10                          817574         13132 ns/op
BenchmarkNewEnv-10                                   9085759          1310 ns/op
BenchmarkCompleteSendingWithPreamble-10               163772         73314 ns/op
BenchmarkCompleteSendingWithPreambleSolved-10         127953         91924 ns/op
PASS
```
