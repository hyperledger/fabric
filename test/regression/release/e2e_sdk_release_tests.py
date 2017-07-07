# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

import unittest
import subprocess

class sdk_Release_Tests(unittest.TestCase):

    def test_e2e_node_sdk_release_tests(self):
        '''
         In this e2e node sdk release test, we execute all node sdk unit and e2e tests
         are working without any issues.

         Passing criteria: e2e node sdk release test completed successfully with
         exit code 0
        '''
        logfile = open("output_e2e_node_sdk_release_tests.log", "w")
        returncode = subprocess.call(
                "./run_e2e_node_sdk.sh",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile)
        logfile.close()
        self.assertEqual(returncode, 0, msg="Run e2e_node_sdk_release_tests"
                "e2e_node_sdk_release_tests are failed. \nPlease check the logfile "
                +logfile.name+" for more details.")


    def test_e2e_java_sdk_release_tests(self):
        '''
         In this e2e java sdk release test, we execute all java sdk unit and e2e tests
         are working without any issues.

         Passing criteria: e2e java sdk release test completed successfully with
         exit code 0
        '''
        logfile = open("output_e2e_java_sdk_release_tests.log", "w")
        returncode = subprocess.call(
                "./run_e2e_java_sdk.sh",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile)
        logfile.close()
        self.assertEqual(returncode, 0, msg="Run e2e_java_sdk_release_tests"
                "e2e_java_sdk_release_tests are failed. \nPlease check the logfile "
                +logfile.name+" for more details.")
