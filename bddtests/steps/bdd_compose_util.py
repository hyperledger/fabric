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

import os, time, re, requests

from bdd_request_util import httpGetToContainer, CORE_REST_PORT
from bdd_json_util import getAttributeFromJSON
from bdd_test_util import cli_call, bdd_log

class Container:
    def __init__(self, name, ipAddress, envFromInspect, composeService):
        self.name = name
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
            raise Exception("ENV key not found ({0}) for container ({1})".format(key, self.name))
        return envValue

    def __str__(self):
        return "{} - {}".format(self.name, self.ipAddress)

    def __repr__(self):
        return self.__str__()

DOCKER_COMPOSE_FOLDER = "bdd-docker"

def getDockerComposeFileArgsFromYamlFile(compose_yaml):
    parts = compose_yaml.split()
    args = []
    for part in parts:
        part = "{}/{}".format(DOCKER_COMPOSE_FOLDER, part)
        args = args + ["-f"] + [part]
    return args

def parseComposeOutput(context):
    """Parses the compose output results and set appropriate values into context.  Merges existing with newly composed."""
    containerNames = getContainerNamesFromContext(context)

    bdd_log("Containers started: ")
    bdd_log(containerNames)
    # Now get the Network Address for each name, and set the ContainerData onto the context.
    containerDataList = []
    for containerName in containerNames:
        ipAddress = getIpFromContainerName(containerName)
        env = getEnvironmentVariablesFromContainerName(containerName)
        dockerComposeService = getDockerComposeServiceForContainer(containerName)

        containerDataList.append(Container(containerName, ipAddress, env, dockerComposeService))
    # Now merge the new containerData info with existing
    newContainerDataList = []
    if "compose_containers" in context:
        # Need to merge I new list
        newContainerDataList = context.compose_containers
    newContainerDataList = newContainerDataList + containerDataList

    setattr(context, "compose_containers", newContainerDataList)
    bdd_log("")

def getContainerNamesFromContext(context):
    containerNames = []
    for l in context.compose_error.splitlines():
        tokens = l.split()

        if len(tokens) > 1:
            thisContainer = tokens[1]
            if hasattr(context, "containerAliasMap"):
               thisContainer = context.containerAliasMap.get(tokens[1], tokens[1])

            if thisContainer not in containerNames:
               containerNames.append(thisContainer)

    return containerNames

def getIpFromContainerName(containerName):
    output, error, returncode = \
            cli_call(["docker", "inspect", "--format",  "{{ .NetworkSettings.IPAddress }}", containerName], expect_success=True)
    bdd_log("container {0} has address = {1}".format(containerName, output.splitlines()[0]))

    return output.splitlines()[0]

def getEnvironmentVariablesFromContainerName(containerName):
    output, error, returncode = \
            cli_call(["docker", "inspect", "--format",  "{{ .Config.Env }}", containerName], expect_success=True)
    env = output.splitlines()[0][1:-1].split()
    bdd_log("container {0} has env = {1}".format(containerName, env))

    return env

def getDockerComposeServiceForContainer(containerName):
    # Get the Labels to access the com.docker.compose.service value
    output, error, returncode = \
        cli_call(["docker", "inspect", "--format",  "{{ .Config.Labels }}", containerName], expect_success=True)
    labels = output.splitlines()[0][4:-1].split()
    dockerComposeService = [composeService[27:] for composeService in labels if composeService.startswith("com.docker.compose.service:")][0]
    bdd_log("dockerComposeService = {0}".format(dockerComposeService))

    return dockerComposeService

def allContainersAreReadyWithinTimeout(context, timeout):
    allContainers = context.compose_containers
    return containersAreReadyWithinTimeout(context, allContainers, timeout)

def containersAreReadyWithinTimeout(context, containers, timeout):
    timeoutTimestamp = time.time() + timeout
    formattedTime = time.strftime("%X", time.localtime(timeoutTimestamp))
    bdd_log("All containers should be up by {}".format(formattedTime))

    for container in containers:
        if 'dbstore' in container.name:
            containers.remove(container)
        elif not containerIsInitializedByTimestamp(container, timeoutTimestamp):
            return False

    peersAreReady = peersAreReadyByTimestamp(context, containers, timeoutTimestamp)

    if peersAreReady:
        bdd_log("Containers in ready state, ready to proceed")

    return peersAreReady

def containerIsInitializedByTimestamp(container, timeoutTimestamp):
    while containerIsNotInitialized(container):
        if timestampExceeded(timeoutTimestamp):
            bdd_log("Timed out waiting for {} to initialize".format(container.name))
            return False

        bdd_log("{} not initialized, waiting...".format(container.name))
        time.sleep(1)

    bdd_log("{} now available".format(container.name))
    return True

def timestampExceeded(timeoutTimestamp):
    return time.time() > timeoutTimestamp

def containerIsNotInitialized(container):
    return not containerIsInitialized(container)

def containerIsInitialized(container):
    isReady = tcpPortsAreReady(container)
    isReady = isReady and restPortRespondsIfContainerIsPeer(container)

    return isReady

def tcpPortsAreReady(container):
    netstatOutput = getContainerNetstatOutput(container.name)

    for line in netstatOutput.splitlines():
        if re.search("ESTABLISHED|LISTEN", line):
            return True

    bdd_log("No TCP connections are ready in container {}".format(container.name))
    return False

def getContainerNetstatOutput(containerName):
    command = ["docker", "exec", containerName, "netstat", "-atun"]
    stdout, stderr, returnCode = cli_call(command, expect_success=False)

    return stdout

def restPortRespondsIfContainerIsPeer(container):
    containerName = container.name
    command = ["docker", "exec", containerName, "curl", "localhost:{}".format(CORE_REST_PORT)]

    if containerIsPeer(container):
        stdout, stderr, returnCode = cli_call(command, expect_success=False)

        if returnCode != 0:
            bdd_log("Connection to REST Port on {} failed".format(containerName))

        return returnCode == 0

    return True

def peersAreReadyByTimestamp(context, containers, timeoutTimestamp):
    peers = getPeerContainers(containers)
    bdd_log("Detected Peers: {}".format(peers))

    for peer in peers:
        if not peerIsReadyByTimestamp(context, peer, peers, timeoutTimestamp):
            return False

    return True

def getPeerContainers(containers):
    peers = []

    for container in containers:
        if containerIsPeer(container):
            peers.append(container)

    return peers

def containerIsPeer(container):
    # This is not an ideal way of detecting whether a container is a peer or not since
    # we are depending on the name of the container. Another way of detecting peers is
    # is to determine if the container is listening on the REST port. However, this method
    # may run before the listening port is ready. Hence, as along as the current
    # convention of vp[0-9] is adhered to this function will be good enough.
    if 'dbstore' not in container.name:
        return re.search("vp[0-9]+", container.name, re.IGNORECASE)
    return False

def peerIsReadyByTimestamp(context, peerContainer, allPeerContainers, timeoutTimestamp):
    while peerIsNotReady(context, peerContainer, allPeerContainers):
        if timestampExceeded(timeoutTimestamp):
            bdd_log("Timed out waiting for peer {}".format(peerContainer.name))
            return False

        bdd_log("Peer {} not ready, waiting...".format(peerContainer.name))
        time.sleep(1)

    bdd_log("Peer {} now available".format(peerContainer.name))
    return True

def peerIsNotReady(context, thisPeer, allPeers):
    return not peerIsReady(context, thisPeer, allPeers)

def peerIsReady(context, thisPeer, allPeers):
    connectedPeers = getConnectedPeersFromPeer(context, thisPeer)

    if connectedPeers is None:
        return False

    numPeers = len(allPeers)
    numConnectedPeers = len(connectedPeers)

    if numPeers != numConnectedPeers:
        bdd_log("Expected {} peers, got {}".format(numPeers, numConnectedPeers))
        bdd_log("Connected Peers: {}".format(connectedPeers))
        bdd_log("Expected Peers: {}".format(allPeers))

    return numPeers == numConnectedPeers

def getConnectedPeersFromPeer(context, thisPeer):
    response = httpGetToContainer(context, thisPeer, "/network/peers")

    if response.status_code != 200:
        return None

    return getAttributeFromJSON("peers", response.json())

def mapAliasesToContainers(context):
    aliasToContainerMap = {}

    for container in context.compose_containers:
        alias = extractAliasFromContainerName(container.name)
        aliasToContainerMap[alias] = container

    return aliasToContainerMap

def extractAliasFromContainerName(containerName):
    """ Take a compose created container name and extract the alias to which it
        will be refered. For example bddtests_vp1_0 will return vp0 """
    return containerName.split("_")[1]

def mapContainerNamesToContainers(context):
    nameToContainerMap = {}

    for container in context.compose_containers:
        name = container.name
        nameToContainerMap[name] = container

    return nameToContainerMap
