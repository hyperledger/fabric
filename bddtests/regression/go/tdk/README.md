## tdk
Hyperledger Fabric Go language Test Development Kit (TDK): tests, scripts, and
supporting framework for testing hyperledger/fabric

## SETUP

###Obtain the TDK and test programs:
In v1.0, find them within hyperledger/fabric repository.

        $ cd .../fabric/bddtests/regression/go/tdk
        $ git checkout master

In v0.6, clone from private repo to the src directory where GO is
installed (use either $GOROOT/src or $GOPATH/src).

        $ cd $GOPATH/src
        $ git clone https://github.ibm.com/IBM-Blockchain/release-criteria
        $ cd release-criteria/fvt/consensus
        $ git checkout v0.6

Bash scripts local_fabric_gerrit.sh and others will create docker containers
to run the peers. For more information on setting up a peer network,
[read these instructions] (https://github.com/rameshthoomu/fabric1/blob/tools/localpeersetup/local_Readme.md)

## USAGE

###How to execute the programs:
- If you wish to connect to an existing network, change the credentials in
  NetworkCredentials.json as needed.
- Helpful shell scripts are located in the tdk/automation directory:
```
        ../automation/go_record.sh <tests.go>   - execute this from any of the
        test directories, to run go tests and record stdout logs in unique
        GO_TEST* named files in the current working directory.
```
- LOGFILES for all Peers can be saved in the automation directory by
  uncommenting a few lines in the local_fabric_gerrit.sh script. (Search for
  LOGFILE.) Execute go_record.sh (or local_fabric.sh or similar script) without
  parameters to get help with the options to control the log detail level.
- Go to the test directories and execute the tests. Good luck!
```
        #  Examples how you can preload some of the environment vars for local
        #  vagrant/docker network testing:
        $  .   ../automation/ENVVARS_LOCAL
        $  .   ../automation/ENVVARS_Z
        # OR, for HSBN/BlueMix network:
        $  .   ../automation/ENVVARS_Z
        $  export COMMIT=<commit>

        # To run a basic test to create a 4-peer network and exercise the
        # REST API functionalities:
        $ cd tdk/chcotest
        $ go run BasicFuncNewNetwork.go

        # To run Consensus Acceptance Tests or two of the long-run tests:
        $ cd tdk/CAT
        $ ../automation/go_record.sh CAT*go
        $ ../automation/go_record.sh CRT_501_StopAndRestartRandom_10Hrs.go
        $ ../automation/go_record.sh CRT_502_StopAndRestart1or2_10Hrs.go

        # To run ledger stress tests: After first starting up a new network by
        # running a test that creates and uses a networkcredentials file
        # (which maps to tdk/util/NetworkCredentials.json),
        # simply run these tests by:
        $ cd tdk/CAT
        $ go run testtemplate.go
        $ cd tdk/ledgerstresstest
        $ TEST_NETWORK=LOCAL go run LST_2Client2Peer20K.go

        # Or, here is the easiest way to run the basic API test and five
        # concurrency testcases in this directory:
        $ cd tdk/ledgerstresstest
        $ ./run_LST_regression.sh

        # To run COMMIT=821a3c7, the v0.6 Sep 7th build, in local environment
        # with one of these commands:
        $ cd tdk/automation
        $ ./local_fabric_gerrit.sh -c 821a3c7 -n 4 -f 1 -l error -m pbft -b 500 -s
        $ export COMMIT=821a3c7; export REPOSITORY_SOURCE=GERRIT
        $ go_record.sh ../CAT/testtemplate.go ../chcotest/BasicFuncNewNetwork.go
```
###Running tests on remote network requires some tweaking to make things run.
- Put the IP address info of the peers into the util/NetworkCredentials.json;
  use script update_z.py to help generate that file for BlueMix networks.
- Set environment variable TEST_NET_COMM_PROTOCOL to use HTTPS instead of
  default HTTP; required for Ledger Stress Tests on Z network, and may also
  be useful when running other tests in other networks that could use HTTPS,
- Define its own usernames/passwords (may need to edit threadutil/threadutil.go)
- (Ledger Stress Tests only): Set environment variable TEST_NETWORK to Z when
  using the Z network and its usernames/passwords
- (Ledger Stress Tests only): set the correct chaincode name in lstutil/util.go
  to change it from "mycc" (a special version of example02 that adds payloads
  records the ledger, instead of just changing the values of A and B) to
  whatever is deployed on the actual Z network you are using. Before running
  any tests, rebuild:  "cd ledgerstresstest; ../automation/go_build_all.sh"
```
        # For example:

        $ cd ../CAT; TEST_NET_COMM_PROTOCOL=HTTPS go run CAT_101*.go

        # To run tests with example02 in BlueMix network on
        # v.5 (COMMIT=3e0e80a) or v.6 (COMMIT=821a3c7),
        # download the ServiceCredentials from BlueMix and run script
        # update_z.py to generate the networkcredentials file for the
        # Z or Starter or HSBN network; then try one of:

        $ ./update_z.py -b -f <service_credentials_file>
        $ cp networkcredentials ../util/NetworkCredentials.json
        $ export TEST_NET_COMM_PROTOCOL=HTTPS; export TEST_NETWORK=Z
        $     export COMMIT=821a3c7; export REPOSITORY_SOURCE=GERRIT
        #   OR
        $     export COMMIT=3e0e80a; export REPOSITORY_SOURCE=GITHUB
        $ go run ../chcotest/BasicFuncExistingNetwork.go;
        $ go_record.sh ../CAT/testtemplate.go

        # To run ledger stress tests with a different chaincode named
        # example02_addrecs, which is a modified version of example02,
        # in BlueMix network on v05 or v06:
        $ cd ../ledgerstresstest
        $ export TEST_NETWORK=Z ; export TEST_NET_COMM_PROTOCOL=HTTPS
        $ go run BasicFuncExistingNetworkLST.go
        $ go run LST_1Client1Peer20K.go
```

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
s
