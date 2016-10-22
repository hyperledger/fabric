
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

import os
import re
import subprocess

def cli_call(arg_list, expect_success=True, env=os.environ.copy()):
    """Executes a CLI command in a subprocess and return the results.

    @param arg_list: a list command arguments
    @param expect_success: use False to return even if an error occurred when executing the command
    @return: (string, string, int) output message, error message, return code
    """
    # We need to run the cli command by actually calling the python command
    # the update-cli.py script has a #!/bin/python as the first line
    # which calls the system python, not the virtual env python we
    # setup for running the update-cli
    p = subprocess.Popen(arg_list, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
    output, error = p.communicate()
    if p.returncode != 0:
        if output is not None:
            print("Output:\n" + output)
        if error is not None:
            print("Error Message:\n" + error)
        if expect_success:
            raise subprocess.CalledProcessError(p.returncode, arg_list, output)
    return output, error, p.returncode

class UserRegistration:
    def __init__(self, secretMsg, composeService):
        self.secretMsg = secretMsg
        self.composeService = composeService
        self.tags = {}
        self.lastResult = None

    def getUserName(self):
        return self.secretMsg['enrollId']

# Registerses a user on a specific composeService
def registerUser(context, secretMsg, composeService):
    userName = secretMsg['enrollId']
    if 'users' in context:
        pass
    else:
        context.users = {}
    if userName in context.users:
        raise Exception("User already registered: {0}".format(userName))
    context.users[userName] = UserRegistration(secretMsg, composeService)

# Registerses a user on a specific composeService
def getUserRegistration(context, enrollId):
    userRegistration = None
    if 'users' in context:
        pass
    else:
        context.users = {}
    if enrollId in context.users:
        userRegistration = context.users[enrollId]
    else:
        raise Exception("User has not been registered: {0}".format(enrollId))
    return userRegistration


def ipFromContainerNamePart(namePart, containerDataList):
    """Returns the IPAddress based upon a name part of the full container name"""
    containerData = containerDataFromNamePart(namePart, containerDataList)

    if containerData == None:
        raise Exception("Could not find container with namePart = {0}".format(namePart))

    return containerData.ipAddress

def fullNameFromContainerNamePart(namePart, containerDataList):
    containerData = containerDataFromNamePart(namePart, containerDataList)

    if containerData == None:
        raise Exception("Could not find container with namePart = {0}".format(namePart))

    return containerData.containerName

def containerDataFromNamePart(namePart, containerDataList):
    for containerData in containerDataList:
        if containerData.composeService == namePart:
            return containerData
    raise Exception("composeService not found: {0}".format(namePart))

def getContainerDataValuesFromContext(context, aliases, callback):
    """Returns the IPAddress based upon a name part of the full container name"""
    assert 'compose_containers' in context, "compose_containers not found in context"
    values = []
    for namePart in aliases:
        for containerData in context.compose_containers:
            if containerData.composeService == namePart:
                values.append(callback(containerData))
                break
    return values

def start_background_process(context, program_name, arg_list):
    p = subprocess.Popen(arg_list, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    setattr(context, program_name, p)
