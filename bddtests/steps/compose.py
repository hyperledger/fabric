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
from contexthelper import ContextHelper
import json

from abc import ABCMeta, abstractmethod

class ContainerData:
    def __init__(self, containerName, ipAddress, envFromInspect, composeService, ports):
        self.containerName = containerName
        self.ipAddress = ipAddress
        self.envFromInspect = envFromInspect
        self.composeService = composeService
        self.ports = ports

    def getEnv(self, key):
        envValue = None
        for val in self.envFromInspect:
            if val.startswith(key):
                envValue = val[len(key):]
                break
        if envValue == None:
            raise Exception("ENV key not found ({0}) for container ({1})".format(key, self.containerName))
        return envValue


class CompositionCallback:
    __metaclass__ = ABCMeta
    @abstractmethod
    def composing(self, composition, context):
        pass
    @abstractmethod
    def decomposing(self, composition, context):
        pass
    @abstractmethod
    def getEnv(self, composition, context, env):
        pass

class Test(CompositionCallback):
    def composing(self, composition, context):
        pass
    def decomposing(self, composition, context):
        pass
    def getEnv(self, composition, context, env):
        pass

def GetDockerSafeUUID():
    return str(uuid.uuid1()).replace('-','')

class Composition:

    @classmethod
    def RegisterCallbackInContext(cls, context, callback):
        if not isinstance(callback, CompositionCallback):
            raise TypeError("Expected type to be {0}, instead received {1}".format(CompositionCallback, type(callback)))
        Composition.GetCompositionCallbacksFromContext(context).append(callback)

    @classmethod
    def GetCompositionCallbacksFromContext(cls, context):
        if not "compositionCallbacks" in context:
            context.compositionCallbacks = []
        return context.compositionCallbacks


    @classmethod
    def GetUUID(cls):
        return GetDockerSafeUUID()

    def __init__(self, context, composeFilesYaml, projectName=None,
                 force_recreate=True, components=[], register_and_up=True):
        self.contextHelper = ContextHelper.GetHelper(context=context)
        if not projectName:
            projectName = self.contextHelper.getGuuid()
        self.projectName = projectName
        self.context = context
        self.containerDataList = []
        self.composeFilesYaml = composeFilesYaml
        self.serviceNames = []
        self.serviceNames = self._collectServiceNames()
        if register_and_up:
            # Register with contextHelper (Supports docgen)
            self.contextHelper.registerComposition(self)
            [callback.composing(self, context) for callback in Composition.GetCompositionCallbacksFromContext(context)]
            self.up(context, force_recreate, components)

    def _collectServiceNames(self):
        'First collect the services names.'
        servicesList = [service for service in self.issueCommand(["config", "--services"]).splitlines() if "WARNING" not in service]
        return servicesList

    def up(self, context, force_recreate=True, components=[]):
        self.serviceNames = self._collectServiceNames()
        command = ["up", "-d"]
        if force_recreate:
            command += ["--force-recreate"]
        self.issueCommand(command + components)

    def scale(self, context, serviceName, count=1):
        self.serviceNames = self._collectServiceNames()
        command = ["scale", "%s=%d" %(serviceName, count)]
        self.issueCommand(command)

    def stop(self, context, components=[]):
        self.serviceNames = self._collectServiceNames()
        command = ["stop"]
        self.issueCommand(command, components)

    def start(self, context, components=[]):
        self.serviceNames = self._collectServiceNames()
        command = ["start"]
        self.issueCommand(command, components)

    def getServiceNames(self):
        return list(self.serviceNames)

    def parseComposeFilesArg(self, composeFileArgs):
        args = [arg for sublist in [["-f", file] for file in [file if not os.path.isdir(file) else os.path.join(file, 'docker-compose.yml') for file in composeFileArgs.split()]] for arg in sublist]
        return args

    def getFileArgs(self):
        return self.parseComposeFilesArg(self.composeFilesYaml)

    def getEnvAdditions(self):
        myEnv = {}
        myEnv["COMPOSE_PROJECT_NAME"] = self.projectName
        myEnv["CORE_PEER_NETWORKID"] = self.projectName
        # Invoke callbacks
        [callback.getEnv(self, self.context, myEnv) for callback in Composition.GetCompositionCallbacksFromContext(self.context)]
        return myEnv

    def getEnv(self):
        myEnv = os.environ.copy()
        for key,value in self.getEnvAdditions().iteritems():
            myEnv[key] = value
        # myEnv["COMPOSE_PROJECT_NAME"] = self.projectName
        # myEnv["CORE_PEER_NETWORKID"] = self.projectName
        # # Invoke callbacks
        # [callback.getEnv(self, self.context, myEnv) for callback in Composition.GetCompositionCallbacksFromContext(self.context)]
        return myEnv

    def getConfig(self):
        return self.issueCommand(["config"])

    def refreshContainerIDs(self):
        containers = self.issueCommand(["ps", "-q"]).split()
        return containers

    def _callCLI(self, argList, expect_success, env):
        return bdd_test_util.cli_call(argList, expect_success=expect_success, env=env)

    def issueCommand(self, command, components=[]):
        componentList = []
        useCompose = True
        for component in components:
            if '_' in component:
                useCompose = False
                componentList.append("%s_%s" % (self.projectName, component))
            else:
                break

        # If we need to perform an operation on a specific container, use
        # docker not docker-compose
        if useCompose:
            cmdArgs = self.getFileArgs()+ command + components
            cmd = ["docker-compose"] + cmdArgs
        else:
            cmdArgs = command + componentList
            cmd = ["docker"] + cmdArgs

        #print("cmd:", cmd)
        output, error, returncode = \
            self._callCLI(cmd, expect_success=True, env=self.getEnv())

        # Don't rebuild if ps command
        if command[0] !="ps" and command[0] !="config":
            self.rebuildContainerData()
        return output

    def rebuildContainerData(self):
        self.containerDataList[:] = []
        for containerID in self.refreshContainerIDs():

            # get container metadata
            container = json.loads(bdd_test_util.cli_call(["docker", "inspect", containerID], expect_success=True)[0])[0]

            # container name
            container_name = container['Name'][1:]

            # container ip address (only if container is running)
            container_ipaddress = None
            if container['State']['Running']:
                container_ipaddress = container['NetworkSettings']['IPAddress']
                if not container_ipaddress:
                    # ipaddress not found at the old location, try the new location
                    container_ipaddress = container['NetworkSettings']['Networks'].values()[0]['IPAddress']

            # container environment
            container_env = container['Config']['Env']

            # container exposed ports
            container_ports = container['NetworkSettings']['Ports']

            # container docker-compose service
            container_compose_service = container['Config']['Labels']['com.docker.compose.service']

            self.containerDataList.append(ContainerData(container_name, container_ipaddress, container_env, container_compose_service, container_ports))

    def decompose(self):
        self.issueCommand(["unpause"])
        self.issueCommand(["down"])
        self.issueCommand(["kill"])
        self.issueCommand(["rm", "-f"])

        # Now remove associated chaincode containers if any
        output, error, returncode = \
            bdd_test_util.cli_call(["docker"] + ["ps", "-qa", "--filter", "name={0}".format(self.projectName)], expect_success=True, env=self.getEnv())
        for containerId in output.splitlines():
            output, error, returncode = \
                bdd_test_util.cli_call(["docker"] + ["rm", "-f", containerId], expect_success=True, env=self.getEnv())

        # Remove the associated network
        output, error, returncode = \
            bdd_test_util.cli_call(["docker"] + ["network", "ls", "-q", "--filter", "name={0}".format(self.projectName)], expect_success=True, env=self.getEnv())
        for networkId in output.splitlines():
            output, error, returncode = \
                bdd_test_util.cli_call(["docker"] + ["network", "rm", networkId], expect_success=True, env=self.getEnv())

        # Invoke callbacks
        [callback.decomposing(self, self.context) for callback in Composition.GetCompositionCallbacksFromContext(self.context)]
