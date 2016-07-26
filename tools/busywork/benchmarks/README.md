# Benchmarks

This directory contains benchmarks, and a Makefile to run the benchmarks.

### crypto-ubench

This target runs a set of microbenchmarks for the Go cryptographic primitives
used by the Hyperledger fabric. Simply execute

    make crypto-ubench
	
to run the microbenchmarks. You can define `TIMES` on the make command line to
define how many times each run is made. The default is 1 run; If `TIMES` > 1
then the results are averaged. Define `ENV` to define an environment or
wrapper-command. For example

    make crypto-ubench ENV=time TIMES=3

runs each benchmark 3 times and also reports the elapsed time.
