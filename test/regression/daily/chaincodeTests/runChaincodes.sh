#!/bin/bash

#Launches test network and executes multiple tests that
#exist in chaincodeTests python script inside a  CLI container
cd envsetup
py.test -v --junitxml YourChaincodeResults.xml testYourChaincode.py
