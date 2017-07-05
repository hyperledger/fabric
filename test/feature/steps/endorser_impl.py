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

from behave import *
import json
import time
import endorser_util
import config_util


@when(u'a user deploys chaincode at path "{path}" with {args} with name "{name}" to "{containerName}" on channel "{channel}"')
def deploy_impl(context, path, args, name, containerName, channel):
    # Be sure there is a transaction block for this channel
    config_util.generateChannelConfig(channel, config_util.CHANNEL_PROFILE, context.composition.projectName)

    chaincode = {
        "path": path,
        "language": "GOLANG",
        "name": name,
        "channelID": channel,
        "args": args,
    }
    context.results = endorser_util.deploy_chaincode(context, chaincode, [containerName], channel)
    # Save chaincode name and path and args
    context.chaincode = chaincode

@when(u'a user deploys chaincode at path "{path}" with {args} with name "{name}" on channel "{channel}"')
def step_impl(context, path, args, name, channel):
    deploy_impl(context, path, args, name, "peer0.org1.example.com", channel)

@when(u'a user deploys chaincode at path "{path}" with {args} with name "{name}"')
def step_impl(context, path, args, name):
    deploy_impl(context, path, args, name, "peer0.org1.example.com", endorser_util.TEST_CHANNEL_ID)

@when(u'a user deploys chaincode at path "{path}" with {args}')
def step_impl(context, path, args):
    deploy_impl(context, path, args, "mycc", "peer0.org1.example.com", endorser_util.TEST_CHANNEL_ID)

@when(u'a user deploys chaincode')
def step_impl(context):
    deploy_impl(context,
                "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02",
                '["init", "a", "100" , "b", "200"]',
                "mycc",
                "peer0.org1.example.com",
                (endorser_util.TEST_CHANNEL_ID))

@when(u'a user queries on the channel "{channel}" using chaincode named "{name}" with args {args} on "{component}"')
def query_impl(context, channel, name, args, component):
    # Temporarily sleep for 2 sec. This delay should be able to be removed once we start using the python sdk
    time.sleep(2)
    chaincode = {"args": args,
                 "name": name}
    context.result = endorser_util.query_chaincode(context, chaincode, component, channel)

@when(u'a user queries on the chaincode named "{name}" with args {args} on "{component}"')
def step_impl(context, name, args, component):
    query_impl(context, endorser_util.TEST_CHANNEL_ID, name, args, component)

@when(u'a user queries on the chaincode named "{name}" with args {args}')
def step_impl(context, name, args):
    query_impl(context, endorser_util.TEST_CHANNEL_ID, name, args, "peer0.org1.example.com")

@when(u'a user queries on the chaincode with args {args}')
def step_impl(context, args):
    query_impl(context, endorser_util.TEST_CHANNEL_ID, "mycc", args, "peer0.org1.example.com")

@when(u'a user queries on the chaincode named "{name}"')
def step_impl(context, name):
    query_impl(context, endorser_util.TEST_CHANNEL_ID, name, '["query","a"]', "peer0.org1.example.com")

@when(u'a user queries on the chaincode')
def step_impl(context):
    query_impl(context, endorser_util.TEST_CHANNEL_ID, "mycc", '["query","a"]', "peer0.org1.example.com")

@when(u'a user invokes {numInvokes} times on the channel "{channel}" using chaincode named "{name}" with args {args} on "{component}"')
def invokes_impl(context, numInvokes, channel, name, args, component):
    chaincode = {"args": args,
                 "name": name}
    orderers = endorser_util.get_orderers(context)
    for count in range(int(numInvokes)):
        context.result = endorser_util.invoke_chaincode(context, chaincode, orderers, component, channel)

@when(u'a user invokes {numInvokes} times on the channel "{channel}" using chaincode named "{name}" with args {args}')
def step_impl(context, numInvokes, channel, name, args):
    invokes_impl(context, numInvokes, channel, name, args, "peer0.org1.example.com")

@when(u'a user invokes {numInvokes} times using chaincode named "{name}" with args {args}')
def step_impl(context, numInvokes, name, args):
    invokes_impl(context, numInvokes, endorser_util.TEST_CHANNEL_ID, name, args, "peer0.org1.example.com")

@when(u'a user invokes on the chaincode named "{name}" with args {args}')
def step_impl(context, name, args):
    invokes_impl(context, 1, endorser_util.TEST_CHANNEL_ID, name, args, "peer0.org1.example.com")

@when(u'a user invokes {numInvokes} times on the chaincode')
def step_impl(context, numInvokes):
    invokes_impl(context, numInvokes, endorser_util.TEST_CHANNEL_ID, "mycc", '["invoke","a","b","5"]', "peer0.org1.example.com")

@when(u'a user invokes on the chaincode named "{name}"')
def step_impl(context, name):
    invokes_impl(context, 1, endorser_util.TEST_CHANNEL_ID, name, '["invoke","a","b","5"]', "peer0.org1.example.com")

@when(u'a user invokes on the chaincode')
def step_impl(context):
    invokes_impl(context, 1, endorser_util.TEST_CHANNEL_ID, "mycc", '["invoke","a","b","5"]', "peer0.org1.example.com")

@then(u'the chaincode is deployed')
def step_impl(context):
    # Temporarily sleep for 2 sec. This delay should be able to be removed once we start using the python sdk
    time.sleep(2)

    # Grab the key/value pair from the deploy
    key = value = None
    print(context.chaincode['args'])
    print(json.loads(context.chaincode['args']))
    args = json.loads(context.chaincode['args'])
    for arg in args:
        if arg == 'init':
            keyIndex = args.index(arg)+1
            valueIndex = args.index(arg)+2
            key = args[keyIndex]
            value = args[valueIndex]
    assert key is not None, "The last message was not a deploy: {0}".format(context.chaincode['args'])

    chaincode = {
        "path": context.chaincode['path'],
        "language": context.chaincode['language'],
        "name": context.chaincode['name'],
        "args": r'["query","{0}"]'.format(key),
    }
    orderers = endorser_util.get_orderers(context)
    peers = endorser_util.get_peers(context)
    result = endorser_util.query_chaincode(context, chaincode, peers[0], endorser_util.TEST_CHANNEL_ID)
    print(result)
    assert result[peers[0]] == "Query Result: {0}\n".format(value), "Expect {0} = {1}, received from the deploy: {2}".format(key, value, result[peers[0]])

@then(u'a user receives expected response of {response} from "{peer}"')
def expected_impl(context, response, peer):
    assert peer in context.result, "There is no response from {0}".format(peer)
    assert context.result[peer] == "Query Result: {0}\n".format(response), "Expected response was {0}; received {1}".format(response, context.result[peer])

@then(u'a user receives an error response of {response} from "{peer}"')
def step_impl(context, response, peer):
    assert peer in context.result, "There is no response from {0}".format(peer)
    assert context.result[peer] == "Error: {0}\n".format(response), "Expected response was {0}; received {1}".format(response, context.result[peer])

@then(u'a user receives expected response of {response}')
def step_impl(context, response):
    expected_impl(context, response, "peer0.org1.example.com")
