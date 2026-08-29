After removal of all regexp, on a Raspberry Pi 4 (for context):

```bash
❯ go test -bench=. ./...
goos: linux
goarch: arm64
pkg: github.com/jig/lisp
BenchmarkLoadSymbols-4                         	        3325        332901 ns/op
BenchmarkMAL1-4                                	        5253        202188 ns/op
BenchmarkMAL2-4                                	       41412         28911 ns/op
BenchmarkParallelREAD-4                        	      135973          8880 ns/op
BenchmarkParallelREP-4                         	       85090         14754 ns/op
BenchmarkREP-4                                 	       40946         29273 ns/op
BenchmarkFibonacci-4                           	         391       3041926 ns/op
BenchmarkParallelFibonacci-4                   	        1695        665055 ns/op
BenchmarkAtomParallel-4                        	       46522         25724 ns/op
BenchmarkAddPreamble-4                         	      152671          7246 ns/op
BenchmarkAddPreambleAlternative-4              	       47991         24526 ns/op
BenchmarkREADWithPreamble-4                    	       13071         94109 ns/op
BenchmarkNewEnv-4                              	      108345         10661 ns/op
BenchmarkCompleteSendingWithPreamble-4         	        1966        577461 ns/op
BenchmarkCompleteSendingWithPreambleSolved-4   	        1516        710604 ns/op
PASS
```
