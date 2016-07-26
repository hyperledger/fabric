#!/usr/bin/env bash

if [ -f "crypto.test" ]
then
    echo Removing crypto.test
    rm crypto.test
fi

echo Compile, banchmark, pprof...

go test -c
#./crypto.test -test.cpuprofile=crypto.prof
#./crypto.test -test.bench BenchmarkConfidentialTCertHExecuteTransaction -test.run XXX -test.cpuprofile=crypto.prof
./crypto.test -test.bench BenchmarkTransaction -test.run XXX -test.cpuprofile=crypto.prof
#./crypto.test -test.run TestParallelInitClose -test.cpuprofile=crypto.prof
go tool pprof crypto.test crypto.prof