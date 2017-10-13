# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

import unittest
import subprocess

tool_directory = '../../tools/LTE/scripts'

class perf_goleveldb(unittest.TestCase):

    def test_FAB_3790_VaryNumParallelTxPerChain(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the number of parallel
         transactions per chain and observe the performance.

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryNumParallelTxPerChain.log", "w")
        returncode = subprocess.call(
                "./runbenchmarks.sh -f parameters_daily_CI.sh "
                "varyNumParallelTxPerChain",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile,
                cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0, msg="VaryNumParallelTxPerChain "
                "performance test failed. \nPlease check the logfile "
                +logfile.name+" for more details.")

    def test_FAB_3795_VaryNumChains(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the number of chains
         (ledgers).

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryNumChains.log", "w")
        returncode = subprocess.call(
                "./runbenchmarks.sh -f parameters_daily_CI.sh varyNumChains",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile,
                cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0, msg="VaryNumChains performance test"
                " failed. \nPlease check the logfile "+logfile.name+" for more "
                "details.")

    def test_FAB_3798_VaryNumParallelTxWithSingleChain(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the number of parallel
         transactions on a single chain.

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryNumParallelTxWithSingleChain.log", "w")
        returncode = subprocess.call(
                "./runbenchmarks.sh -f parameters_daily_CI.sh "
                "varyNumParallelTxWithSingleChain",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile,
                cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0, msg="VaryNumParallelTxWithSingleChain "
                "performance test failed. \nPlease check the logfile "
                +logfile.name+" for more details.")

    def test_FAB_3799_VaryNumChainsWithNoParallelism(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the number of chains
         without any parallelism within a single chain.

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryNumChainsWithNoParallelism.log", "w")
        returncode = subprocess.call(
                "./runbenchmarks.sh -f parameters_daily_CI.sh "
                "varyNumChainsWithNoParallelism",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile,
                cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0, msg="varyNumChainsWithNoParallelism "
                "performance test failed. \nPlease check the logfile "
                +logfile.name+" for more details.")

    def test_FAB_3801_VaryKVSize(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the size of key-value.

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryKVSize.log", "w")
        returncode = subprocess.call(
                "./runbenchmarks.sh -f parameters_daily_CI.sh varyKVSize",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile,
                cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0, msg="varyKVSize performance test"
                " failed. \nPlease check the logfile "+logfile.name+" for more "
                "details.")

    def test_FAB_3802_VaryBatchSize(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the value of the batch
         size

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryBatchSize.log", "w")
        returncode = subprocess.call(
                "./runbenchmarks.sh -f parameters_daily_CI.sh varyBatchSize",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile,
                cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0, msg="varyBatchSize performance test"
                " failed. \nPlease check the logfile "+logfile.name+" for more "
                "details.")

    def test_FAB_3800_VaryNumKeysInEachTx(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the number of keys in
         each transaction.

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryNumKeysInEachTx.log", "w")
        returncode = subprocess.call(
                "./runbenchmarks.sh -f parameters_daily_CI.sh "
                "varyNumKeysInEachTx",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile,
                cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0, msg="varyNumKeysInEachTx performance "
                "test failed. \nPlease check the logfile "+logfile.name
                +" for more details.")

    def test_FAB_3803_VaryNumTxs(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with goleveldb as the state database. We vary the number of
         transactions carried out.

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryNumTxs.log", "w")
        returncode = subprocess.call(
                "./runbenchmarks.sh -f parameters_daily_CI.sh varyNumTxs",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile,
                cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0, msg="varyNumTxs performance test"
                " failed. \nPlease check the logfile "+logfile.name+" for more "
                "details.")


class perf_couchdb(unittest.TestCase):
    def test_FAB_3870_VaryNumParallelTxPerChain(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with couchdb as the state database. We vary the number of parallel
         transactions per chain and observe the performance.

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryNumParallelTxPerChain_couchdb.log", "w")
        returncode = subprocess.call( "./runbenchmarks.sh -f "
                "parameters_couchdb_daily_CI.sh varyNumParallelTxPerChain",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile,
                cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0,
                msg="VaryNumParallelTxPerChain for CouchDB performance test"
                "failed. \nPlease check the logfile " +logfile.name+" for more"
                " details.")


    def test_FAB_3871_VaryNumChain(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with couchdb as the state database. We vary the number of chains
         (ledgers).

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryNumChains_couchdb.log", "w")
        returncode = subprocess.call(
                "./runbenchmarks.sh -f parameters_couchdb_daily_CI.sh "
                "varyNumChains",shell=True, stderr=subprocess.STDOUT,
                stdout=logfile, cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0, msg="VaryNumChains performance test"
                "for CouchDB failed. \nPlease check the logfile "+logfile.name+
                " for more " "details.")

    def test_FAB_3872_VaryNumParallelTxWithSingleChain(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with couchdb as the state database. We vary the number of parallel
         transactions on a single chain.

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryNumParallelTxWithSingleChain_couchdb.log",
                "w")
        returncode = subprocess.call(
                "./runbenchmarks.sh -f parameters_couchdb_daily_CI.sh "
                "varyNumParallelTxWithSingleChain", shell=True,
                stderr=subprocess.STDOUT, stdout=logfile, cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0, msg="VaryNumParallelTxWithSingleChain "
                "performance test for CouchDB failed. \nPlease check the logfile "
                +logfile.name+" for more details.")


    def test_FAB_3873_VaryNumChainWithNoParallelism(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with couchdb as the state database. We vary the number of chains
         without any parallelism within a single chain.

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryNumChainsWithNoParallelism_couchdb.log", "w")
        returncode = subprocess.call(
                "./runbenchmarks.sh -f parameters_couchdb_daily_CI.sh "
                "varyNumChainsWithNoParallelism", shell=True,
                stderr=subprocess.STDOUT, stdout=logfile, cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0, msg="varyNumChainsWithNoParallelism "
                "performance test for CouchDB failed. \nPlease check the logfile "
                +logfile.name+" for more details.")


    def test_FAB_3874_VaryKVSize(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with couchdb as the state database. We vary the size of key-value.

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryKVSize_couchdb.log", "w")
        returncode = subprocess.call(
                "./runbenchmarks.sh -f parameters_couchdb_daily_CI.sh varyKVSize",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile,
                cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0, msg="varyKVSize for CouchDB performance"
                " test failed. \nPlease check the logfile "+logfile.name+" for "
                "more details.")


    def test_FAB_3875_VaryBatchSize(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with couchdb as the state database. We vary the value of the batch
         size

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryBatchSize_couchdb.log", "w")
        returncode = subprocess.call(
                "./runbenchmarks.sh -f parameters_couchdb_daily_CI.sh "
                "varyBatchSize", shell=True, stderr=subprocess.STDOUT,
                stdout=logfile, cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0, msg="varyBatchSize for CouchDB "
                "performance test failed. \nPlease check the logfile "
                +logfile.name+" for more etails.")


    def test_FAB_3876_VaryNumKeysInEachTX(self):
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with couchdb as the state database. We vary the number of keys in
         each transaction.

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryNumKeysInEachTx_couchdb.log", "w")
        returncode = subprocess.call(
                "./runbenchmarks.sh -f parameters_couchdb_daily_CI.sh "
                "varyNumKeysInEachTx", shell=True, stderr=subprocess.STDOUT,
                stdout=logfile, cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0, msg="varyNumKeysInEachTx for CouchDB "
                "performance test failed. \nPlease check the logfile "
                +logfile.name +" for more details.")


    def test_FAB_3877_VaryNumTxs(self):
        self.assertTrue(True)
        '''
         In this Performance test, we observe the performance (time to
         complete a set number of Ledger operations) of the Ledger component,
         with couchdb as the state database. We vary the number of
         transactions carried out.

         Passing criteria: Underlying LTE test completed successfully with
         exit code 0
        '''
        logfile = open("output_VaryNumTxs_couchdb.log", "w")
        returncode = subprocess.call(
                "./runbenchmarks.sh -f parameters_couchdb_daily_CI.sh varyNumTxs",
                shell=True, stderr=subprocess.STDOUT, stdout=logfile,
                cwd=tool_directory)
        logfile.close()
        self.assertEqual(returncode, 0, msg="varyNumTxs for CouchDB "
                "performance test failed. \nPlease check the logfile "
                +logfile.name+" for more " "details.")

