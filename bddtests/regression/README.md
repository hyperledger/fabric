## regression
Regression Test Suites scripts, and folders for: execution results for
significant releases, and
configuration scripts and supporting files in a variety of languages:

- [GO](https://github.com/hyperledger/fabric/bddtests/regression/go)
  language test scripts
- [NODE](https://github.com/hyperledger/fabric/bddtests/regression/node)
  language test scripts
  Note: a flexible-use performance engine, which can be configured and used
  for a variety of tests, is located in the
  [node/performance](https://github.com/hyperledger/fabric/bddtests/regression/node/performance)
  folder.  For examples how to use it, check there, or look for examples in the
  daily test suite and long-run test suite scripts.
- [results](https://github.com/hyperledger/fabric/bddtests/regression/results)
  folder containing logfiles and results of running the Test Suites on
  significant releases, for reference


### Continous Integration Setup
Continuous Integration team will execute the *daily test suite* each day when a
merge commit has been pushed to fabric repository.
We have configured Jenkins in vLaunch to execute the Daily Test Suite:
the jobs listed below are downstream jobs which are run after successfully
executing the upstream job.
After the build is successfully executed, Jenkins will post build results
back to **rel-criteria-build** slack channel,
and generate a test summary report, to be available for viewing for 30 days.
For significant releases, the results will be stored
in **bddtests/regression/results** folder.


### Daily Test Suite - daily_test_suite.sh
Expected total time duration is between 13 - 16 hours.

* Consensus Acceptance Tests (CAT) - using chaincode example02, GO, gRPC
  - Objective of CAT tests is to ensure the stability and resiliency of the
    BFT Batch design.
* Large Networks Basic API And Consensus Tests - using chaincode example02,
  GO, gRPC
  - Objective of Basic gRPC API test is to ensure basic gRPC API functions
    are working as expected
* Ledger Stress Tests (LST) for API and 20K runs with concurrency and
  1K payload - using chaincode addrecs, GO, gRPC
* Speed Tests for measuring turnaround performance of the communication path
  and blockchain network processing - using chaincode addrecs, GO, gRPC
* Concurrency Tests - using chaincode addrecs, GO, gRPC
  - Objective of Concurrency test is to ensure system is able to accept
    multiple threads concurrently for specified number of mins.
* Complex Transactions Tests - using chaincode auction, node, gRPC
  - Objective of this test is to send complex transactions using
    auction chaincode
* Performance Tests, random sized payloads - using chaincodes example02
  and auction, node, gRPC
  - CI performs below tests in Performance testing
     * Invoke on chaincode_example02 using 4 peers and
       4 Threads for 180 secs
     * Query on chaincode_example02 using 4 peers and
       4 Threads for 180 secs
     * Invoke on auction chaincode using 4 peers and
       1000 Tx for each of the 4 Threads
     * Query on auction chaincode using 4 peers and
       1000 Tx for each of the 4 Threads
     * Invoke on auction chaincode using 4 peers and 4 Threads.
       Each Invoke is followed by a Query on every Thread
     * Invoke on example02 chaincode using 4 peers and 4 Threads.
Each Invoke is followed by a Query on every Thread


### LongRun Test Suite - longrun_test_suite.sh
Expected total time duration is 72 hours when executed from
Jenkins automation scripts, which run several parallel jobs.

* Consensus peers restarts tests, 2 tests, each approximately 10 hours -
  using chaincode example02, GO, gRPC
* Ledger Stress Test 1 Million transactions, approximately 36 hours -
  using chaincode addrecs, GO, gRPC
* Ledger Stress Test, 72 hours - using chaincode addrecs, GO, gRPC
  - Send parallel Invoke requests on all 4 peers
* Performance Tests, 72 hours, random sized payloads - using chaincode auction,
  node, gRPC (includes variable traffic, concurrency, complex transactions)
  - Invoke 1 Tx per sec on 1 peer for 72 hrs (1 Thread)


### Full Regression Test Suite
Execute both the Daily Test Suite AND the Long Run Test Suite.


<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
s
