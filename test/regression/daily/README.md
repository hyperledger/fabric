# Daily Test Suite

This readme explains everything there is to know about our daily regression test suite.  **Note 1**: This applies similarly for both the *test/regression/daily/* and *test/regression/weekly/* test suites.  **Note 2**: The Release Criteria (*test/regression/release/*) test suite is a subset of all the Daily and Weekly tests.

- How to Run the Tests
- Where to View the Results produced by the daily automation tests
- Where to Find Existing Tests
- How to Add New Tests to the Automated Test Suite
  * Why Test Output Format Must Be **xml** and How to Make It So
  * Alternative 1: Add a test using an existing tool and test driver script
  * Alternative 2: Add a new test with a new tool and new test driver script
  * How to Add a New Chaincode Test

## How to Run the Tests, and Where to View the Results

Everything starts with [runDailyTestSuite.sh](./runDailyTestSuite.sh), which invokes all test driver scripts, such as *systest_pte.py* and *chaincodes.py*. Together, these driver scripts initiate all tests in the daily test suite. You can manually execute *runDailyTestSuite.sh* in its entirety, or, run one any one of the test driver scripts on the command line. Or, you may simply view the results generated daily by an automated Continuous Improvement (CI) tool which executes *runDailyTestSuite.sh*. Reports are displayed on the [Daily Test Suite Results Page](https://jenkins.hyperledger.org/view/Daily/job/fabric-daily-chaincode-tests-x86_64/test_results_analyzer). When you look at the reports; click the buttons in the **'See children'** column to see the results breakdown by component and by individual tests.

#### Where to Find Existing Tests

Examine the driver scripts to find the individual tests, which are actually stored in several locations under */path/to/fabric/test/*. Some tests are located in test suite subdirectories such as

- *test/regression/daily/chaincodeTests/*

whereas other tests are located in the tools directories themselves, such as

- *test/feature/ft/* - User-friendly **Behave** functional tests feature files
- *test/tools/PTE/* - Performance Traffic Engine **(PTE)** tool and tests
- *test/tools/OTE/* - Orderer Traffic Engine **(OTE)** tool and tests

Each testcase title should provide the test objective and a Jira FAB issue which can be referenced for more information. Test steps and specific details can be found in the summary comments of the test scripts themselves. Additional information can be found in the README files associated with the various test directories.

## How to Add New Tests to the Automated Test Suite

We love contributors! Anyone may add a new test to an existing test driver script, or even create a new tool and new test driver script. The steps for both scenarios are provided further below as **Alternative 1** and **Alternative 2**. First, a few things to note:

- Before linking a test case into the CI automation tests, please merge your (tool and) testcase into gerrit, and create a Jira task, as follows:

  1. First merge your tool and tests to gerrit in appropriate folders under */path/to/fabric/test/*.
  1. Of course, all tests must pass before being submitted. We do not want to see any false positives for test case failures.
  1. To integrate your new tests into the CI automation test suite, create a new Jira task FAB-nnnn for each testcase, and use 'relates-to' to link it to epic FAB-3770.
  1. You will this new Jira task to submit a changeset to gerrit, to invoke your testcase from a driver script similar to */path/to/fabric/test/regression/daily/Example.py*. In the comments of the gerrit merge request submission, include the
      - Jira task FAB-nnnn
      - the testcase title and objective
      - copy and fill in the template from Jira epic FAB-3770
  1. Follow all the steps below in either **Alternative**, and then the test will be executed automatically as part of the next running of the CI daily test suite. The results will show up on the daily test suite display board - which can be viewed by following the link at the top of this page.

#### Why Test Output Format Must Be **xml** and How to Make It So

The Continuous Improvement (CI) team utilizes a Jenkins job to execute the full test suite, *runDailyTestSuite.sh*. The CI job consumes xml output files, creates reports, and displays them. **Note:** When adding new scripts that generate new xml files, if you do not see the results displayed correctly, please contact us on [Rocket.Chat channel #fabric-ci](https://chat.hyperledger.org). For this reason, we execute tests in one of the following ways:

  1. Invoke the individual testcase from within a test driver script in *regression/daily/*. There are many examples here, such as *Example.py* and *systest_pte.py*. These test driver scripts are basically wrappers written in python, which makes it easy to produce the desired junitxml output format required for displaying reports. This method is useful for almost any test language, including bash, tool binaries, and more. More details are provided below explaining how to call testcases from within a test driver script. Here we show how simple it is to execute the test driver and all the testcases within it. **Note:** File *results_sample.xml* will be created, containing the sample testcases output.

  ```
     cd /path/to/fabric/test/regression/daily
     py.test -v --junitxml example_results.xml ./Example.py
  ```

  1. Execute 'go test', and pipe the output through tool github.com/jstemmer/go-junit-report to convert to xml. **Note:** In the example shown, file 'results.xml' will be created with the test output.

  ```
     cd /path/to/fabric/test/tools/OTE
     go get github.com/jstemmer/go-junit-report
     go test -run ORD77 -v | go-junit-report >> results.xml
  ```

  1. *If you know another method that produces xml files that can be displayed correctly, please share it here!*

### Alternative 1:  Add a test using an existing tool and test driver script

To add another test using an existing tool (such as **PTE**), simply add a test inside the existing test driver (such as *systest_pte.py*). It is as simple as copying a block of ten lines and modify these things:

  1. Insert the testcase in the correct test component class and edit the test name
  1. Edit the testcase description
  1. Edit the specified command and arguments to be executed
  1. Edit the asserted test result to be matched

Refer to *Example.py* for a model to clone and get started quickly. The testcases should use the format shown in this example:

  ```
      def test_FAB9876_1K_Payload(self):
        '''
        Launch standard network.
        Use PTE stress mode to send 100 invoke transactions
        concurrently to all peers on all channels on all
        chaincodes, with 1K payloads. Query the ledger for
        each to ensure the last transaction was written,
        calculate tps, and remove network and cleanup
        '''
        result = subprocess.check_output("../../tools/PTE/tests/run1KPayloadTest.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)
  ```

### Alternative 2:  Add a new test with a new tool and new test driver script

Adding a new test with a new tool involves a few more steps.

  1. Create and merge a new tool, for example, */path/to/fabric/test/tools/NewTool/newTool.sh*
  1. Create a new test driver script such as */path/to/fabric/test/regression/daily/newTool.py*.  Model it after others like *Example.py*, found under driver scripts under */path/to/test/regression/daily/* and *test/regression/weekly/*.
  1. Add your new testcases to *newTool.py*. The testcases should use the following format. Refer also to the steps described in Alternative 1, above.

  ```
    class <component_feature>(unittest.TestCase):
      def test_<FAB9999>_<title>(self):
        '''
        <Network Configuration>
        <Test Description and Test Objective>
        '''
        result = subprocess.check_output("<command to invoke newTool.sh arg1 arg2>", shell=True)
        self.assertIn("<string from stdout of newTool that indicates PASS>", result)
  ```

  1. Edit */path/to/test/regression/daily/runDailyTestSuite.sh* to run the new testcases. Add a new line, or append your new test driver scriptname *newTool.py* to an existing line:

  ```
     py.test -v --junitxml results_newTool.xml newTool.py
  ```

### How to Add a New Chaincode Test

To leverage our CI mechanism to automatically test your own ChainCode daily, refer to [this regression/daily/chaincodeTests/README](./chaincodeTests/README.rst) for instructions.


<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
s
