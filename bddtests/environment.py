import subprocess
import os
import glob
import json, time

from steps import bdd_remote_util
from steps.bdd_test_util import cli_call, bdd_log
from steps.bdd_compose_util import getDockerComposeFileArgsFromYamlFile

from steps.coverage import saveCoverageFiles, createCoverageAggregate

def coverageEnabled(context):
    return context.config.userdata.get("coverage", "false") == "true"


def tlsEnabled(context):
    return context.config.userdata.get("tls", "false") == "true"


def retrieve_logs(context, scenario):
    bdd_log("Scenario {0} failed. Getting container logs".format(scenario.name))
    file_suffix = "_" + scenario.name.replace(" ", "_") + ".log"
    if context.compose_containers == []:
        bdd_log("docker-compose command failed on '{0}'. There are no docker logs.".format(context.compose_yaml))

    # get logs from the peer containers
    for containerData in context.compose_containers:
        with open(containerData.containerName + file_suffix, "w+") as logfile:
            sys_rc = subprocess.call(["docker", "logs", containerData.containerName], stdout=logfile, stderr=logfile)
            if sys_rc !=0 :
                bdd_log("Cannot get logs for {0}. Docker rc = {1}".format(containerData.containerName,sys_rc))

    # get logs from the chaincode containers
    cc_output, cc_error, cc_returncode = \
        cli_call(["docker",  "ps", "-f",  "name=dev-", "--format", "{{.Names}}"], expect_success=True)
    for containerName in cc_output.splitlines():
        namePart,sep,junk = containerName.rpartition("-")
        with open(namePart + file_suffix, "w+") as logfile:
            sys_rc = subprocess.call(["docker", "logs", containerName], stdout=logfile, stderr=logfile)
            if sys_rc !=0 :
                bdd_log("Cannot get logs for {0}. Docker rc = {1}".format(namepart,sys_rc))


def decompose_containers(context, scenario):
    fileArgsToDockerCompose = getDockerComposeFileArgsFromYamlFile(context.compose_yaml)

    bdd_log("Decomposing with yaml '{0}' after scenario {1}, ".format(context.compose_yaml, scenario.name))
    context.compose_output, context.compose_error, context.compose_returncode = \
        cli_call(["docker-compose"] + fileArgsToDockerCompose + ["unpause"], expect_success=True)
    context.compose_output, context.compose_error, context.compose_returncode = \
        cli_call(["docker-compose"] + fileArgsToDockerCompose + ["stop"], expect_success=True)

    if coverageEnabled(context):
        #Save the coverage files for this scenario before removing containers
        containerNames = [containerData.containerName for  containerData in context.compose_containers]
        saveCoverageFiles("coverage", scenario.name.replace(" ", "_"), containerNames, "cov")

    context.compose_output, context.compose_error, context.compose_returncode = \
        cli_call(["docker-compose"] + fileArgsToDockerCompose + ["rm","-f"], expect_success=True)
    # now remove any other containers (chaincodes)
    context.compose_output, context.compose_error, context.compose_returncode = \
        cli_call(["docker",  "ps",  "-qa"], expect_success=True)
    if context.compose_returncode == 0:
        # Remove each container
        for containerId in context.compose_output.splitlines():
            context.compose_output, context.compose_error, context.compose_returncode = \
                cli_call(["docker",  "rm", "-f", containerId], expect_success=True)



def before_scenario(context, scenario):
    context.compose_containers = []
    context.compose_yaml = ""


def after_scenario(context, scenario):
    # Handle logs
    get_logs = context.config.userdata.get("logs", "N")
    if get_logs.lower() == "force" or (scenario.status == "failed" and get_logs.lower() == "y"):
        retrieve_logs(context, scenario)

    # Handle coverage
    if coverageEnabled(context):
        for containerData in context.compose_containers:
            #Save the coverage files for this scenario before removing containers
            containerNames = [containerData.name for  containerData in context.compose_containers]
            saveCoverageFiles("coverage", scenario.name.replace(" ", "_"), containerNames, "cov")

    # Handle decomposition
    if 'doNotDecompose' in scenario.tags and 'compose_yaml' in context:
        bdd_log("Not going to decompose after scenario {0}, with yaml '{1}'".format(scenario.name, context.compose_yaml))
    elif context.byon:
        bdd_log("Stopping a BYON (Bring Your Own Network) setup")
        decompose_remote(context, scenario)
    elif 'compose_yaml' in context:
        decompose_containers(context, scenario)
    else:
        bdd_log("Nothing to stop in this setup")


# stop any running peer that could get in the way before starting the tests
def before_all(context):
    context.byon = os.path.exists("networkcredentials")
    context.remote_ip = context.config.userdata.get("remote-ip", None)
    context.tls = tlsEnabled(context)
    if context.byon:
        context = get_remote_servers(context)
        time.sleep(5)
    else:
        cli_call(["../build/bin/peer", "node", "stop"], expect_success=False)
        #cli_call(["../../build/bin/peer", "node", "stop"], expect_success=False)

# stop any running peer that could get in the way before starting the tests
def after_all(context):
    bdd_log("context.failed = {0}".format(context.failed))

    if coverageEnabled(context):
        createCoverageAggregate()

##########################################
def get_remote_servers(context):
    with open("networkcredentials", "r") as network_file:
        network_creds = json.loads(network_file.read())
        context.remote_servers = [{'ip': peer['host'], 'port': peer['port']} for peer in network_creds['PeerData']]
        context.remote_map = {}
        for peer in network_creds['PeerData']:
            context.remote_map[peer['name']] = {'ip': peer['host'], 'port': peer['port']}
        context.remote_user = network_creds["CA_username"]
        context.remote_secret = network_creds["CA_secret"]
        context.user_creds = network_creds['UserData']
    context.remote_ip = context.config.userdata.get("remote-ip", None)
    return context

def decompose_remote(context, scenario):
    context = get_remote_servers(context)
    headers = {'Content-type': 'application/vnd.ibm.zaci.payload+json;version=1.0',
               'Accept': 'application/vnd.ibm.zaci.payload+json;version=1.0',
               'zACI-API': 'com.ibm.zaci.system/1.0'}
    if context.remote_ip:
        for target in ["vp0", "vp1", "vp2", "vp3"]:
            status = bdd_remote_util.getNodeStatus(context, target).json()
            if status.get(target, "Unknown") != "STARTED":
                status = bdd_remote_util.restartNode(context, target)
                time.sleep(60)
    else:
        command = " export SUDO_ASKPASS=~/.remote_pass.sh;sudo iptables -A INPUT -p tcp --destination-port 30303 -j DROP"
        ssh_call(context, command)


