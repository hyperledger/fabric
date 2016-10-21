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
import uuid
import bdd_test_util
import peer_basic_impl

class Composition:

    def GetUUID():
        return str(uuid.uuid1()).replace('-','')

    def __init__(self, composeFilesYaml, projectName = GetUUID()):
        self.projectName = projectName
        self.containerDataList = []
        self.composeFilesYaml = composeFilesYaml
        self.issueCommand(["up", "--force-recreate", "-d"])

    def parseComposeFilesArg(self, composeFileArgs):
        args = [arg for sublist in [["-f", file] for file in composeFileArgs.split()] for arg in sublist]
        return args

    def getFileArgs(self):
        return self.parseComposeFilesArg(self.composeFilesYaml)

    def getEnv(self):
        myEnv = os.environ.copy()
        myEnv["COMPOSE_PROJECT_NAME"] = self.projectName
        myEnv["CORE_PEER_NETWORKID"] = self.projectName
        return myEnv

    def refreshContainerIDs(self):
        containers = self.issueCommand(["ps", "-q"]).split()
        return containers


    def issueCommand(self, args):
        cmdArgs = self.getFileArgs()+ args
        output, error, returncode = \
            bdd_test_util.cli_call(["docker-compose"] + cmdArgs, expect_success=True, env=self.getEnv())
        # Don't rebuild if ps command
        if args[0] !="ps":
            self.rebuildContainerData()
        return output

    def rebuildContainerData(self):
        self.containerDataList = []
        for containerID in self.refreshContainerIDs():
            output, error, returncode = \
                bdd_test_util.cli_call(["docker", "inspect", "--format",  "{{ .Name }}", containerID], expect_success=True)
            containerName = output.splitlines()[0][1:]
            #print("container has address = {0}".format(containerName))
            ipAddress = output.splitlines()[0]

            output, error, returncode = \
                bdd_test_util.cli_call(["docker", "inspect", "--format",  "{{ .NetworkSettings.IPAddress }}", containerID], expect_success=True)
            #print("container {0} has address = {1}".format(containerName, output.splitlines()[0]))
            ipAddress = output.splitlines()[0]

            # Get the environment array
            output, error, returncode = \
                bdd_test_util.cli_call(["docker", "inspect", "--format",  "{{ .Config.Env }}", containerID], expect_success=True)
            env = output.splitlines()[0][1:-1].split()

            # Get the Labels to access the com.docker.compose.service value
            output, error, returncode = \
                bdd_test_util.cli_call(["docker", "inspect", "--format",  "{{ .Config.Labels }}", containerID], expect_success=True)
            labels = output.splitlines()[0][4:-1].split()
            dockerComposeService = [composeService[27:] for composeService in labels if composeService.startswith("com.docker.compose.service:")][0]
            #print("dockerComposeService = {0}".format(dockerComposeService))
            #print("container {0} has env = {1}".format(containerName, env))
            self.containerDataList.append(peer_basic_impl.ContainerData(containerName, ipAddress, env, dockerComposeService))

    def decompose(self):
        self.issueCommand(["unpause"])
        self.issueCommand(["kill"])
        self.issueCommand(["rm", "-f"])
        # Now remove associated chaincode containers if any
        #c.dockerHelper.RemoveContainersWithNamePrefix(c.projectName)
        output, error, returncode = \
            bdd_test_util.cli_call(["docker"] + ["ps", "-qa", "--filter", "name={0}".format(self.projectName)], expect_success=True, env=self.getEnv())
        for containerId in output.splitlines():
            output, error, returncode = \
                bdd_test_util.cli_call(["docker"] + ["rm", "-f", containerId], expect_success=True, env=self.getEnv())

