A framework to verify Hyperledger-Fabric functionality using chaincodes and CLI 

Multiple tests spanning multiple chaincodes can be run.
Supports XML Reporting for each test run


Write and Test Chaincodes
----------------------------------------------------------
Users can write their own GO chaincodes that implements one or more fabric features.
These chaincode calls can then be verified using e2e CLI bash scripts


How to test your chaincode
----------------------------------------------------------------------

* Place your "Go" chaincode under test/regression/daily/chaincodeTests/fabricFeatureChaincodes/go
* Place your CLI shell script under test/regression/daily/chaincodeTests/fabricFeatureChaincodes
* Add a new definition for your test script inside test/regression/daily/chaincodeTests/envsetup/scripts/fabricFeaturerChaincodeTestiRuns.py

cd test/regression/daily/chaincodeTests 
./runChaincodes.sh

===========================================================================


runChaincodes.sh calls network_setup.sh under test/regression/daily/chaincodeTests/envsetup


.. code:: bash

 ./network_setup.sh <up|down|retstart> [channel-name] [num-channels] [num-chaincodes] [endorsers count] [script_name]

channel_name - channel prefix
num-channels - default 1
num-chaincodes - default 1
endorsers count - 4
script_name - script that has test runs in it



output of each of the steps when executing chaincode_example02 looks like

.. code:: bash

Running tests...
----------------------------------------------------------------------

./scripts/e2e_test_example02.sh myc 1 1 4 create
./scripts/e2e_test_example02.sh myc 1 1 4 join
./scripts/e2e_test_example02.sh myc 1 1 4 install
./scripts/e2e_test_example02.sh myc 1 1 4 instantiate
./scripts/e2e_test_example02.sh myc 1 1 4 invokeQuery

Ran 1 test in 135.392s



OK



To deactivate docker network

cd envsetup

.. code:: bash

  ./network_setup.sh down
                                                                                                                                                                                           95,1          97%


