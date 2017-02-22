
# Copyright IBM Corp. 2016 All Rights Reserved.
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

import re

import bdd_test_util

import grpc

def getGRPCChannel(ipAddress):
    channel = grpc.insecure_channel("{0}:{1}".format(ipAddress, 7051), options = [('grpc.max_message_length', 100*1024*1024)])
    print("Returning GRPC for address: {0}".format(ipAddress))
    return channel

def getGRPCChannelAndUser(context, enrollId):
    '''Returns a tuple of GRPC channel and UserRegistration instance.  The channel is open to the composeService that the user registered with.'''
    userRegistration = bdd_test_util.getUserRegistration(context, enrollId)

    # Get the IP address of the server that the user registered on
    ipAddress = bdd_test_util.ipFromContainerNamePart(userRegistration.composeService, context.compose_containers)

    channel = getGRPCChannel(ipAddress)

    return (channel, userRegistration)


def getDeployment(context, ccAlias):
    '''Return a deployment with chaincode alias from prior deployment, or None if not found'''
    deployment = None
    if 'deployments' in context:
        pass
    else:
        context.deployments = {}
    if ccAlias in context.deployments:
        deployment = context.deployments[ccAlias]
    # else:
    #     raise Exception("Deployment alias not found: '{0}'.  Are you sure you have deployed a chaincode with this alias?".format(ccAlias))
    return deployment

def getArgsFromContextForUser(context, enrollId):
    # Update the chaincodeSpec ctorMsg for invoke
    args = []
    if 'table' in context:
        if context.table:
            # There are function arguments
            userRegistration = bdd_test_util.getUserRegistration(context, enrollId)
            # Allow the user to specify expressions referencing tags in the args list
            pattern = re.compile('\{(.*)\}$')
            for arg in context.table[0].cells:
                m = pattern.match(arg)
                if m:
                    # tagName reference found in args list
                    tagName = m.groups()[0]
                    # make sure the tagName is found in the users tags
                    assert tagName in userRegistration.tags, "TagName '{0}' not found for user '{1}'".format(tagName, userRegistration.getUserName())
                    args.append(userRegistration.tags[tagName])
                else:
                    #No tag referenced, pass the arg
                    args.append(arg)
    return args

def toStringArray(items):
    itemsAsStr = []
    for item in items:
        if type(item) == str:
            itemsAsStr.append(item)
        elif type(item) == unicode:
            itemsAsStr.append(str(item))
        else:
            raise Exception("Error tring to convert to string: unexpected type '{0}'".format(type(item)))
    return itemsAsStr


