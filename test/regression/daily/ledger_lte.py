import unittest
import subprocess

class perf_goleveldb(unittest.TestCase):

    def test_FAB_3790_VaryNumParallelTxPerChain(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the number of parallel
         transactions per chain and observe the performance.

         Passing criteria: all subtests (8) completed successfully
        '''
        result = subprocess.check_output(
                "./runbenchmarks.sh varyNumParallelTxPerChain",
                shell=True, stderr=subprocess.STDOUT,
                cwd='../../tools/LTE/scripts')
        completion_count = result.count("PASS")
        self.assertEqual(completion_count, 8)

    def test_FAB_3795_VaryNumChain(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the number of chains
         (ledgers).

         Passing criteria: all subtests (8) completed successfully
        '''
        result = subprocess.check_output(
                "./runbenchmarks.sh varyNumChain",
                shell=True, stderr=subprocess.STDOUT,
                cwd='../../tools/LTE/scripts')
        completion_count = result.count("PASS")
        self.assertEqual(completion_count, 8)

    def test_FAB_3798_VaryNumParallelTxWithSingleChain(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the number of parallel
         transactions on a single chain.

         Passing criteria: all subtests (8) completed successfully
        '''
        result = subprocess.check_output(
                "./runbenchmarks.sh varyNumParallelTxWithSingleChain",
                shell=True, stderr=subprocess.STDOUT,
                cwd='../../tools/LTE/scripts')
        completion_count = result.count("PASS")
        self.assertEqual(completion_count, 8)

    def test_FAB_3799_VaryNumChainWithNoParallelism(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the number of chains
         without any parallelism within a single chain.

         Passing criteria: all subtests (8) completed successfully
        '''
        result = subprocess.check_output(
                "./runbenchmarks.sh varyNumChainWithNoParallelism",
                shell=True, stderr=subprocess.STDOUT,
                cwd='../../tools/LTE/scripts')
        completion_count = result.count("PASS")
        self.assertEqual(completion_count, 8)

    def test_FAB_3801_VaryKVSize(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the size of key-value.

         Passing criteria: all subtests (5) completed successfully
        '''
        result = subprocess.check_output(
                "./runbenchmarks.sh varyKVSize",
                shell=True, stderr=subprocess.STDOUT,
                cwd='../../tools/LTE/scripts')
        completion_count = result.count("PASS")
        self.assertEqual(completion_count, 5)

    def test_FAB_3802_VaryBatchSize(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the value of the batch
         size

         Passing criteria: all subtests (4) completed successfully
        '''
        result = subprocess.check_output(
                "./runbenchmarks.sh varyBatchSize",
                shell=True, stderr=subprocess.STDOUT,
                cwd='../../tools/LTE/scripts')
        completion_count = result.count("PASS")
        self.assertEqual(completion_count, 4)

    def test_FAB_3800_VaryNumKeysInEachTX(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the number of keys in
         each transaction.

         Passing criteria: all subtests (5) completed successfully
        '''
        result = subprocess.check_output(
                "./runbenchmarks.sh varyNumKeysInEachTX",
                shell=True, stderr=subprocess.STDOUT,
                cwd='../../tools/LTE/scripts')
        completion_count = result.count("PASS")
        self.assertEqual(completion_count, 5)

    def test_FAB_3803_VaryNumTxs(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the number of
         transactions carried out.

         Passing criteria: all subtests (4) completed successfully
        '''
        result = subprocess.check_output(
                "./runbenchmarks.sh varyNumTxs",
                shell=True, stderr=subprocess.STDOUT,
                cwd='../../tools/LTE/scripts')
        completion_count = result.count("PASS")
        self.assertEqual(completion_count, 4)


class perf_couchdb(unittest.TestCase):
    @unittest.skip("WIP, skipping")
    def test_FAB_3870_VaryNumParallelTxPerChain(self):
        '''
         In this Performance test, we observe the performance (operations
         per second) of the Ledger component, with CouchDB as the state
         database, as we vary the number of parallel transactions per chain.
        '''
        self.assertTrue(True)

    @unittest.skip("WIP, skipping")
    def test_FAB_3871_VaryNumChain(self):
        '''
         In this Performance test, we observe the performance (operations
         per second) of the Ledger component, with CouchDB as the state
         database, as we vary the number of chains (ledgers).
        '''
        self.assertTrue(True)

    @unittest.skip("WIP, skipping")
    def test_FAB_3872_VaryNumParallelTxWithSingleChain(self):
        '''
         In this Performance test, we observe the performance (operations
         per second) of the Ledger component, with CouchDB as the state
         database, vary the number of parallel transactions on a single chain.
        '''
        self.assertTrue(True)

    @unittest.skip("WIP, skipping")
    def test_FAB_3873_VaryNumChainWithNoParallelism(self):
        '''
         In this Performance test, we observe the performance (operations
         per second) of the Ledger component, with CouchDB as the state
         database, as we vary the number of chains without any parallelism.
         within a single chain.
        '''
        self.assertTrue(True)

    @unittest.skip("WIP, skipping")
    def test_FAB_3874_VaryKVSize(self):
        '''
         In this Performance test, we observe the performance (operations
         per second) of the Ledger component, with CouchDB as the state
         database, varying the size of key-value.
        '''
        self.assertTrue(True)

    @unittest.skip("WIP, skipping")
    def test_FAB_3875_VaryBatchSize(self):
        '''
         In this Performance test, we observe the performance (operations
         per second) of the Ledger component, with CouchDB as the state
         database, as we vary the value of the batch size.
        '''
        self.assertTrue(True)

    @unittest.skip("WIP, skipping")
    def test_FAB_3876_VaryNumKeysInEachTX(self):
        '''
         In this Performance test, we observe the performance (operations
         per second) of the Ledger component, with CouchDB as the state
         database, as we vary the number of keys in each transaction.
        '''
        self.assertTrue(True)

    @unittest.skip("WIP, skipping")
    def test_FAB_3877_VaryNumTxs(self):
        '''
         In this Performance test, we observe the performance (operations
         per second) of the Ledger component, with CouchDB as the state
         database, as we vary the number of transactions carried out.
        '''
        self.assertTrue(True)
