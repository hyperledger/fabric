#!/bin/bash

######################################################################
### Run one group of the tests in weekly test suite.

echo "========== Performance Stress PTE 12Hr test"
py.test -v --junitxml results_systest_pte_12Hr.xml systest_pte.py -k TimedRun_12Hr

