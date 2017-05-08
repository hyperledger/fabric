#!/bin/bash

#Launches test network and executes multiple tests that 
#exist in chaincodeTests python script inside a  CLI container
cd envsetup 
./network_setup.sh restart channel 2 1 4 fabricFeatureChaincodeTestRuns.py
