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

@when(u'a message is broadcasted')
def step_impl(context):
    broadcast_impl(context, 1)

@when(u'{count} unique messages are broadcasted')
def broadcast_impl(context, count):
    pass

@then(u'all {count} messages are delivered within {timeout} seconds')
def step_impl(context, count, timeout):
    pass

@then(u'all {count} messages are delivered in {numBlocks} within {timeout} seconds')
def step_impl(context, count, numBlocks, timeout):
    pass

@then(u'we get a successful broadcast response')
def step_impl(context):
    recv_broadcast_impl(context, 1)

@then(u'we get {count} successful broadcast responses')
def recv_broadcast_impl(context, count):
    pass

@then(u'the broadcasted message is delivered')
def step_impl(context):
    verify_deliver_impl(context, 1, 1)

@then(u'all {count} messages are delivered in {numBlocks} block')
def verify_deliver_impl(context, count, numBlocks):
    pass

