#
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

import json
from behave import *
from bdd_test_util import cli_call, bdd_log
from peer_basic_impl import getAttributeFromJSON

@when(u'I execute "{command}" in container {containerAlias}')
def step_impl(context, command, containerAlias):
    bdd_log("Run command: \"{0}\" in container {1}".format(command, containerAlias))
    executeCommandInContainer(context, command, containerAlias)
    bdd_log("stdout: {0}".format(context.command["stdout"]))
    bdd_log("stderr: {0}".format(context.command["stderr"]))
    bdd_log("returnCode: {0}".format(context.command["returnCode"]))

def executeCommandInContainer(context, command, containerAlias):
    fullContainerName = context.containerAliasMap[containerAlias].name
    command = "docker exec {} {}".format(fullContainerName, command)
    executeCommand(context, command)

def executeCommand(context, command):
    # cli_call expects an array of arguments, hence splitting here.
    commandArgs = command.split()
    stdout, stderr, retcode = cli_call(commandArgs, expect_success=False)

    context.command = {
        "stdout": stdout,
        "stderr": stderr,
        "returnCode": retcode
    }

@then(u'the command should not complete successfully')
def step_impl(context):
    assert not commandCompletedSuccessfully(context)

@then(u'the command should complete successfully')
def step_impl(context):
    assert commandCompletedSuccessfully(context)

def commandCompletedSuccessfully(context):
    return isSuccessfulReturnCode(context.command["returnCode"])

def isSuccessfulReturnCode(returnCode):
    return returnCode == 0

@then(u'{stream} should contain JSON')
def step_impl(context, stream):
    assertIsJson(context.command[stream])

@then(u'{stream} should contain JSON with "{attribute}" array of length {length}')
def step_impl(context, stream, attribute, length):
    data = context.command[stream]
    assertIsJson(data)

    json = decodeJson(data)
    array = getAttributeFromJSON(attribute, json)
    assertLength(array, int(length))

@then(u'I should get result with "{expectResult}"')
def step_impl(context, expectResult):
    assert context.command["stdout"].strip('\n') == expectResult

def assertIsJson(data):
    assert isJson(data), "Data is not in JSON format"

def isJson(data):
    try:
        decodeJson(data)
    except ValueError:
        return False

    return True

def decodeJson(data):
    return json.loads(data)

def assertLength(array, length):
    arrayLength = len(array)
    assert arrayLength == length, "Unexpected array length. Expected {}, got {}".format(length, arrayLength)
