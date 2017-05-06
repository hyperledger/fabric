# Copyright IBM Corp. 2017 All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

import subprocess

from steps.docgen import DocumentGenerator
from steps.bdd_test_util import cli_call
from steps.contexthelper import ContextHelper
from steps.coverage import saveCoverageFiles, createCoverageAggregate
from steps.bootstrap_util import getDirectory

def coverageEnabled(context):
    return context.config.userdata.get("coverage", "false") == "true"


def getDockerComposeFileArgsFromYamlFile(compose_yaml):
    parts = compose_yaml.split()
    args = []
    for part in parts:
        args = args + ["-f"] + [part]
    return args

def before_step(context, step):
    contextHelper = ContextHelper.GetHelper(context=context)
    contextHelper.before_step(step)

def after_step(context, step):
    contextHelper = ContextHelper.GetHelper(context=context)
    contextHelper.after_step(step)


def before_scenario(context, scenario):
    contextHelper = ContextHelper.GetHelper(context=context)
    contextHelper.before_scenario(scenario)
    if 'generateDocs' in scenario.tags:
        DocumentGenerator(contextHelper=contextHelper, scenario=scenario)


def after_scenario(context, scenario):
    contextHelper = ContextHelper.GetHelper(context=context)
    contextHelper.after_scenario(scenario)

    get_logs = context.config.userdata.get("logs", "N")
    if "compose_containers" in context:
        if get_logs.lower() == "force" or (scenario.status == "failed" and get_logs.lower() == "y"):
            print("Scenario {0} failed. Getting container logs".format(scenario.name))
            file_suffix = "_" + scenario.name.replace(" ", "_") + ".log"
            # get logs from the peer containers
            for containerData in context.compose_containers:
                (fileName, fileExists) = contextHelper.getTmpPathForName(name="{0}_{1}".format(containerData.containerName, scenario.name), extension="log", path_relative_to_tmp="logs")
                with open(fileName, "w+") as logfile:
                    sys_rc = subprocess.call(["docker", "logs", containerData.containerName], stdout=logfile, stderr=logfile)
                    if sys_rc !=0 :
                        print("Cannot get logs for {0}. Docker rc = {1}".format(containerData.containerName,sys_rc))
            # get logs from the chaincode containers
            cc_output, cc_error, cc_returncode = \
                cli_call(["docker",  "ps", "-f",  "name=dev-", "--format", "{{.Names}}"], expect_success=True)
            for containerName in cc_output.splitlines():
                namePart,sep,junk = containerName.rpartition("-")
                (fileName, fileExists) = contextHelper.getTmpPathForName(name="{0}_{1}".format(namePart, scenario.name), extension="log", path_relative_to_tmp="logs")
                with open(fileName, "w+") as logfile:
                    sys_rc = subprocess.call(["docker", "logs", containerName], stdout=logfile, stderr=logfile)
                    if sys_rc !=0 :
                        print("Cannot get logs for {0}. Docker rc = {1}".format(namepart,sys_rc))
    if 'doNotDecompose' in scenario.tags:
        if 'compose_yaml' in context:
            print("Not going to decompose after scenario {0}, with yaml '{1}'".format(scenario.name, context.compose_yaml))
    elif 'composition' in context:
        if coverageEnabled(context):
            # First stop the containers to allow for coverage files to be created.
            context.composition.issueCommand(["stop"])
            #Save the coverage files for this scenario before removing containers
            containerNames = [containerData.containerName for  containerData in context.compose_containers]
            saveCoverageFiles("coverage", scenario.name.replace(" ", "_"), containerNames, "cov")
        context.composition.decompose()
    # Ask the directory to cleanup
    getDirectory(context).cleanup()

# stop any running peer that could get in the way before starting the tests
def before_all(context):
    pass

# stop any running peer that could get in the way before starting the tests
def after_all(context):
    print("context.failed = {0}".format(context.failed))

    if coverageEnabled(context):
        createCoverageAggregate()
