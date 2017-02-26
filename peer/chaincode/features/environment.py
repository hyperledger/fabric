from behave import *
import os
import subprocess

# set the default step matcher
use_step_matcher("re")

def before_all(context):
    # set some handy values
    context.go_path = os.environ['GOPATH']
    context.fabric_dir = os.path.join(context.go_path, 'src/github.com/hyperledger/fabric')
    context.peer_exe = os.path.join(context.fabric_dir, 'build/bin/peer')
    context.sample_chaincode = {
        'golang':'github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02',
        'java':'examples/chaincode/java/SimpleSample'
    }

def after_scenario(context, scenario):
    if getattr(context, 'peer_container_id', None):
        subprocess.check_output(['docker', 'rm', '--force', '--volumes', context.peer_container_id], stderr=subprocess.STDOUT)
    if getattr(context, 'orderer_container_id', None):
        subprocess.check_output(['docker', 'rm', '--force', '--volumes', context.orderer_container_id], stderr=subprocess.STDOUT)
    if getattr(context, 'network_id', None):
        subprocess.check_output(['docker', 'network', 'rm', context.network_id], stderr=subprocess.STDOUT)
