# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

######################################################################
# To execute:
# Install: sudo apt-get install python python-pytest
# Run on command line: py.test -v --junitxml results.xml ./systest_pte.py

import unittest
import subprocess

TEST_PASS_STRING="RESULT=PASS"


######################################################################
### LEVELDB
######################################################################

class Perf_Stress_LevelDB(unittest.TestCase):
    @unittest.skip("skipping")
    def test_FAB3808_TPS_Queries_1_Thread_TinyNtwk(self):
        '''
        Tiny Network: 1 Ord, 1 KB, 1 ZK, 2 Org, 2 Peers, 1 Chan, 1 CC, 2 thrds
        Launch tiny network, use PTE in STRESS mode to continuously
        send 10000 query transactions concurrently to 1 peer in both orgs,
        calculate tps, and remove network and cleanup
        '''
        # Replace TestPlaceholder.sh with actual test name, something like:
        #     ../../tools/PTE/tests/runSkeletonQueriesLevel.sh
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3811_TPS_Invokes_1_Thread_TinyNtwk(self):
        '''
        Tiny Network: 1 Ord, 1 KB, 1 ZK, 2 Org, 2 Peers, 1 Chan, 1 CC, 2 thrds
        Launch tiny network, use PTE in STRESS mode to continuously
        send 10000 query transactions concurrently to 1 peer in both orgs,
        query the ledger to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3833_TPS_Queries_8_Thread_TinyNtwk(self):
        '''
        Tiny Network: 1 Ord, 1 KB, 1 ZK, 2 Org, 2 Peers, 1 Chan, 1 CC, 2 thrds
        Launch tiny network, use PTE in STRESS mode to continuously
        send 10000 query transactions concurrently to 1 peer in both orgs,
        calculate tps, and remove network and cleanup
        '''
        # Replace TestPlaceholder.sh with actual test name, something like:
        #     ../../tools/PTE/tests/runSkeletonQueriesLevel.sh
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3835_TPS_Invokes_8_Thread_TinyNtwk(self):
        '''
        Tiny Network: 1 Ord, 1 KB, 1 ZK, 2 Org, 2 Peers, 1 Chan, 1 CC, 2 thrds
        Launch tiny network, use PTE in STRESS mode to continuously
        send 10000 query transactions concurrently to 1 peer in both orgs,
        query the ledger to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)


######################################################################
### COUCHDB
######################################################################

class Perf_Stress_CouchDB(unittest.TestCase):
    @unittest.skip("skipping")
    def test_FAB3807_TPS_Queries_1_Thread_TinyNtwk(self):
        '''
        Tiny Network: 1 Ord, 1 KB, 1 ZK, 2 Org, 2 Peers, 1 Chan, 1 CC, 2 thrds
        Launch tiny network, use PTE in STRESS mode to continuously
        send 10000 query transactions concurrently to 1 peer in both orgs,
        calculate tps, and remove network and cleanup
        '''
        # Replace TestPlaceholder.sh with actual test name, something like:
        #     ../../tools/PTE/tests/runSkeletonQueriesCouch.sh
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3810_TPS_Invokes_1_Thread_TinyNtwk(self):
        '''
        Tiny Network: 1 Ord, 1 KB, 1 ZK, 2 Org, 2 Peers, 1 Chan, 1 CC, 2 thrds
        Launch tiny network, use PTE in STRESS mode to continuously
        send 10000 query transactions concurrently to 1 peer in both orgs,
        query the ledger to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3832_TPS_Queries_8_Thread_TinyNtwk(self):
        '''
        Tiny Network: 1 Ord, 1 KB, 1 ZK, 2 Org, 2 Peers, 1 Chan, 1 CC, 2 thrds
        Launch tiny network, use PTE in STRESS mode to continuously
        send 10000 query transactions concurrently to 1 peer in both orgs,
        calculate tps, and remove network and cleanup
        '''
        # Replace TestPlaceholder.sh with actual test name, something like:
        #     ../../tools/PTE/tests/runSkeletonQueriesCouch.sh
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3834_TPS_Invokes_8_Thread_TinyNtwk(self):
        '''
        Tiny Network: 1 Ord, 1 KB, 1 ZK, 2 Org, 2 Peers, 1 Chan, 1 CC, 2 thrds
        Launch tiny network, use PTE in STRESS mode to continuously
        send 10000 query transactions concurrently to 1 peer in both orgs,
        query the ledger to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3813_Baseline_StandardNtwk_8_Thread(self):
        '''
        "Standard Network": 2 Orderers, 3 KafkaBrokers, 3 ZooKeepers,
        2 Certificate Authorities (CAs - 1 per Org), 2 Organizations,
        2 Peers per Org, 4 Peers, 2 Channels, 2 ChainCodes, 8 total threads.
        Launch network, use PTE stress mode to send 10000 invoke transactions
        concurrently to a peer in each org on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3814_Payload_1Meg(self):
        '''
        Standard Network: 2 Orderers, 3 KafkaBrokers, 3 ZooKeepers,
        2 Certificate Authorities (CAs - 1 per Org), 2 Organizations,
        2 Peers per Org, 4 Peers, 2 Channels, 2 ChainCodes, 8 total threads.
        Launch network, use PTE stress mode to send 10000 invoke transactions
        concurrently to a peer in each org on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3816_GossipStress_10_PeersPerOrg(self):
        '''
        Standard Network plus extra peers: 2 Orderers, 3 KafkaBrokers, 3 ZKs,
        2 Certificate Authorities (CAs - 1 per Org), 2 Organizations,
        10 Peers per Org, 4 Peers, 2 Channels, 2 ChainCodes, 8 total threads.
        Launch network, use PTE stress mode to send 10000 invoke transactions
        concurrently to a peer in each org on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

