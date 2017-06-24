# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

# To run this:
# Install: sudo apt-get install python python-pytest
# Install: sudo pip install xmlrunner
# At command line: py.test -v --junitxml results_sample.xml Example.py

import unittest
import xmlrunner
import subprocess

TEST_PASS_STRING="RESULT=PASS"

class SampleTest(unittest.TestCase):
    @unittest.skip("skipping")
    def test_skipped(self):
        '''
        This test will be skipped.
        '''
        self.fail("I should not see this")

    # This runs on ubuntu x86 laptop, but it fails when run by CI, because
    # "bc" is not installed on the servers used for CI jobs.
    @unittest.skip("skipping")
    def test_SampleAdditionTestSkippedButWillPassIfInstallBC(self):
        '''
        This test will pass.
        '''
        result = subprocess.check_output("echo '7+3' | bc", shell=True)
        self.assertEqual(int(result.strip()), 10)

    def test_SampleStringTestWillPass(self):
        '''
        This test will pass.
        '''
        result = subprocess.check_output("echo '7+3'", shell=True)
        self.assertEqual(result.strip(), "7+3")

    def test_SampleScriptPassTest(self):
        '''
        This test will pass because the executed script prints the RESULT=PASS string to stdout
        '''
        result = subprocess.check_output("./SampleScriptPassTest.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    def test_SampleScriptFailTest(self):
        '''
        This test will pass because the executed script does NOT print the RESULT=PASS string to stdout
        '''
        result = subprocess.check_output("./SampleScriptFailTest.sh", shell=True)
        self.assertNotIn(TEST_PASS_STRING, result)

if __name__ == '__main__':
    unittest.main(testRunner=xmlrunner.XMLTestRunner(output='runner-results'))
