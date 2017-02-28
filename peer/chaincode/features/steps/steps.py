from behave import *
import os
import random
import subprocess
import tempfile
import time
import socket

@step(u'a fabric peer and orderer')
def step_impl(context):

    # create network
    context.network_name = 'behave_' + ''.join(random.choice('0123456789') for i in xrange(7))
    context.network_id = subprocess.check_output([
        'docker', 'network', 'create', context.network_name
    ]).strip()

    # start orderer
    context.orderer_container_id = subprocess.check_output([
        'docker', 'run', '-d', '-p', '7050',
        '--expose', '7050',
        '--network', context.network_name,
        '--network-alias', 'orderer',
        '--env', 'ORDERER_GENERAL_LISTENADDRESS=0.0.0.0',
        'hyperledger/fabric-orderer'
    ]).strip()
    context.orderer_address = subprocess.check_output(['docker', 'port', context.orderer_container_id, '7050']).strip()

    # start peer
    context.peer_container_id = subprocess.check_output([
        'docker', 'run', '-d', '-p', '7051',
        '--network', context.network_name,
        '--network-alias', 'vp0',
        '--env', 'CORE_PEER_ADDRESSAUTODETECT=true',
        '--env', 'CORE_PEER_ID=vp0',
        '--env', 'CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050',
        '--env', 'CORE_CHAINCODE_STARTUPTIMEOUT=5000',
        '--volume', '/var/run/docker.sock:/var/run/docker.sock',
        'hyperledger/fabric-peer',
        'peer', 'node', 'start', '--logging-level', 'debug'
    ]).strip()
    context.peer_address = subprocess.check_output(['docker', 'port', context.peer_container_id, '7051']).strip()
    time.sleep(1)

    # setup env for peer cli commands
    context.peer_env = os.environ.copy()
    context.peer_env['CORE_PEER_ADDRESS'] = context.peer_address
    context.peer_env['CORE_PEER_COMMITTER_LEDGER_ORDERER'] = context.orderer_address

@step(r'a (?P<lang>java|go|golang|car) chaincode is installed via the CLI')
def step_impl(context, lang):
    context.chaincode_lang = 'golang' if lang == 'go' else lang
    context.chaincode_id_name = lang + '_cc_' + ''.join(random.choice('0123456789') for i in xrange(7))
    context.chaincode_id_version = '1.0.0.0'
    try:
        print(subprocess.check_output([
            context.peer_exe, 'chaincode', 'install',
            '--logging-level', 'debug',
            '--name', context.chaincode_id_name,
            '--path', context.sample_chaincode_path[context.chaincode_lang],
            '--version', context.chaincode_id_version,
            '--lang', context.chaincode_lang
        ], cwd=context.fabric_dir, stderr=subprocess.STDOUT, env=context.peer_env))
    except subprocess.CalledProcessError as e:
        print(e.output)
        print('CORE_PEER_ADDRESS = ' + context.peer_env['CORE_PEER_ADDRESS'])
        print('CORE_PEER_COMMITTER_LEDGER_ORDERER = ' + context.peer_env['CORE_PEER_COMMITTER_LEDGER_ORDERER'])
        raise

@step(u'the chaincode is installed on the peer')
def step_impl(context):
    print(subprocess.check_output([
        'docker', 'exec', context.peer_container_id, 'ls', '-l', '/var/hyperledger/production/chaincodes/' + context.chaincode_id_name + '.' + context.chaincode_id_version
    ]))

@step(r'version (?P<version>\S+) of a (?P<lang>java|go|golang|car) chaincode is installed via the CLI')
def step_impl(context, version, lang):
    context.chaincode_lang = 'golang' if lang == 'go' else lang
    context.chaincode_id_name = lang + '_cc_' + ''.join(random.choice('0123456789') for i in xrange(7))
    context.chaincode_id_version = version
    try:
        print(subprocess.check_output([
            context.peer_exe, 'chaincode', 'install',
            '--logging-level', 'debug',
            '--name', context.chaincode_id_name,
            '--path', context.sample_chaincode_path[context.chaincode_lang],
            '--version', context.chaincode_id_version,
            '--lang', context.chaincode_lang
        ], cwd=context.fabric_dir, stderr=subprocess.STDOUT, env=context.peer_env))
    except subprocess.CalledProcessError as e:
        print(e.output)
        raise

@step(r'installing version (?P<version>\S+) of the same chaincode via the CLI will fail')
def step_impl(context, version):
    assert getattr(context, 'chaincode_id_name', None), 'No chaincode previously installed.'
    context.chaincode_id_version = version
    try:
        print(subprocess.check_output([
            context.peer_exe, 'chaincode', 'install',
            '--logging-level', 'debug',
            '--name', context.chaincode_id_name,
            '--path', context.sample_chaincode_path[context.chaincode_lang],
            '--version', context.chaincode_id_version,
            '--lang', context.chaincode_lang
        ], cwd=context.fabric_dir, stderr=subprocess.STDOUT, env=context.peer_env))
    except subprocess.CalledProcessError as e:
        print(e.output)
        raise

@step(u'the chaincode can be instantiated via the CLI')
def step_impl(context):
    assert getattr(context, 'chaincode_id_name', None), 'No chaincode previously installed.'
    try:
        print(subprocess.check_output([
            context.peer_exe, 'chaincode', 'instantiate',
            '--logging-level', 'debug',
            '--name', context.chaincode_id_name,
            '--path', context.sample_chaincode_path[context.chaincode_lang],
            '--version', context.chaincode_id_version,
            '--lang', context.chaincode_lang,
            '--ctor', context.sample_chaincode_ctor_args[context.chaincode_lang]
        ], cwd=context.fabric_dir, stderr=subprocess.STDOUT, env=context.peer_env))
    except subprocess.CalledProcessError as e:
        print(e.output)
        raise
