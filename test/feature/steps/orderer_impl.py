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
import os
import subprocess
import time


ORDERER_TYPES = ["solo",
                 "kafka",
                 "solo-msp"]

PROFILE_TYPES = {"solo": "SampleInsecureSolo",
                 "kafka": "SampleInsecureKafka",
                 "solo-msp": "SampleSingleMSPSolo"}


@given(u'a bootstrapped orderer network of type {networkType}')
def step_impl(context, networkType):
    pass

@given(u'an unbootstrapped network using "{dockerFile}"')
def compose_impl(context, dockerFile):
    pass

@given(u'an orderer connected to the kafka cluster')
def step_impl(context):
    pass

@given(u'the orderer Batchsize MaxMessageCount is {maxMsgCount}')
def step_impl(context, maxMsgCount):
    pass

@given(u'the orderer BatchTimeout is {timeout} {units}')
def step_impl(context, timeout, units):
    pass

@given(u'a certificate from {organization} is added to the kafka orderer network')
def step_impl(context, organization):
    pass

@given(u'the kafka default replication factor is {factor}')
def step_impl(context, factor):
    pass

@given(u'a kafka cluster')
def step_impl(context):
    pass

@when(u'a message is broadcasted')
def step_impl(context):
    broadcast_impl(context, 1)

@when(u'{count} unique messages are broadcasted')
def broadcast_impl(context, count):
    pass

@when(u'the topic partition leader is stopped')
def step_impl(context):
    pass

@when(u'a new organization {organization} certificate is added')
def step_impl(context, organization):
    pass

@when(u'authorization for {organization} is removed from the kafka cluster')
def step_impl(context, organization):
    pass

@when(u'authorization for {organization} is added to the kafka cluster')
def step_impl(context, organization):
    pass

@then(u'the broadcasted message is delivered')
def step_impl(context):
    verify_deliver_impl(context, 1, 1)

@then(u'all {count} messages are delivered in {numBlocks} block')
def step_impl(context, count, numBlocks):
    verify_deliver_impl(context, count, numBlocks)

@then(u'all {count} messages are delivered within {timeout} seconds')
def step_impl(context, count, timeout):
    verify_deliver_impl(context, count, None, timeout)

@then(u'all {count} messages are delivered in {numBlocks} within {timeout} seconds')
def verify_deliver_impl(context, count, numBlocks, timeout=60):
    pass

@then(u'we get a successful broadcast response')
def step_impl(context):
    recv_broadcast_impl(context, 1)

@then(u'we get {count} successful broadcast responses')
def recv_broadcast_impl(context, count):
    pass

@then(u'the {organization} cannot connect to the kafka cluster')
def step_impl(context, organization):
    pass

@then(u'the {organization} is able to connect to the kafka cluster')
def step_impl(context, organization):
    pass

@then(u'the zookeeper notifies the orderer of the disconnect')
def step_impl(context):
    pass

@then(u'the orderer functions successfully')
def step_impl(context):
    # Check the logs for certain key info - be sure there are no errors in the logs
    pass

@then(u'the orderer stops sending messages to the cluster')
def step_impl(context):
    pass
