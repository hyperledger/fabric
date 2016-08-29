import subprocess
import os
import glob

from steps.bdd_test_util import cli_call, bdd_log
from steps.bdd_compose_util import getDockerComposeFileArgsFromYamlFile

from steps.coverage import saveCoverageFiles, createCoverageAggregate

def coverageEnabled(context):
    return context.config.userdata.get("coverage", "false") == "true"

def after_scenario(context, scenario):
    get_logs = context.config.userdata.get("logs", "N")
    if get_logs.lower() == "force" or (scenario.status == "failed" and get_logs.lower() == "y" and "compose_containers" in context):
        bdd_log("Scenario {0} failed. Getting container logs".format(scenario.name))
        file_suffix = "_" + scenario.name.replace(" ", "_") + ".log"
        # get logs from the peer containers
        for containerData in context.compose_containers:
            with open(containerData.name + file_suffix, "w+") as logfile:
                sys_rc = subprocess.call(["docker", "logs", containerData.name], stdout=logfile, stderr=logfile)
                if sys_rc !=0 :
                    bdd_log("Cannot get logs for {0}. Docker rc = {1}".format(containerData.name,sys_rc))
        # get logs from the chaincode containers
        cc_output, cc_error, cc_returncode = \
            cli_call(["docker",  "ps", "-f",  "name=dev-", "--format", "{{.Names}}"], expect_success=True)
        for containerName in cc_output.splitlines():
            namePart,sep,junk = containerName.rpartition("-")
            with open(namePart + file_suffix, "w+") as logfile:
                sys_rc = subprocess.call(["docker", "logs", containerName], stdout=logfile, stderr=logfile)
                if sys_rc !=0 :
                    bdd_log("Cannot get logs for {0}. Docker rc = {1}".format(namepart,sys_rc))
    if 'doNotDecompose' in scenario.tags:
        if 'compose_yaml' in context:
            bdd_log("Not going to decompose after scenario {0}, with yaml '{1}'".format(scenario.name, context.compose_yaml))
    else:
        if 'compose_yaml' in context:
            fileArgsToDockerCompose = getDockerComposeFileArgsFromYamlFile(context.compose_yaml)

            bdd_log("Decomposing with yaml '{0}' after scenario {1}, ".format(context.compose_yaml, scenario.name))
            context.compose_output, context.compose_error, context.compose_returncode = \
                cli_call(["docker-compose"] + fileArgsToDockerCompose + ["unpause"], expect_success=True)
            context.compose_output, context.compose_error, context.compose_returncode = \
                cli_call(["docker-compose"] + fileArgsToDockerCompose + ["stop"], expect_success=True)

            if coverageEnabled(context):
                #Save the coverage files for this scenario before removing containers
                containerNames = [containerData.name for  containerData in context.compose_containers]
                saveCoverageFiles("coverage", scenario.name.replace(" ", "_"), containerNames, "cov")

            context.compose_output, context.compose_error, context.compose_returncode = \
                cli_call(["docker-compose"] + fileArgsToDockerCompose + ["rm","-f"], expect_success=True)
            # now remove any other containers (chaincodes)
            context.compose_output, context.compose_error, context.compose_returncode = \
                cli_call(["docker",  "ps",  "-qa"], expect_success=True)
            if context.compose_returncode == 0:
                # Remove each container
                for containerId in context.compose_output.splitlines():
                    #bdd_log("docker rm {0}".format(containerId))
                    context.compose_output, context.compose_error, context.compose_returncode = \
                        cli_call(["docker",  "rm", "-f", containerId], expect_success=True)

# stop any running peer that could get in the way before starting the tests
def before_all(context):
        cli_call(["../build/bin/peer", "node", "stop"], expect_success=False)

# stop any running peer that could get in the way before starting the tests
def after_all(context):
    bdd_log("context.failed = {0}".format(context.failed))

    if coverageEnabled(context):
        createCoverageAggregate()
