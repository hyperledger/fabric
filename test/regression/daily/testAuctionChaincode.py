#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

#!/usr/bin/python
# -*- coding: utf-8 -*-

######################################################################
# To execute:
# Install: sudo apt-get install python python-pytest
# Run on command line: py.test -v --junitxml results_auction_daily.xml testAuctionChaincode.py

import subprocess
import unittest
from subprocess import check_output
import shutil


class ChaincodeAPI(unittest.TestCase):

    @classmethod
    def setUpClass(cls):
        cls.CHANNEL_NAME = 'channel'
        cls.CHANNELS = '2'
        cls.CHAINCODES = '1'
        cls.ENDORSERS = '4'
        cls.RUN_TIME_HOURS = '1'
        check_output(['./generateCfgTrx.sh {0} {1}'.format(cls.CHANNEL_NAME, cls.CHANNELS)], cwd='../../envsetup', shell=True)
        check_output(['docker-compose -f docker-compose.yaml up -d'], cwd='../../envsetup', shell=True)

    @classmethod
    def tearDownClass(cls):
        check_output(['docker-compose -f docker-compose.yaml down'], cwd='../../envsetup', shell=True)
        delete_this = ['__pycache__', '.cache']
        for item in delete_this:
            shutil.rmtree(item)

#################################################################################

    def runIt(self, command, scriptName):
        cmd = \
            '/opt/gopath/src/github.com/hyperledger/fabric/test/tools/AuctionApp/%s %s %s %s %s %s %s' \
            % (
            scriptName,
            self.CHANNEL_NAME,
            self.CHANNELS,
            self.CHAINCODES,
            self.ENDORSERS,
            self.RUN_TIME_HOURS,
            command,
            )
        output = \
            check_output(['docker exec cli bash -c "{0}"'.format(cmd)],
                         shell=True)
        return output

    def test_FAB3934_1_Create_Channel(self):
        '''
            Network: 1 Ord, 2 Org, 4 Peers, 2 Chan, 1 CC
            Description: This test is used to creates channels
            Passing criteria: Creating Channel is successful
        '''
        output = self.runIt('createChannel', 'api_driver.sh')
        self.assertIn('Creating Channel is successful', output)


    def test_FAB3934_2_Join_Channel(self):
        '''
            Network: 1 Ord, 2 Org, 4 Peers, 2 Chan, 1 CC
            Description: This test is used to join peers on all channels
            Passing criteria: Join Channel is successful
        '''
        output = self.runIt('joinChannel', 'api_driver.sh')
        self.assertIn('Join Channel is successful', output)


    def test_FAB3934_3_Install_Chaincode(self):
        '''
            Network: 1 Ord, 2 Org, 4 Peers, 2 Chan, 1 CC
            Description: This test is used to install all chaincodes on all peers
            Passing criteria: Installing chaincode is successful
        '''
        output = self.runIt('installChaincode', 'api_driver.sh')
        self.assertIn('Installing chaincode is successful', output)


    def test_FAB3934_4_Instantiate_Chaincode(self):
        '''
            Network: 1 Ord, 2 Org, 4 Peers, 2 Chan, 1 CC
            Description: This test is used to instantiate all chaincodes on all channels
            Passing criteria: Instantiating chaincode is successful
        '''
        output = self.runIt('instantiateChaincode', 'api_driver.sh')
        self.assertIn('Instantiating chaincode is successful', output)


    def test_FAB3934_5_Post_Users(self):
        '''
            Network: 1 Ord, 2 Org, 4 Peers, 2 Chan, 1 CC
            Description: This test is used to submit users to the auction application
            Passing criteria: Posting Users transaction is successful
        '''
        output = self.runIt('postUsers', 'api_driver.sh')
        self.assertIn('Posting Users transaction is successful', output)


    def test_FAB3934_6_Get_Users(self):
        '''
            Network: 1 Ord, 2 Org, 4 Peers, 2 Chan, 1 CC
            Description: This test is used to query users submitted to the auction application
            Passing criteria: Get Users transaction is successful
        '''
        output = self.runIt('getUsers', 'api_driver.sh')
        self.assertIn('Get Users transaction is successful', output)

    def test_FAB3934_7_Download_Images(self):
        '''
            Network: 1 Ord, 2 Org, 4 Peers, 2 Chan, 1 CC
            Description: This test is used to download auction images on all chaincode containers
            Passing criteria: Download Images transaction is successful
        '''
        output = self.runIt('downloadImages', 'api_driver.sh')
        self.assertIn('Download Images transaction is successful',
                      output)


    def test_FAB3934_8_Post_Items(self):
        '''
            Network: 1 Ord, 2 Org, 4 Peers, 2 Chan, 1 CC
            Description: This test is used to submit auction items for a user to the auction application
            Passing criteria: Post Items transaction is successful
        '''
        output = self.runIt('postItems', 'api_driver.sh')
        self.assertIn('Post Items transaction is successful', output)


    def test_FAB3934_9_Auction_Invoke(self):
        '''
            Network: 1 Ord, 2 Org, 4 Peers, 2 Chan, 1 CC
            Description: This test runs for 1 Hr. It is used to open auction on the submitted items,
            submit bids to the auction, close auction and finally transfer item to a user.
            Passing criteria: Open Auction/Submit Bids/Close Auction/Transfer Item transaction(s) are successful
        '''
        output = self.runIt('auctionInvokes', 'api_driver.sh')
        self.assertIn('Open Auction/Submit Bids/Close Auction/Transfer Item transaction(s) are successful'
                      , output)
