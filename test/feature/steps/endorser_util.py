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
import time

try:
    pbFilePath = "../../bddtests"
    sys.path.insert(0, pbFilePath)
    from peer import chaincode_pb2
except:
    print("ERROR! Unable to import the protobuf libraries from the hyperledger/fabric/bddtests directory: {0}".format(sys.exc_info()[0]))
    sys.exit(1)

# The default channel ID
SYS_CHANNEL_ID = "behavesyschan"
TEST_CHANNEL_ID = "behavesystest"


def get_chaincode_deploy_spec(projectDir, ccType, path, name, args):
    subprocess.call(["peer", "chaincode", "package",
                     "-n", name,
                     "-c", '{"Args":{0}}'.format(args),
                     "-p", path,
                     "configs/{0}/test.file".format(projectDir)], shell=True)
    ccDeploymentSpec = chaincode_pb2.ChaincodeDeploymentSpec()
    with open("test.file", 'rb') as f:
        ccDeploymentSpec.ParseFromString(f.read())
    return ccDeploymentSpec


def install_chaincode(context, chaincode, peers):
    configDir = "/var/hyperledger/configs/{0}".format(context.composition.projectName)
    output = {}
    for peer in peers:
        peerParts = peer.split('.')
        org = '.'.join(peerParts[1:])
        command = ["/bin/bash", "-c",
                   '"CORE_PEER_MSPCONFIGPATH={0}/peerOrganizations/{1}/users/Admin@{1}/msp'.format(configDir, org),
                   'CORE_PEER_LOCALMSPID={0}'.format(org),
                   'CORE_PEER_ID={0}'.format(peer),
                   'CORE_PEER_ADDRESS={0}:7051'.format(peer),
                   "peer", "chaincode", "install",
                   #"--lang", chaincode['language'],
                   "--name", chaincode['name'],
                   "--version", str(chaincode.get('version', 0)),
                   #"--channelID", str(chaincode.get('channelID', TEST_CHANNEL_ID)),
                   "--path", chaincode['path']]
        if "orderers" in chaincode:
            command = command + ["--orderer", '{0}:7050'.format(chaincode["orderers"][0])]
        if "user" in chaincode:
            command = command + ["--username", chaincode["user"]]
        if "policy" in chaincode:
            command = command + ["--policy", chaincode["policy"]]
        command.append('"')
        ret = context.composition.docker_exec(command, ['cli'])
        output[peer] = ret['cli']
#        assert "Error occurred" not in str(ret['cli']), str(ret['cli'])
    print("[{0}]: {1}".format(" ".join(command), output))
    return output


def instantiate_chaincode(context, chaincode, containers):
    configDir = "/var/hyperledger/configs/{0}".format(context.composition.projectName)
    args = chaincode.get('args', '[]').replace('"', r'\"')
    command = ["/bin/bash", "-c",
               '"CORE_PEER_MSPCONFIGPATH={0}/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp'.format(configDir),
               'CORE_PEER_TLS_ROOTCERT_FILE={0}/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt'.format(configDir),
               'CORE_PEER_LOCALMSPID=org1.example.com',
               'CORE_PEER_ID=peer0.org1.example.com',
               'CORE_PEER_ADDRESS=peer0.org1.example.com:7051',
               "peer", "chaincode", "instantiate",
               #"--lang", chaincode['language'],
               "--name", chaincode['name'],
               "--version", str(chaincode.get('version', 0)),
               "--channelID", str(chaincode.get('channelID', TEST_CHANNEL_ID)),
               "--ctor", r"""'{\"Args\": %s}'""" % (args)]
    if "orderers" in chaincode:
        command = command + ["--orderer", '{0}:7050'.format(chaincode["orderers"][0])]
    if "user" in chaincode:
        command = command + ["--username", chaincode["user"]]
    if "policy" in chaincode:
        command = command + ["--policy", chaincode["policy"]]
    command.append('"')
    ret = context.composition.docker_exec(command, ['peer0.org1.example.com'])
#    assert "Error occurred" not in str(ret['peer0.org1.example.com']), str(ret['peer0.org1.example.com'])
    print("[{0}]: {1}".format(" ".join(command), ret))
    return ret


def create_channel(context, containers, orderers, channelId=TEST_CHANNEL_ID):
    configDir = "/var/hyperledger/configs/{0}".format(context.composition.projectName)
    ret = context.composition.docker_exec(["ls", configDir], containers)

    command = ["/bin/bash", "-c",
               '"CORE_PEER_MSPCONFIGPATH={0}/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp'.format(configDir),
               'CORE_PEER_LOCALMSPID=org1.example.com',
               'CORE_PEER_ID=peer0.org1.example.com',
               'CORE_PEER_ADDRESS=peer0.org1.example.com:7051',
               "peer", "channel", "create",
               "--file", "/var/hyperledger/configs/{0}/{1}.tx".format(context.composition.projectName, channelId),
               "--channelID", channelId,
               "--timeout", "120", # This sets the timeout for the channel creation instead of the default 5 seconds
               "--orderer", '{0}:7050"'.format(orderers[0])]
    print("Create command: {0}".format(command))
    output = context.composition.docker_exec(command, ['cli'])

#    for item in output:
#        assert "Error occurred" not in str(output[item]), str(output[item])

    # For now, copy the channel block to the config directory
    output = context.composition.docker_exec(["cp",
                                              "{0}.block".format(channelId),
                                              configDir],
                                             ['cli'])
    #output = context.composition.docker_exec(["ls", configDir], ['cli'])
    #print("Create: {0}".format(output))
    print("[{0}]: {1}".format(" ".join(command), output))
    return output


def fetch_channel(context, peers, orderers, channelId=TEST_CHANNEL_ID):
    configDir = "/var/hyperledger/configs/{0}".format(context.composition.projectName)
    for peer in peers:
        peerParts = peer.split('.')
        org = '.'.join(peerParts[1:])
        command = ["/bin/bash", "-c",
                   '"CORE_PEER_MSPCONFIGPATH={0}/peerOrganizations/{1}/users/Admin@{1}/msp'.format(configDir, org),
                   "peer", "channel", "fetch", "config",
                   "/var/hyperledger/configs/{0}/{1}.block".format(context.composition.projectName, channelId),
                   "--file", "/var/hyperledger/configs/{0}/{1}.tx".format(context.composition.projectName, channelId),
                   "--channelID", channelId,
                   "--orderer", '{0}:7050"'.format(orderers[0])]
        output = context.composition.docker_exec(command, [peer])
        print("Fetch: {0}".format(str(output)))
#        assert "Error occurred" not in str(output[peer]), str(output[peer])
    print("[{0}]: {1}".format(" ".join(command), output))
    return output


def join_channel(context, peers, orderers, channelId=TEST_CHANNEL_ID):
    configDir = "/var/hyperledger/configs/{0}".format(context.composition.projectName)

    for peer in peers:
        peerParts = peer.split('.')
        org = '.'.join(peerParts[1:])
        command = ["/bin/bash", "-c",
                   '"CORE_PEER_MSPCONFIGPATH={0}/peerOrganizations/{1}/users/Admin@{1}/msp'.format(configDir, org),
                   "peer", "channel", "join",
                   "--blockpath", '/var/hyperledger/configs/{0}/{1}.block"'.format(context.composition.projectName, channelId)]
        count = 0
        output = "Error"
        while count < 5 and "Error" in output:
            output = context.composition.docker_exec(command, [peer])
            #print("Join: {0}".format(str(output)))
            time.sleep(2)
            count = count + 1
            output = output[peer]

        # If the LedgerID doesn't already exist check for other errors
#        if "due to LedgerID already exists" not in output:
#            assert "Error occurred" not in str(output), str(output)
    print("[{0}]: {1}".format(" ".join(command), output))
    return output


def invoke_chaincode(context, chaincode, orderers, peer, channelId=TEST_CHANNEL_ID):
    configDir = "/var/hyperledger/configs/{0}".format(context.composition.projectName)
    args = chaincode.get('args', '[]').replace('"', r'\"')
    peerParts = peer.split('.')
    org = '.'.join(peerParts[1:])
    command = ["/bin/bash", "-c",
               '"CORE_PEER_MSPCONFIGPATH={0}/peerOrganizations/{1}/users/Admin@{1}/msp'.format(configDir, org),
               "peer", "chaincode", "invoke",
               "--name", chaincode['name'],
               "--ctor", r"""'{\"Args\": %s}'""" % (args),
               "--channelID", channelId,
               "--orderer", '{0}:7050"'.format(orderers[0])]
    output = context.composition.docker_exec(command, [peer])
    print("Invoke[{0}]: {1}".format(" ".join(command), str(output)))
    #assert "Error occurred" not in output[peer], output[peer]
    return output


def query_chaincode(context, chaincode, peer, channelId=TEST_CHANNEL_ID):
    configDir = "/var/hyperledger/configs/{0}".format(context.composition.projectName)
    peerParts = peer.split('.')
    org = '.'.join(peerParts[1:])
    args = chaincode.get('args', '[]').replace('"', r'\"')
    command = ["/bin/bash", "-c",
               '"CORE_PEER_MSPCONFIGPATH={0}/peerOrganizations/{1}/users/Admin@{1}/msp'.format(configDir, org),
               'CORE_PEER_TLS_ROOTCERT_FILE={0}/peerOrganizations/{1}/peers/{2}/tls/ca.crt'.format(configDir, org, peer),
               "peer", "chaincode", "query",
               "--name", chaincode['name'],
               "--ctor", r"""'{\"Args\": %s}'""" % (args),
               "--channelID", channelId, '"']
    print("Query Exec command: {0}".format(" ".join(command)))
    return context.composition.docker_exec(command, [peer])


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

    chaincode.update({"orderers": orderers,
                      "channelID": channelId,
                      })
    create_channel(context, containers, orderers, channelId)
    #fetch_channel(context, peers, orderers, channelId)
    join_channel(context, peers, orderers, channelId)
    install_chaincode(context, chaincode, peers)
    instantiate_chaincode(context, chaincode, containers)
