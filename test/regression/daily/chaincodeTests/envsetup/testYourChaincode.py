# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

#!/usr/bin/python2.7
import subprocess
import unittest
from subprocess import check_output

class ChaincodeAPI(unittest.TestCase):

    @classmethod
    def setUpClass(cls):
        cls.CHANNEL_NAME = "channel"
        cls.CHANNELS = '2'
        cls.CHAINCODES = '1'
        cls.ENDORSERS = '4'
        check_output(["./network_setup.sh up {0} {1} {2} {3}".format(cls.CHANNEL_NAME, cls.CHANNELS, cls.CHAINCODES, cls.ENDORSERS)],
                     shell=True)

    @classmethod
    def tearDownClass(cls):
        check_output(["./network_setup.sh down"], shell=True)

#################################################################################

    def runIt(self, command, scriptName):
        cmd = "/opt/gopath/src/github.com/hyperledger/fabric/test/regression/daily/chaincodeTests/%s %s %s %s %s %s"% (scriptName, self.CHANNEL_NAME, self.CHANNELS, self.CHAINCODES, self.ENDORSERS, command)
        output = check_output(["docker exec cli {0}".format(cmd)], shell=True)
        return output

#####Begin tests for different chaincode samples that exist in fabricExamples folder ###################
    def test_AFAB3843_Create_Join_Channel(self):
        ''' Create_Join_Channel must be run as the first test. Each chaincode test is dependent on this test
            So make sure when adding new tests this is picked up as the first one by py.test
        '''
        output = self.runIt("create", "fabricFeatureChaincodes/create_join_channel.sh")
        self.assertIn('Channel "channel0" is created successfully', output)
        self.assertIn('Channel "channel1" is created successfully', output)

        output = self.runIt("join", "fabricFeatureChaincodes/create_join_channel.sh")
        self.assertIn('PEER0 joined on the channel "channel0" successfully', output)
        self.assertIn('PEER0 joined on the channel "channel1" successfully', output)
        self.assertIn('PEER1 joined on the channel "channel0" successfully', output)
        self.assertIn('PEER1 joined on the channel "channel1" successfully', output)
        self.assertIn('PEER2 joined on the channel "channel0" successfully', output)
        self.assertIn('PEER2 joined on the channel "channel1" successfully', output)
        self.assertIn('PEER3 joined on the channel "channel0" successfully', output)
        self.assertIn('PEER3 joined on the channel "channel1" successfully', output)

    #@unittest.skip("skipping")
    def test_FAB3794_example02_invoke_query_inaloop_onallpeers(self):
        ''' This tests example02 chaincode where transfer values from assets
            "a" and "b" via Invoke and then verify values by Querying on Ledger.
        '''
        output = self.runIt("install", "fabricFeatureChaincodes/e2e_test_example02.sh")
        self.assertIn("Chaincode 'myccex020' is installed on PEER0 successfully", output)
        self.assertIn("Chaincode 'myccex020' is installed on PEER1 successfully", output)
        self.assertIn("Chaincode 'myccex020' is installed on PEER2 successfully", output)
        self.assertIn("Chaincode 'myccex020' is installed on PEER3 successfully", output)

        output = self.runIt("instantiate", "fabricFeatureChaincodes/e2e_test_example02.sh")
        self.assertIn("Chaincode 'myccex020' Instantiation on PEER2 on 'channel0' is successful", output)
        self.assertIn("Chaincode 'myccex020' Instantiation on PEER2 on 'channel1' is successful", output)

        output = self.runIt("invokeQuery", "fabricFeatureChaincodes/e2e_test_example02.sh")
        self.assertIn('Query on PEER0 on channel0/myccex020 is successful', output)
        countPeer0Ch0 = output.count('Query on PEER0 on channel0/myccex020 is successful')
        self.assertEquals(countPeer0Ch0, 2)
        self.assertIn('Query on PEER1 on channel0/myccex020 is successful', output)
        countPeer1Ch0 = output.count('Query on PEER1 on channel0/myccex020 is successful')
        self.assertEquals(countPeer1Ch0, 2)
        self.assertIn('Query on PEER2 on channel0/myccex020 is successful', output)
        countPeer2Ch0 = output.count('Query on PEER2 on channel0/myccex020 is successful')
        self.assertEquals(countPeer2Ch0, 2)
        self.assertIn('Query on PEER3 on channel0/myccex020 is successful', output)
        countPeer3Ch0 = output.count('Query on PEER3 on channel0/myccex020 is successful')
        self.assertEquals(countPeer3Ch0, 2)
        self.assertIn('Query on PEER0 on channel1/myccex020 is successful', output)
        countPeer0Ch1 = output.count('Query on PEER0 on channel1/myccex020 is successful')
        self.assertEquals(countPeer0Ch1, 2)
        self.assertIn('Query on PEER1 on channel1/myccex020 is successful', output)
        countPeer1Ch1 = output.count('Query on PEER1 on channel1/myccex020 is successful')
        self.assertEquals(countPeer1Ch1, 2)
        self.assertIn('Query on PEER2 on channel1/myccex020 is successful', output)
        countPeer2Ch1 = output.count('Query on PEER2 on channel1/myccex020 is successful')
        self.assertEquals(countPeer2Ch1, 2)
        self.assertIn('Query on PEER3 on channel1/myccex020 is successful', output)
        countPeer3Ch1 = output.count('Query on PEER3 on channel1/myccex020 is successful')
        self.assertEquals(countPeer3Ch1, 2)
        self.assertIn('Invoke transaction on PEER0 on channel0/myccex020 is successful', output)
        self.assertIn('Invoke transaction on PEER1 on channel0/myccex020 is successful', output)
        self.assertIn('Invoke transaction on PEER2 on channel0/myccex020 is successful', output)
        self.assertIn('Invoke transaction on PEER3 on channel0/myccex020 is successful', output)
        self.assertIn('Invoke transaction on PEER0 on channel1/myccex020 is successful', output)
        self.assertIn('Invoke transaction on PEER1 on channel1/myccex020 is successful', output)
        self.assertIn('Invoke transaction on PEER2 on channel1/myccex020 is successful', output)
        self.assertIn('Invoke transaction on PEER3 on channel1/myccex020 is successful', output)
        self.assertIn('End-2-End for chaincode example02 completed successfully', output)

    @unittest.skip("skipping")
    def test_FAB3791_example03_negative_test_disallow_write_attempt_in_query(self):
        ''' Test writing to ledger via query fails as expected
        '''
        output = self.runIt("install", "fabricFeatureChaincodes/e2e_test_example03.sh")
        self.assertIn("successfully", output)
        output = self.runIt("instantiate", "fabricFeatureChaincodes/e2e_test_example03.sh")
        self.assertIn("successfully", output)
        output = self.runIt("invokeQuery", "fabricFeatureChaincodes/e2e_test_example03.sh")
        self.assertIn('Query result on PEER0 is INVALID', output)

    @unittest.skip("skipping")
    def test_FAB3796_example04_chaincode_to_chaincode_call_on_occurrence_of_an_event(self):
        ''' Test chaincode to chaincode calling when an event is generated.
        '''
        output = self.runIt("install", "fabricFeatureChaincodes/e2e_test_example04.sh")
        self.assertIn("successfully", output)
        output = self.runIt("instantiate", "fabricFeatureChaincodes/e2e_test_example04.sh")
        self.assertIn("successfully", output)
        output = self.runIt("invokeQuery", "fabricFeatureChaincodes/e2e_test_example04.sh")
        self.assertIn("successfully", output)

    @unittest.skip("skipping")
    def test_FAB3797_example05_chaincode_to_chaincode_call_on_same_and_different_channel(self):
        ''' Test chaincode to chaincode calling, when second chaincode exists
            on a different channel.
        '''
        output = self.runIt("install", "fabricFeatureChaincodes/e2e_test_example05.sh")
        self.assertIn("Chaincode 'myccex05' is installed on PEER0 successfully", output)
        self.assertIn("Chaincode 'myccex05' is installed on PEER1 successfully", output)
        self.assertIn("Chaincode 'myccex05' is installed on PEER2 successfully", output)
        self.assertIn("Chaincode 'myccex05' is installed on PEER3 successfully", output)
        self.assertIn("Chaincode 'ex02_b' is installed on PEER0 successfully", output)
        self.assertIn("Chaincode 'ex02_b' is installed on PEER1 successfully", output)
        self.assertIn("Chaincode 'ex02_b' is installed on PEER2 successfully", output)
        self.assertIn("Chaincode 'ex02_b' is installed on PEER3 successfully", output)
        output = self.runIt("instantiate", "fabricFeatureChaincodes/e2e_test_example05.sh")
        self.assertIn("successfully", output)
        output = self.runIt("invokeQuery", "fabricFeatureChaincodes/e2e_test_example05.sh")
        self.assertIn("successfully", output)

    @unittest.skip("skipping")
    def test_FAB3792_marbles02_init_query_transfer_marbles(self):
        ''' This test few basic operations from marbles02 chaincode.
        '''
        output = self.runIt("install", "fabricFeatureChaincodes/e2e_test_marbles02.sh")
        self.assertIn("successfully", output)
        output = self.runIt("instantiate", "fabricFeatureChaincodes/e2e_test_marbles02.sh")
        self.assertIn("successfully", output)
        output = self.runIt("initMarble1", "fabricFeatureChaincodes/e2e_test_marbles02.sh")
        self.assertIn("successful", output)
        output = self.runIt("queryMarble1", "fabricFeatureChaincodes/e2e_test_marbles02.sh")
        self.assertIn("successful", output)
        output = self.runIt("initMarble2", "fabricFeatureChaincodes/e2e_test_marbles02.sh")
        self.assertIn("successful", output)
        output = self.runIt("queryMarble2", "fabricFeatureChaincodes/e2e_test_marbles02.sh")
        self.assertIn("Query marble2 on PEER2 on channel0 on chaincode mymarbles020 is successful", output)
        output = self.runIt("txfrMarble", "fabricFeatureChaincodes/e2e_test_marbles02.sh")
        self.assertIn("Txfr marble1 on PEER2 on channel0 on chaincode mymarbles020 to JERRY is successful", output)
        output = self.runIt("queryAfterTxfrMarble", "fabricFeatureChaincodes/e2e_test_marbles02.sh")
        self.assertIn("Query marble1 on PEER2 on channel0 on chaincode mymarbles020 is successful", output)

    @unittest.skip("skipping")
    def test_FAB3793_chaincodeAPIDriver_exercise_chaincode_api_calls_as_invoke_functions(self):
        ''' Calling functions in shim/chaincode.go.
        '''
        output = self.runIt("install", "fabricFeatureChaincodes/e2e_test_ccapidriver.sh")
        self.assertIn("successfully", output)
        output = self.runIt("instantiate", "fabricFeatureChaincodes/e2e_test_ccapidriver.sh")
        self.assertIn("successfully", output)
        output = self.runIt("invoke_getTxTimeStamp", "fabricFeatureChaincodes/e2e_test_ccapidriver.sh")
        self.assertIn("successful", output)
        output = self.runIt("invoke_getBinding", "fabricFeatureChaincodes/e2e_test_ccapidriver.sh")
        self.assertIn("successful", output)
        output = self.runIt("invoke_getCreator", "fabricFeatureChaincodes/e2e_test_ccapidriver.sh")
        self.assertIn("successful", output)
        output = self.runIt("invoke_getSignedProposal", "fabricFeatureChaincodes/e2e_test_ccapidriver.sh")
        self.assertIn("successful", output)
        output = self.runIt("invoke_getTransient", "fabricFeatureChaincodes/e2e_test_ccapidriver.sh")
        self.assertIn("successful", output)

    @unittest.skip("skipping")
    def test_FAB3793_chaincodeAPIDriver_2_exercise_chaincode_api_calls_as_direct_calls(self):
        ''' Calling functions in shim/chaincode.go.
        '''
        output = self.runIt("install", "fabricFeatureChaincodes/e2e_test_ccapidriver_two.sh")
        self.assertIn("successfully", output)
        output = self.runIt("instantiate", "fabricFeatureChaincodes/e2e_test_ccapidriver_two.sh")
        self.assertIn("successfully", output)
        output = self.runIt("invoke_getTxTimeStamp", "fabricFeatureChaincodes/e2e_test_ccapidriver_two.sh")
        self.assertIn("successful", output)
        output = self.runIt("invoke_getBinding", "fabricFeatureChaincodes/e2e_test_ccapidriver_two.sh")
        self.assertIn("successful", output)
        output = self.runIt("invoke_getCreator", "fabricFeatureChaincodes/e2e_test_ccapidriver_two.sh")
        self.assertIn("successful", output)
        output = self.runIt("invoke_getSignedProposal", "fabricFeatureChaincodes/e2e_test_ccapidriver_two.sh")
        self.assertIn("successful", output)
        output = self.runIt("invoke_getTransient", "fabricFeatureChaincodes/e2e_test_ccapidriver_two.sh")
        self.assertIn("successful", output)

    @unittest.skip("skipping")
    def test_FABXXXX_Laurie_Chaincode(self):
        ''' Calling functions in shim/chaincode.go.
        '''
        pass

    @unittest.skip("skipping")
    def test_FABXXXX_Alpha_Chaincode(self):
        ''' Calling functions in shim/chaincode.go.
        '''
        pass

    @unittest.skip("skipping")
    def test_FABXXXX_XYZ_Chaincode(self):
        ''' Calling functions in shim/chaincode.go.
        '''
        pass

    @unittest.skip("skipping")
    def test_FABXXXX_ABC_Chaincode(self):
        ''' Calling functions in shim/chaincode.go.
        '''
        pass
