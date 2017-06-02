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

import os
import sys
import subprocess

try:
    pbFilePath = "../../bddtests"
    sys.path.insert(0, pbFilePath)
    from common import common_pb2
    from peer import chaincode_pb2
    from peer import proposal_pb2
    from peer import transaction_pb2
except:
    print("ERROR! Unable to import the protobuf libraries from the hyperledger/fabric/bddtests directory: {0}".format(sys.exc_info()[0]))
    sys.exit(1)


# The default channel ID
TEST_CHANNEL_ID = "behavesystest"


def get_chaincode_deploy_spec(projectDir, ccType, path, name, args):
    pass

def install_chaincode(context, chaincode, containers):
    pass

def instantiate_chaincode(context, chaincode, containers):
    pass

def create_channel(context, containers, orderers, channelId=TEST_CHANNEL_ID):
    pass

def join_channel(context, peers, orderers, channelId=TEST_CHANNEL_ID):
    pass

def invoke_chaincode(context, chaincode, orderers, containers, channelId=TEST_CHANNEL_ID):
    pass

def query_chaincode(context, chaincode, containers, channelId=TEST_CHANNEL_ID):
    pass

def get_orderers(context):
    orderers = []
    for container in context.composition.collectServiceNames():
        if container.startswith("orderer"):
            orderers.append(container)
    return orderers


def get_peers(context):
    peers = []
    for container in context.composition.collectServiceNames():
        if container.startswith("peer"):
            peers.append(container)
    return peers


def deploy_chaincode(context, chaincode, containers, channelId=TEST_CHANNEL_ID):
    for container in containers:
        assert container in context.composition.collectServiceNames(), "Unknown component '{0}'".format(container)

    orderers = get_orderers(context)
    peers = get_peers(context)
    assert orderers != [], "There are no active orderers in this network"

    create_channel(context, containers, orderers, channelId)
    join_channel(context, peers, orderers, channelId)
    install_chaincode(context, chaincode, containers)
    instantiate_chaincode(context, chaincode, containers)
