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

import subprocess
import shutil


def getDockerComposeFileArgsFromYamlFile(composeYaml):
    parts = composeYaml.split()
    args = []
    for part in parts:
        args = args + ["-f"] + [part]
    return args


def getLogFiles(containers, fileSuffix):
    """ This will gather the logs for the different component containers as well as
        the chaincode containers. If the containers is a list of strings, it is
        assumed this is a chaincode container list. Otherwise, the list is a list
        of Container objects.
    """
    for container in containers:
        if isinstance(container, str):
            namePart, sep, _ = container.rpartition("-")
            containerName = container
        else:
            namePart = container.containerName
            containerName = container.containerName
        with open(namePart + fileSuffix, "w+") as logfile:
            rc = subprocess.call(["docker", "logs", containerName], stdout=logfile, stderr=logfile)
            if rc !=0 :
                print("Cannot get logs for {0}. Docker rc = {1}".format(namePart, rc))


def after_scenario(context, scenario):
    getLogs = context.config.userdata.get("logs", "N")
    if getLogs.lower() == "force" or (scenario.status == "failed" and getLogs.lower() == "y" and "compose_containers" in context):
        print("Scenario {0} failed. Getting container logs".format(scenario.name))
        fileSuffix = "_" + scenario.name.replace(" ", "_") + ".log"
        # get logs from the peer containers
        getLogFiles(containers, fileSuffix)
        # get logs from the chaincode containers
        chaincodeContainers = subprocess.call(["docker",  "ps", "-f",  "name=dev-", "--format", "{{.Names}}"])
        getLogFiles(chaincodeContainers.splitlines(), fileSuffix)

    if 'doNotDecompose' in scenario.tags:
        if 'compose_yaml' in context:
            print("Not going to decompose after scenario {0}, with yaml '{1}'".format(scenario.name, context.compose_yaml))
    elif 'composition' in context:
        # Remove config data and docker containers
        shutil.rmtree("configs/%s" % context.composition.projectName)
        context.composition.decompose()

# stop any running peer that could get in the way before starting the tests
def before_all(context):
    pass

# stop any running peer that could get in the way before starting the tests
def after_all(context):
    print("context.failed = {0}".format(context.failed))
