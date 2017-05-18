#!/bin/bash

######################################################################
### Run one group of the tests in weekly test suite.

echo "========== Performance Stress PTE 72Hr test"
py.test -v --junitxml results_systest_pte_72Hr.xml systest_pte.py -k TimedRun_72Hr

