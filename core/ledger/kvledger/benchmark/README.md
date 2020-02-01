# Ledger Benchmark Tests

This readme explains the Benchmarks in this package and how to run them.

- What are the Ledger Benchmark tests
- How to Run the Tests

## What are the Ledger Benchmark tests

The Ledger Benchmark tests provide a way to measure the performance of the Ledger
component with a flexible and controlled workload. These benchmarks short-circuit
chaincode and go directly to the transaction simulator. Also, for block creation,
these benchmarks use a simple in-memory block-cutter.

These benchmarks first run insert transactions to populate the data and then run the 
read-write transactions on the populated data. The primary purpose of these benchmarks is
to measure the throughput capacity of the ledger component and how it changes for a given
workload.

## How to Run The tests
In order to run the benchmarks, run the following command from folder fabric/core/ledger/kvledger/benchmark/scripts
```
./runbenchmarks.sh -f <test_parameter_file>.sh
```
The <test_parameter_file> is expected to contain the parameters for the benchmarks. A sample file (sample_params.sh) is provided.
For running the bechmarks, it is advised to make a copy of the file sample_params.sh and change the parameters that you want to run the tests with. For more details on the parameters and the experiment results, see comments in the file sample_params.sh