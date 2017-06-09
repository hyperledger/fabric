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
### COUCHDB
######################################################################

class Perf_Stress_CouchDB(unittest.TestCase):

    @unittest.skip("skipping")
    def test_FAB3820_TimedRun_12Hr(self):
        '''
        Standard Network: 2 Orderers, 3 KafkaBrokers, 3 ZooKeepers,
        2 Certificate Authorities (CAs - 1 per Org), 2 Organizations,
        2 Peers per Org, 4 Peers, 2 Channels, 2 ChainCodes, 8 total threads.
        Launch network, use PTE stress mode to send invoke transactions
        with 1K Payload concurrently to one peer in each organization
        on all channels on all chaincodes, for the specified duration.
        Query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup.
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3821_TimedRun_72Hr(self):
        '''
        Standard Network: 2 Orderers, 3 KafkaBrokers, 3 ZooKeepers,
        2 Certificate Authorities (CAs - 1 per Org), 2 Organizations,
        2 Peers per Org, 4 Peers, 2 Channels, 2 ChainCodes, 8 total threads.
        Launch network, use PTE stress mode to send invoke transactions
        with 1K Payload concurrently to one peer in each organization
        on all channels on all chaincodes, for the specified duration.
        Query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup.
        '''
        result = subprocess.check_output("../daily/TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3822_Scaleup1(self):
        '''
        Network: 2 Ord, 5 KB, 3 ZK, 2 Org, 2 PeersPerOrg, 2 Chan, 1 CC, 4 thrds
        Launch network, use PTE stress mode to send 10000 invoke transactions
        concurrently to a peer in each org on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3823_Scaleup2(self):
        '''
        Network: 2 Ord, 5 KB, 3 ZK, 2 Org, 2 PeersPerOrg, 2 Chan, 2 CC, 8 thrds
        Launch network, use PTE stress mode to send 10000 invoke transactions
        concurrently to a peer in each org on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3824_Scaleup3(self):
        '''
        Network: 2 Ord, 5 KB, 3 ZK, 2 Org, 4 PeersPerOrg, 4 Chan, 2 CC, 16 thrds
        Launch network, use PTE stress mode to send 10000 invoke transactions
        concurrently to a peer in each org on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3825_Scaleup4(self):
        '''
        Network: 4 Ord, 5 KB, 3 ZK, 2 Org, 4 PeersPerOrg, 8 Chan, 2 CC, 32 thrds
        Launch network, use PTE stress mode to send 10000 invoke transactions
        concurrently to a peer in each org on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3826_Scaleup5(self):
        '''
        Network: 4 Ord, 5 KB, 3 ZK, 3 Org, 4 PeersPerOrg (12 Peers), 8 Chan, 2 CC, 48 thrds
        Launch network, use PTE stress mode to send 10000 invoke transactions
        concurrently to a peer in each org on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

