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

import os
import sys
import subprocess
import json
import uuid


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


class Composition:

    def __init__(self, context, composeFilesYaml, projectName = None,
                 force_recreate = True, components = [], startContainers=True):
        if not projectName:
            projectName = str(uuid.uuid1()).replace('-','')
        self.projectName = projectName
        self.context = context
        self.containerDataList = []
        self.composeFilesYaml = composeFilesYaml
        if startContainers:
            self.up(force_recreate, components)

    def collectServiceNames(self):
        'First collect the services names.'
        servicesList = [service for service in self.issueCommand(["config", "--services"]).splitlines() if "WARNING" not in service]
        return servicesList

    def up(self, force_recreate=True, components=[]):
        command = ["up", "-d"]
        if force_recreate:
            command += ["--force-recreate"]
        self.issueCommand(command + components)

    def scale(self, serviceName, count=1):
        command = ["scale", "%s=%d" %(serviceName, count)]
        self.issueCommand(command)

    def stop(self, components=[]):
        command = ["stop"]
        self.issueCommand(command, components)

    def start(self, components=[]):
        self.serviceNames = self.collectServiceNames()
        command = ["start"]
        self.issueCommand(command, components)

    def docker_exec(self, command, components=[]):
        results = {}
        updatedCommand = " ".join(command)
        for component in components:
            execCommand = ["exec", component, updatedCommand]
            results[component] = self.issueCommand(execCommand, [])
        return results

    def parseComposeFilesArg(self, composeFileArgs):
        composeFileList = []
        for composeFile in composeFileArgs.split():
            if not os.path.isdir(composeFile):
                composeFileList.append(composeFile)
            else:
                composeFileList.append(os.path.join(composeFile, 'docker-compose.yml'))

        argSubList = [["-f", composeFile] for composeFile in composeFileList]
        args = [arg for sublist in argSubList for arg in sublist]
        return args

    def getFileArgs(self):
        return self.parseComposeFilesArg(self.composeFilesYaml)

    def getEnvAdditions(self):
        myEnv = {}
        myEnv["COMPOSE_PROJECT_NAME"] = self.projectName
        myEnv["CORE_PEER_NETWORKID"] = self.projectName
        return myEnv

    def getEnv(self):
        myEnv = os.environ.copy()
        for key,value in self.getEnvAdditions().items():
            myEnv[key] = value
        return myEnv

    def refreshContainerIDs(self):
        containers = self.issueCommand(["ps", "-q"]).split()
        return containers

    def getContainerIP(self, container):
        container_ipaddress = None
        if container['State']['Running']:
            container_ipaddress = container['NetworkSettings']['IPAddress']
            if not container_ipaddress:
                # ipaddress not found at the old location, try the new location
                container_ipaddress = container['NetworkSettings']['Networks'].values()[0]['IPAddress']
        return container_ipaddress

    def getContainerFromName(self, containerName, containerList):
        container = None
        for container in containerList:
            if containerName == container.containerName:
                break
        return container

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
        if useCompose and command[0] != "exec":
            cmdArgs = self.getFileArgs()+ command + components
            cmd = ["docker-compose"] + cmdArgs
        elif command[0] == "exec":
            cmdArgs = command + componentList
            cmdList = ["docker"] + cmdArgs
            cmd = [" ".join(cmdList)]
        else:
            cmdArgs = command + componentList
            cmd = ["docker"] + cmdArgs

        try:
            if cmd[0].startswith("docker exec"):
                process = subprocess.Popen(cmd, shell=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=self.getEnv())
                output, _error = process.communicate()
                if "Error: " in _error or "CRIT " in _error:
                    raise Exception(_error)
            else:
                process = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=self.getEnv())
                output, _error = process.communicate()
                if _error:
                    raise Exception(_error)
        except:
            err = "Error occurred {0}: {1}".format(cmd, sys.exc_info()[1])
            output = err

        # Don't rebuild if ps command
        if command[0] !="ps" and command[0] !="config":
            self.rebuildContainerData()
        return str(output)

    def rebuildContainerData(self):
        self.containerDataList = []
        for containerID in self.refreshContainerIDs():
            # get container metadata
            container = json.loads(str(subprocess.check_output(["docker", "inspect", containerID])))[0]
            # container name
            container_name = container['Name'][1:]
            # container ip address (only if container is running)
            container_ipaddress = self.getContainerIP(container)
            # container environment
            container_env = container['Config']['Env']
            # container exposed ports
            container_ports = container['NetworkSettings']['Ports']
            # container docker-compose service
            container_compose_service = container['Config']['Labels']['com.docker.compose.service']
            container_data = ContainerData(container_name,
                                           container_ipaddress,
                                           container_env,
                                           container_compose_service,
                                           container_ports)
            self.containerDataList.append(container_data)

    def decompose(self):
        self.issueCommand(["unpause"])
        self.issueCommand(["down"])
        self.issueCommand(["kill"])
        self.issueCommand(["rm", "-f"])
        env = self.getEnv()

        # Now remove associated chaincode containers if any
        cmd = ["docker", "ps", "-qa", "--filter", "name={0}".format(self.projectName)]
        output = str(subprocess.check_output(cmd, env=env))
        container_list = output.strip().split('\n')
        for container in container_list:
            if container != '':
                subprocess.call(['docker', 'rm', '-f', container], env=env)

        # Need to remove the chaincode images: docker rmi -f $(docker images | grep "example.com-" | awk '{print $3}')
        cmd = ['docker images | grep ".example.com-" | awk \'{print $3}\' | xargs docker rmi']
        subprocess.call(cmd, shell=True, env=env)
