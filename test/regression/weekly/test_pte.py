
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
    def test_FAB3601_Standard_72Hr(self):
        '''
        FAB-2032,FAB-3601
        Network: 2 Ord, 5 KB, 3 ZK, 2 Org, 4 Peers, 10 Chan, 10 CC
        Launch network, use PTE stress mode to send invoke transactions
        concurrently to all peers on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("../daily/TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)


######################################################################
### COUCHDB
######################################################################

class CouchDB_Perf_Stress(unittest.TestCase):
    @unittest.skip("skipping")
    def test_FAB3602_Standard_72Hr(self):
        '''
        FAB-2032,FAB-3602
        Network: 2 Ord, 5 KB, 3 ZK, 2 Org, 4 Peers, 10 Chan, 10 CC
        Launch network, use PTE stress mode to send invoke transactions
        concurrently to all peers on all channels on all chaincodes,
        query the ledger for each to ensure the last transaction was written,
        calculate tps, remove network and cleanup
        '''
        result = subprocess.check_output("../daily/TestPlaceholder.sh", shell=True)
        self.assertIn(TEST_PASS_STRING, result)

