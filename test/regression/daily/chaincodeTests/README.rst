Chaincode Test Framework
==========================================================================

Framework Capabilities
------------------------------------------------------------------------

* Verifies Hyperledger-Fabric features by executing your chaincode
* Supports any chaincodes written in **GO**
* Spins up a docker network and runs CLI test scripts
* Tests all contributed chaincodes as part of our daily Continuous Integration (CI) test suite, and displays XML Reports for each chaincodee


How to Plug In *YourChaincode*
---------------------------------------------------------------------------

* Using **GO** language, write *YourChaincode* and place it at *fabric/test/regression/daily/chaincodeTests/fabricFeatureChaincodes/go/<YourChaincodeDir>/<YourChaincode.go>*
* Write a CLI bash script such as *e2e_test_<YourChaincode>.sh* to execute invokes and queries using your chaincode API, and place it under *fabric/test/regression/daily/chaincodeTests/fabricFeatureChaincodes/*

  * Simply model it after others, such as *e2e_test_example02.sh*
  * During the install step, give the path to chaincode as *github.com/hyperledger/fabric/test/regression/daily/chaincodeTests/fabricFeatureChaincodes/go/<YourChaincodeDir>*
  * Note: **test_AFAB3843_Create_Join_Channel** must be run as first test step

* Add a few lines to define your new test inside *test/regression/daily/chaincodeTests/envsetup/testYourChaincode.py*

  * Copy the few lines from another example such as 'def test_example02', and simply change the name and path

===========================================================================


How to Run the Chaincode Tests
------------------------------------------------------------------------

    ``$ cd fabric/test/regression/daily/chaincodeTests``
    ``$ ./runChaincodes.sh``

      *runChaincodes.sh* calls *testYourChaincode.py*

        ``py.test -v --junitxml YourChaincodeResults.xml testYourChaincode.py``

  The output should look like the following:

  ::

    =========================
    test session starts
    =========================
    platform linux2 -- Python 2.7.12, pytest-2.8.7, py-1.4.31, pluggy-0.3.1 -- /usr/bin/python
    cachedir: .cache
    rootdir: /opt/gopath/src/github.com/hyperledger/fabric/test/regression/daily/chaincodeTests/envsetup, inifile:
    collected 3 items
    testYourChaincode.py::ChaincodeAPI::test_AFAB3843_Create_Join_Channel PASSED
    testYourChaincode.py::ChaincodeAPI::test_FAB3791_example03 PASSED
    testYourChaincode.py::ChaincodeAPI::test_FAB3792_marbles02 PASSED


Logs
-------------------------------------------------------------------------------
Logs from each test script are redirected and stored at envsetup/scripts/output.log


Deactivate Network
-------------------------------------------------------------------------------
The network is automatically deactivated as part of the teardown step after running all tests.
In case of trouble, here is how to deactivate docker network manually:

    ``cd /path/to/fabric/test/regression/daily/chaincodeTests/envsetup``
     ``./network_setup.sh down``

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
