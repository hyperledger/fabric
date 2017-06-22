# Copyright IBM Corp. 2017 All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#


import subprocess
import os
import sys
from shutil import copyfile

CHANNEL_PROFILE = "SysTestChannel"

def generateConfig(channelID, profile, ordererProfile, projectName, block="orderer.block"):
    # Save all the files to a specific directory for the test
    testConfigs = "configs/%s" % projectName
    if not os.path.isdir(testConfigs):
        os.mkdir(testConfigs)

    configFile = "configtx.yaml"
    if os.path.isfile("configs/%s.yaml" % channelID):
        configFile = "%s.yaml" % channelID

    copyfile("configs/%s" % configFile, "%s/configtx.yaml" % testConfigs)

    # Copy config to orderer org structures
    for orgDir in os.listdir("./{0}/ordererOrganizations".format(testConfigs)):
        copyfile("{0}/configtx.yaml".format(testConfigs),
                 "{0}/ordererOrganizations/{1}/msp/config.yaml".format(testConfigs,
                                                                       orgDir))
    # Copy config to peer org structures
    for orgDir in os.listdir("./{0}/peerOrganizations".format(testConfigs)):
        copyfile("{0}/configtx.yaml".format(testConfigs),
                 "{0}/peerOrganizations/{1}/msp/config.yaml".format(testConfigs,
                                                                    orgDir))
        copyfile("{0}/configtx.yaml".format(testConfigs),
                 "{0}/peerOrganizations/{1}/users/Admin@{1}/msp/config.yaml".format(testConfigs,
                                                                                    orgDir))
    try:
        command = ["configtxgen", "-profile", ordererProfile,
                   "-outputBlock", block,
                   "-channelID", channelID]
        subprocess.check_call(command, cwd=testConfigs)

        generateChannelConfig(channelID, profile, projectName)
        generateChannelAnchorConfig(channelID, profile, projectName)
    except:
        print("Unable to generate channel config data: {0}".format(sys.exc_info()[1]))

def generateChannelConfig(channelID, profile, projectName):
    testConfigs = "configs/%s" % projectName
    try:
        command = ["configtxgen", "-profile", profile,
                   "-outputCreateChannelTx", "%s.tx" % channelID,
                   "-channelID", channelID]
        subprocess.check_call(command, cwd=testConfigs)
    except:
        print("Unable to generate channel config data: {0}".format(sys.exc_info()[1]))

def generateChannelAnchorConfig(channelID, profile, projectName):
    testConfigs = "configs/%s" % projectName
    for org in os.listdir("./{0}/peerOrganizations".format(testConfigs)):
        try:
            command = ["configtxgen", "-profile", profile,
                       "-outputAnchorPeersUpdate", "{0}{1}Anchor.tx".format(org, channelID),
                       "-channelID", channelID,
                       "-asOrg", org.title().replace('.', '')]
            subprocess.check_call(command, cwd=testConfigs)
        except:
            print("Unable to generate channel anchor config data: {0}".format(sys.exc_info()[1]))

def generateCrypto(projectName):
    # Save all the files to a specific directory for the test
    testConfigs = "configs/%s" % projectName
    if not os.path.isdir(testConfigs):
        os.mkdir(testConfigs)
    try:
        subprocess.check_call(["cryptogen", "generate",
                               '--output={0}'.format(testConfigs),
                               '--config=./configs/crypto.yaml'],
                              env=os.environ)
    except:
        print("Unable to generate crypto material: {0}".format(sys.exc_info()[1]))
