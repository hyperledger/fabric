#!/bin/bash
#
# Copyright IBM Corp. 2016 All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

# DESCRIPTION:
# These are the tests in the Daily Test Suite.
# To run these tests, user may clone the fabric, make code changes if
# desired, build images, and run any of these scripts - each of which
# may take between 12 hours and 3 days to complete.

# SETUP

WORKDIR=`pwd`





# Note to test developers: Many tests in the Daily Test Suite are
# specific tests that already contain the TEST OBJECTIVE and STEPS.
# However, in some cases, such as running a flexible test engine to
# perform different tests by using configuration parameters or
# environment variables, it must be provided here.
# For EACH of those test cases, before invoking the test, please
# provide a comment block like this:

#-----------------------------------------------------------------------
# TEST OBJECTIVE : basic network setup, deploy, invoke, query all peers
#                  using gRPC interface, and exercise chaincode APIs
# SETUP STEPS:
#   1. Set environment variable: export TEST_NETWORK=LOCAL
# TEST DETAILS (Optional):
#   1. Setup local docker network with security with 4 peer nodes
#   2. Deploy chaincode example02
#   3. Send Invoke Requests on all peers using go routines
#   4. Verify query results and chainheights match on all peers
#-----------------------------------------------------------------------




#UNDER CONSTRUCTION...

########################################################################
# Language      API                Functional Area
# GO TDK        gRPC               Consensus Regression Tests
########################################################################

cd $WORKDIR/go/tdk/CAT
#go run CRT_501_StopAndRestartRandom_10Hrs.go
#go run CRT_502_StopAndRestart1or2_10Hrs.go
## run_largeNetworksBasicApiAndConsensus.sh:
#go run CRT_504_Npeers_Sf_S_R_Rf.go


########################################################################
# Language      API                Functional Area
# GO TDK        gRPC               Ledger Stress Tests
########################################################################

cd $WORKDIR/go/tdk/ledgerstresstest
## release-criteria/fvt/consensus/tdk/automation/run_LST_3M.sh
#go run LST_4client1peer1M.go
#go run LST_4client4peer3M.go
## release-criteria/svt/longruns/run_long_run_72hour_LOCAL.sh
#go run LongRun72hrAuto.go


########################################################################
# Language      API                Functional Area
# GO TDK        gRPC               Concurrency/Ledger
########################################################################

cd $WORKDIR/go/tdk/ledgerstresstest


########################################################################
# Language      API                Functional Area
# Node.js       gRPC               Concurrency with Auction
########################################################################

cd $WORKDIR/node/


########################################################################
# Language      API                Functional Area
# Node.js       gRPC               Complex Transactions with Auction
########################################################################

cd $WORKDIR/node/


########################################################################
# Language      API                Functional Area
# Node.js       gRPC               Performance engine tests
########################################################################

cd $WORKDIR/node/performance/


