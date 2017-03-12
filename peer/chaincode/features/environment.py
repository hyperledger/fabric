from behave import *
import os
import re
import subprocess

# set the default step matcher
use_step_matcher("re")

def before_all(context):
    # set some handy values
    context.go_path = os.environ['GOPATH']
    context.fabric_dir = os.path.join(context.go_path, 'src/github.com/hyperledger/fabric')
    context.peer_exe = os.path.join(context.fabric_dir, 'build/bin/peer')
    context.sample_chaincode_path = {
        'golang':'github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02',
        'java':'examples/chaincode/java/SimpleSample'
    }
    context.sample_chaincode_ctor_args = {
        'golang':'{"Args":["init", "a", "100", "b", "200"]}',
        'java':'{"Args": ["init", "a", "100", "b", "200"]}'
    }

def after_scenario(context, scenario):
    # collect logs if failure
    if context.failed:
        open(re.sub('\W+', '_', scenario.name).lower() + '_peer.log', 'w').write(subprocess.check_output(['docker', 'logs', context.peer_container_id], stderr=subprocess.STDOUT))
        open(re.sub('\W+', '_', scenario.name).lower() + '_orderer.log', 'w').write(subprocess.check_output(['docker', 'logs', context.orderer_container_id], stderr=subprocess.STDOUT))
    # teardown docker containers & network
    if getattr(context, 'peer_container_id', None):
        subprocess.check_output(['docker', 'rm', '--force', '--volumes', context.peer_container_id], stderr=subprocess.STDOUT)
    if getattr(context, 'orderer_container_id', None):
        subprocess.check_output(['docker', 'rm', '--force', '--volumes', context.orderer_container_id], stderr=subprocess.STDOUT)
    if getattr(context, 'network_id', None):
        subprocess.check_output(['docker', 'network', 'rm', context.network_id], stderr=subprocess.STDOUT)
