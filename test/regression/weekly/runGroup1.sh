#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


######################################################################
### Run one group of the tests in weekly test suite.

echo "========== Performance Stress PTE Scaleup tests"
py.test -v --junitxml results_systest_pte_Scaleup.xml systest_pte.py -k Scaleup

