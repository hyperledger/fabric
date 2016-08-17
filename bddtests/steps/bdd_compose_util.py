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

import os
import time
import re

from bdd_test_util import cli_call

REST_PORT = "7050"

class ContainerData:
    def __init__(self, containerName, ipAddress, envFromInspect, composeService):
        self.containerName = containerName
        self.ipAddress = ipAddress
        self.envFromInspect = envFromInspect
        self.composeService = composeService

    def getEnv(self, key):
        envValue = None
        for val in self.envFromInspect:
            if val.startswith(key):
                envValue = val[len(key):]
                break
        if envValue == None:
            raise Exception("ENV key not found ({0}) for container ({1})".format(key, self.containerName))
        return envValue

def getDockerComposeFileArgsFromYamlFile(compose_yaml):
    parts = compose_yaml.split()
    args = []
    for part in parts:
        args = args + ["-f"] + [part]
    return args

def parseComposeOutput(context):
    """Parses the compose output results and set appropriate values into context.  Merges existing with newly composed."""
    # Use the prefix to get the container name
    containerNamePrefix = os.path.basename(os.getcwd()) + "_"
    containerNames = []
    for l in context.compose_error.splitlines():
        tokens = l.split()
        print(tokens)
        if 1 < len(tokens):
            thisContainer = tokens[1]
            if containerNamePrefix not in thisContainer:
               thisContainer = containerNamePrefix + thisContainer + "_1"
            if thisContainer not in containerNames:
               containerNames.append(thisContainer)

    print("Containers started: ")
    print(containerNames)
    # Now get the Network Address for each name, and set the ContainerData onto the context.
    containerDataList = []
    for containerName in containerNames:
        output, error, returncode = \
            cli_call(context, ["docker", "inspect", "--format",  "{{ .NetworkSettings.IPAddress }}", containerName], expect_success=True)
        print("container {0} has address = {1}".format(containerName, output.splitlines()[0]))
        ipAddress = output.splitlines()[0]

        # Get the environment array
        output, error, returncode = \
            cli_call(context, ["docker", "inspect", "--format",  "{{ .Config.Env }}", containerName], expect_success=True)
        env = output.splitlines()[0][1:-1].split()

        # Get the Labels to access the com.docker.compose.service value
        output, error, returncode = \
            cli_call(context, ["docker", "inspect", "--format",  "{{ .Config.Labels }}", containerName], expect_success=True)
        labels = output.splitlines()[0][4:-1].split()
        dockerComposeService = [composeService[27:] for composeService in labels if composeService.startswith("com.docker.compose.service:")][0]
        print("dockerComposeService = {0}".format(dockerComposeService))
        print("container {0} has env = {1}".format(containerName, env))
        containerDataList.append(ContainerData(containerName, ipAddress, env, dockerComposeService))
    # Now merge the new containerData info with existing
    newContainerDataList = []
    if "compose_containers" in context:
        # Need to merge I new list
        newContainerDataList = context.compose_containers
    newContainerDataList = newContainerDataList + containerDataList

    setattr(context, "compose_containers", newContainerDataList)
    print("")

def allContainersAreReadyWithinTimeout(context, timeout):
    timeoutTimestamp = time.time() + timeout
    formattedTime = time.strftime("%X", time.localtime(timeoutTimestamp))
    print("All containers should be up by {}".format(formattedTime))

    for container in context.compose_containers:
        if not containerIsReadyByTimestamp(container, timeoutTimestamp):
            return False

    print("All containers in ready state, ready to proceed")
    return True

def containerIsReadyByTimestamp(container, timeoutTimestamp):
    while containerIsNotReady(container):
        if timestampExceeded(timeoutTimestamp):
            print("Timed out waiting for {}".format(container.containerName))
            return False

        print("{} not ready, waiting...".format(container.containerName))
        time.sleep(1)

    print("{} now available".format(container.containerName))
    return True

def timestampExceeded(timeoutTimestamp):
    return time.time() > timeoutTimestamp

def containerIsNotReady(container):
    return not containerIsReady(container)

def containerIsReady(container):
    isReady = tcpPortsAreReady(container)
    isReady = isReady and restPortRespondsIfContainerIsPeer(container)

    return isReady

def tcpPortsAreReady(container):
    netstatOutput = getContainerNetstatOutput(container.containerName)

    for line in netstatOutput.splitlines():
        if re.search("ESTABLISHED|LISTEN", line):
            return True

    print("No TCP connections are ready in container {}".format(container.containerName))
    return False

def getContainerNetstatOutput(containerName):
    command = ["docker", "exec", containerName, "netstat", "-atun"]
    stdout, stderr, returnCode = cli_call(None, command, expect_success=False)

    return stdout

def restPortRespondsIfContainerIsPeer(container):
    containerName = container.containerName
    command = ["docker", "exec", containerName, "curl", "localhost:" + REST_PORT]

    if containerIsPeer(container):
        stdout, stderr, returnCode = cli_call(None, command, expect_success=False)

        if returnCode != 0:
            print("Connection to REST Port on {} failed".format(containerName))

        return returnCode == 0

    return True

def containerIsPeer(container):
    netstatOutput = getContainerNetstatOutput(container.containerName)

    for line in netstatOutput.splitlines():
        if re.search(REST_PORT, line) and re.search("LISTEN", line):
            return True

    return False