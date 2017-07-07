# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

import unittest
import subprocess

class make_targets(unittest.TestCase):


    def test_makeNative(self):
        '''
         In this make targets test, we execute makeNative target to make sure native target
         is working without any issues.

         Passing criteria: make native test completed successfully with
         exit code 0
        '''
        logfile = open("output_make_native_release_tests.log", "w")
        returncode = subprocess.call(
                "./run_make_targets.sh makeNative",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile)
        logfile.close()
        self.assertEqual(returncode, 0, msg="Run make native targets "
                "make native target tests failed. \nPlease check the logfile ")


    def test_makeBinary(self):
        '''
         In this make targets test, we execute make binary target to make sure binary target
         is working without any issues.

         Passing criteria: make binary test completed successfully with
         exit code 0
        '''
        logfile = open("output_make_binary_release_tests.log", "w")
        returncode = subprocess.call(
                "./run_make_targets.sh makeBinary",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile)
        logfile.close()
        self.assertEqual(returncode, 0, msg="Run make binary target "
                "make binary target tests failed. \nPlease check the logfile ")


    def test_makeDistAll(self):
        '''
         In this make targets test, we execute make dist-all target to make sure dist-all target
         is working without any issues.

         Passing criteria: make dist-all test completed successfully with
         exit code 0
        '''
        logfile = open("output_make_dist-all_release_tests.log", "w")
        returncode = subprocess.call(
                "./run_make_targets.sh makeDistAll",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile)
        logfile.close()
        self.assertEqual(returncode, 0, msg="Run make dist-all target "
                "make dist-all target tests failed. \nPlease check the logfile ")


    def test_makeDocker(self):
        '''
         In this make targets test, we execute make docker target to make sure docker target
         is working without any issues.

         Passing criteria: make docker test completed successfully with
         exit code 0
        '''
        logfile = open("output_make_docker_release_tests.log", "w")
        returncode = subprocess.call(
                "./run_make_targets.sh makeDocker",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile)
        logfile.close()
        self.assertEqual(returncode, 0, msg="Run make Docker target "
                "make Docker target tests failed. \nPlease check the logfile ")

    def test_makeVersion(self):
        '''
         In this make targets test, we execute version check to make sure binaries version
         is correct.

         Passing criteria: make version test completed successfully with
         exit code 0
        '''
        logfile = open("output_make_version_release_tests.log", "w")
        returncode = subprocess.call(
                "./run_make_targets.sh makeVersion",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile)
        logfile.close()
        self.assertEqual(returncode, 0, msg="Run make version target "
                "make version target tests failed. \nPlease check the logfile ")
