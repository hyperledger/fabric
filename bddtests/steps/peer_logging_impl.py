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

import time
from behave import *
from bdd_test_util import cli_call

@then(u'I wait up to {waitTime} seconds for an error in the logs for peer {peerName}')
def step_impl(context, waitTime, peerName):
    timeout = time.time() + float(waitTime)
    hasError = False

    while timeout > time.time():
        stdout, stderr = getPeerLogs(context, peerName)
        hasError =  logHasError(stdout) or logHasError(stderr)

        if hasError:
            break

        time.sleep(1.0)

    assert hasError is True

def getPeerLogs(context, peerName):
    fullContainerName = context.containerAliasMap[peerName].name
    stdout, stderr, retcode = cli_call(["docker", "logs", fullContainerName], expect_success=True)

    return stdout, stderr

def logHasError(logText):
    # This seems to be an acceptable heuristic for detecting errors
    return logText.find("-> ERRO") >= 0

@then(u'ensure after {waitTime} seconds there are no errors in the logs for peer {peerName}')
def step_impl(context, waitTime, peerName):
    time.sleep(float(waitTime))
    stdout, stderr = getPeerLogs(context, peerName)

    assert logHasError(stdout) is False
    assert logHasError(stderr) is False