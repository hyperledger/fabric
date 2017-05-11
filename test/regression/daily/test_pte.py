
######################################################################
# To execute:
# Install: sudo apt-get install python python-pytest
# Run on command line: py.test -v --junitxml results.xml ./test_pte.py

import unittest
import subprocess

TEST_PASS_STRING="RESULT=PASS"


######################################################################
### LEVELDB
######################################################################

class LevelDB_Perf_Stress(unittest.TestCase):
    @unittest.skip("skipping")
    def test_FAB3584_SkeletonQueries(self):
        '''
        FAB-2032,FAB-3584
        Network: 1 Ord, 1 KB, 1 ZK, 2 Org, 2 Peers, 1 Chan, 1 CC
        Launch skeleton network, use PTE in STRESS mode to continuously
        send 10000 query transactions concurrently to 1 peer in both orgs,
        calculate tps, and remove network and cleanup
        '''
        # Replace TestPlaceholder.sh with actual test name, something like:
        #     ../../tools/PTE/tests/runSkeletonQueriesLevel.sh
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3586_SkeletonInvokes(self):
        '''
        FAB-2032,FAB-3586
        Network: 1 Ord, 1 KB, 1 ZK, 2 Org, 2 Peers, 1 Chan, 1 CC
        Launch skeleton network, use PTE in STRESS mode to continuously
        send 10000 query transactions concurrently to 1 peer in both orgs,
        query the ledger to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3593_Standard_basic_TLS(self):
        '''
        FAB-2032,FAB-3593
        Network: 2 Ord, 5 KB, 3 ZK, 2 Org, 4 Peers, 10 Chan, 10 CC
        Launch network, use PTE stress mode to send 100 invoke transactions
        concurrently to all peers on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3595_Standard_basic_1M(self):
        '''
        FAB-2032,FAB-3595
        Network: 2 Ord, 5 KB, 3 ZK, 2 Org, 4 Peers, 10 Chan, 10 CC
        Launch network, use PTE stress mode to send 100 invoke transactions
        concurrently to all peers on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3597_Standard_basic_Gossip(self):
        '''
        FAB-2032,FAB-3597
        Network: 2 Ord, 5 KB, 3 ZK, 2 Org, 4 Peers, 10 Chan, 10 CC
        Launch network, use PTE stress mode to send 100 invoke transactions
        concurrently to all peers on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3599_Standard_12Hr(self):
        '''
        FAB-2032,FAB-3599
        Network: 2 Ord, 5 KB, 3 ZK, 2 Org, 4 Peers, 10 Chan, 10 CC
        Launch network, use PTE stress mode to send invoke transactions
        concurrently to all peers on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)


######################################################################
### COUCHDB
######################################################################

class CouchDB_Perf_Stress(unittest.TestCase):
    @unittest.skip("skipping")
    def test_FAB3585_SkeletonQueries(self):
        '''
        FAB-2032,FAB-3585
        Network: 1 Ord, 1 KB, 1 ZK, 2 Org, 2 Peers, 1 Chan, 1 CC
        Launch skeleton network, use PTE in STRESS mode to continuously
        send 10000 query transactions concurrently to 1 peer in both orgs,
        calculate tps, and remove network and cleanup
        '''
        # Replace TestPlaceholder.sh with actual test name, something like:
        #     ../../tools/PTE/tests/runSkeletonQueriesCouch.sh
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3587_SkeletonInvokes(self):
        '''
        FAB-2032,FAB-3587
        Network: 1 Ord, 1 KB, 1 ZK, 2 Org, 2 Peers, 1 Chan, 1 CC
        Launch skeleton network, use PTE in STRESS mode to continuously
        send 10000 query transactions concurrently to 1 peer in both orgs,
        query the ledger to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3588_Scaleup1(self):
        '''
        FAB-2032,FAB-3588
        Network: 2 Ord, 5 KB, 3 ZK, 2 Org, 4 Peers, 20 Chan, 2 CC
        Launch network, use PTE stress mode to send 100 invoke transactions
        concurrently to all peers on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3589_Scaleup2(self):
        '''
        FAB-2032,FAB-3589
        Network: 2 Ord, 5 KB, 3 ZK, 4 Org, 8 Peers, 40 Chan, 4 CC
        Launch network, use PTE stress mode to send 100 invoke transactions
        concurrently to all peers on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3590_Scaleup3(self):
        '''
        FAB-2032,FAB-3590
        Network: 2 Ord, 5 KB, 3 ZK, 8 Org, 16 Peers, 80 Chan, 8 CC
        Launch network, use PTE stress mode to send 100 invoke transactions
        concurrently to all peers on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3591_Scaleup4(self):
        '''
        FAB-2032,FAB-3591
        Network: 4 Ord, 5 KB, 3 ZK, 16 Org, 32 Peers, 160 Chan, 16 CC
        Launch network, use PTE stress mode to send 100 invoke transactions
        concurrently to all peers on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3592_Scaleup5(self):
        '''
        FAB-2032,FAB-3592
        Network: 4 Ord, 5 KB, 3 ZK, 32 Org, 64 Peers, 320 Chan, 32 CC
        Launch network, use PTE stress mode to send 100 invoke transactions
        concurrently to all peers on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3594_Standard_basic_TLS(self):
        '''
        FAB-2032,FAB-3594
        Network: 2 Ord, 5 KB, 3 ZK, 2 Org, 4 Peers, 10 Chan, 10 CC
        Launch network, use PTE stress mode to send 100 invoke transactions
        concurrently to all peers on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3596_Standard_basic_1M(self):
        '''
        FAB-2032,FAB-3596
        Network: 2 Ord, 5 KB, 3 ZK, 2 Org, 4 Peers, 10 Chan, 10 CC
        Launch network, use PTE stress mode to send 100 invoke transactions
        concurrently to all peers on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3598_Standard_basic_Gossip(self):
        '''
        FAB-2032,FAB-3598
        Network: 2 Ord, 5 KB, 3 ZK, 2 Org, 4 Peers, 10 Chan, 10 CC
        Launch network, use PTE stress mode to send 100 invoke transactions
        concurrently to all peers on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

    @unittest.skip("skipping")
    def test_FAB3600_Standard_12Hr(self):
        '''
        FAB-2032,FAB-3600
        Network: 2 Ord, 5 KB, 3 ZK, 2 Org, 4 Peers, 10 Chan, 10 CC
        Launch network, use PTE stress mode to send invoke transactions
        concurrently to all peers on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("./TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

