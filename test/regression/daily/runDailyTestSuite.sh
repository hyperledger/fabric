#!/bin/bash

echo "========== Example tests and PTE system tests..."
py.test -v --junitxml results.xml test_example.py test_pte.py

echo "========== Chaincode tests..."
chaincodeTests/runChaincodes.sh

