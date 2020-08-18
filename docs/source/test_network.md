# Using the Fabric test network

After you have downloaded the Hyperledger Fabric Docker images and samples, you
can deploy a test network by using scripts that are provided in the
`fabric-samples` repository. You can use the test network to learn about Fabric
by running nodes on your local machine. More experienced developers can use the
network to test their smart contracts and applications. The network is meant to
be used only as a tool for education and testing. It should not be used as a
template for deploying a production network. The test network is being introduced
in Fabric v2.0 as the long term replacement for the `first-network` sample.

The sample network deploys a Fabric network with Docker Compose. Because the
nodes are isolated within a Docker Compose network, the test network is not
configured to connect to other running fabric nodes.

**Note:** These instructions have been verified to work against the
latest stable Docker images and the pre-compiled setup utilities within the
supplied tar file. If you run these commands with images or tools from the
current master branch, it is possible that you will encounter errors.

## Before you begin

Before you can run the test network, you need to clone the `fabric-samples`
repository and download the Fabric images. Make sure that you have installed
the [Prerequisites](prereqs.html) and [Installed the Samples, Binaries and Docker Images](install.html).

## Bring up the test network

You can find the scripts to bring up the network in the `test-network` directory
of the ``fabric-samples`` repository. Navigate to the test network directory by
using the following command:
```
cd fabric-samples/test-network
```

In this directory, you can find an annotated script, ``network.sh``, that stands
up a Fabric network using the Docker images on your local machine. You can run
``./network.sh -h`` to print the script help text:

```
Usage:
  network.sh <Mode> [Flags]
    Modes:
      up - bring up fabric orderer and peer nodes. No channel is created
      up createChannel - bring up fabric network with one channel
      createChannel - create and join a channel after the network is created
      deployCC - deploy the asset transfer basic chaincode on the channel or specify
      down - clear the network with docker-compose down
      restart - restart the network

    Flags:
    -ca <use CAs> -  create Certificate Authorities to generate the crypto material
    -c <channel name> - channel name to use (defaults to "mychannel")
    -s <dbtype> - the database backend to use: goleveldb (default) or couchdb
    -r <max retry> - CLI times out after certain number of attempts (defaults to 5)
    -d <delay> - delay duration in seconds (defaults to 3)
    -ccn <name> - the short name of the chaincode to deploy: basic (default),ledger, private, secured
    -ccl <language> - the programming language of the chaincode to deploy: go (default), java, javascript, typescript
    -ccv <version>  - chaincode version. 1.0 (default)
    -ccs <sequence>  - chaincode definition sequence. Must be an integer, 1 (default), 2, 3, etc
    -ccp <path>  - Optional, chaincode path. Path to the chaincode. When provided the -ccn will be used as the deployed name and not the short name of the known chaincodes.
    -cci <fcn name>  - Optional, chaincode init required function to invoke. When provided this function will be invoked after deployment of the chaincode and will define the chaincode as initialization required.
    -i <imagetag> - the tag to be used to launch the network (defaults to "latest")
    -cai <ca_imagetag> - the image tag to be used for CA (defaults to "latest")
    -verbose - verbose mode
    -h - print this message

 Possible Mode and flag combinations
   up -ca -c -r -d -s -i -verbose
   up createChannel -ca -c -r -d -s -i -verbose
   createChannel -c -r -d -verbose
   deployCC -ccn -ccl -ccv -ccs -ccp -cci -r -d -verbose

 Taking all defaults:
   network.sh up

 Examples:
   network.sh up createChannel -ca -c mychannel -s couchdb -i 2.0.0
   network.sh createChannel -c channelName
   network.sh deployCC -ccn basic -ccl javascript
```

From inside the `test-network` directory, run the following command to remove
any containers or artifacts from any previous runs:
```
./network.sh down
```

You can then bring up the network by issuing the following command. You will
experience problems if you try to run the script from another directory:
```
./network.sh up
```

This command creates a Fabric network that consists of two peer nodes, one
ordering node. No channel is created when you run `./network.sh up`, though we
will get there in a [future step](#creating-a-channel). If the command completes
successfully, you will see the logs of the nodes being created:
```
Creating network "net_test" with the default driver
Creating volume "net_orderer.example.com" with default driver
Creating volume "net_peer0.org1.example.com" with default driver
Creating volume "net_peer0.org2.example.com" with default driver
Creating orderer.example.com    ... done
Creating peer0.org2.example.com ... done
Creating peer0.org1.example.com ... done
CONTAINER ID        IMAGE                               COMMAND             CREATED             STATUS                  PORTS                              NAMES
8d0c74b9d6af        hyperledger/fabric-orderer:latest   "orderer"           4 seconds ago       Up Less than a second   0.0.0.0:7050->7050/tcp             orderer.example.com
ea1cf82b5b99        hyperledger/fabric-peer:latest      "peer node start"   4 seconds ago       Up Less than a second   0.0.0.0:7051->7051/tcp             peer0.org1.example.com
cd8d9b23cb56        hyperledger/fabric-peer:latest      "peer node start"   4 seconds ago       Up 1 second             7051/tcp, 0.0.0.0:9051->9051/tcp   peer0.org2.example.com
```

If you don't get this result, jump down to [Troubleshooting](#troubleshooting)
for help on what might have gone wrong. By default, the network uses the
cryptogen tool to bring up the network. However, you can also
[bring up the network with Certificate Authorities](#bring-up-the-network-with-certificate-authorities).

### The components of the test network

After your test network is deployed, you can take some time to examine its
components. Run the following command to list all of Docker containers that
are running on your machine. You should see the three nodes that were created by
the `network.sh` script:
```
docker ps -a
```

Each node and user that interacts with a Fabric network needs to belong to an
organization that is a network member. The group of organizations that are
members of a Fabric network are often referred to as the consortium. The test
network has two consortium members, Org1 and Org2. The network also includes one
orderer organization that maintains the ordering service of the network.

[Peers](peers/peers.html) are the fundamental components of any Fabric network.
Peers store the blockchain ledger and validate transactions before they are
committed to the ledger. Peers run the smart contracts that contain the business
logic that is used to manage the assets on the blockchain ledger.

Every peer in the network needs to belong to a member of the consortium. In the
test network, each organization operates one peer each, `peer0.org1.example.com`
and `peer0.org2.example.com`.

Every Fabric network also includes an [ordering service](orderer/ordering_service.html).
While peers validate transactions and add blocks of transactions to the
blockchain ledger, they do not decide on the order of transactions or include
them into new blocks. On a distributed network, peers may be running far away
from each other and not have a common view of when a transaction was created.
Coming to consensus on the order of transactions is a costly process that would
create overhead for the peers.

An ordering service allows peers to focus on validating transactions and
committing them to the ledger. After ordering nodes receive endorsed transactions
from clients, they come to consensus on the order of transactions and then add
them to blocks. The blocks are then distributed to peer nodes, which add the
blocks the blockchain ledger. Ordering nodes also operate the system channel
that defines the capabilities of a Fabric network, such as how blocks are made
and which version of Fabric that nodes can use. The system channel defines which
organizations are members of the consortium.

The sample network uses a single node Raft ordering service that is operated by
the ordering organization. You can see the ordering node running on your machine
as `orderer.example.com`. While the test network only uses a single node ordering
service, a real network would have multiple ordering nodes, operated by one or
multiple orderer organizations. The different ordering nodes would use the Raft
consensus algorithm to come to agreement on the order of transactions across
the network.

## Creating a channel

Now that we have peer and orderer nodes running on our machine, we can use the
script to create a Fabric channel for transactions between Org1 and Org2.
Channels are a private layer of communication between specific network members.
Channels can be used only by organizations that are invited to the channel, and
are invisible to other members of the network. Each channel has a separate
blockchain ledger. Organizations that have been invited "join" their peers to
the channel to store the channel ledger and validate the transactions on the
channel.

You can use the `network.sh` script to create a channel between Org1 and Org2
and join their peers to the channel. Run the following command to create a
channel with the default name of `mychannel`:
```
./network.sh createChannel
```
If the command was successful, you can see the following message printed in your
logs:
```
========= Channel successfully joined ===========
```

You can also use the channel flag to create a channel with custom name. As an
example, the following command would create a channel named `channel1`:
```
./network.sh createChannel -c channel1
```

The channel flag also allows you to create multiple channels by specifying
different channel names. After you create `mychannel` or `channel1`, you can use
the command below to create a second channel named `channel2`:
```
./network.sh createChannel -c channel2
```

If you want to bring up the network and create a channel in a single step, you
can use the `up` and `createChannel` modes together:
```
./network.sh up createChannel
```

## Starting a chaincode on the channel

After you have created a channel, you can start using [smart contracts](smartcontract/smartcontract.html) to
interact with the channel ledger. Smart contracts contain the business logic
that governs assets on the blockchain ledger. Applications run by members of the
network can invoke smart contracts to create assets on the ledger, as well as
change and transfer those assets. Applications also query smart contracts to
read data on the ledger.

To ensure that transactions are valid, transactions created using smart contracts
typically need to be signed by multiple organizations to be committed to the
channel ledger. Multiple signatures are integral to the trust model of Fabric.
Requiring multiple endorsements for a transaction prevents one organization on
a channel from tampering with the ledger on their peer or using business logic
that was not agreed to. To sign a transaction, each organization needs to invoke
and execute the smart contract on their peer, which then signs the output of the
transaction. If the output is consistent and has been signed by enough
organizations, the transaction can be committed to the ledger. The policy that
specifies the set organizations on the channel that need to execute the smart
contract is referred to as the endorsement policy, which is set for each
chaincode as part of the chaincode definition.

In Fabric, smart contracts are deployed on the network in packages referred to
as chaincode. A Chaincode is installed on the peers of an organization and then
deployed to a channel, where it can then be used to endorse transactions and
interact with the blockchain ledger. Before a chaincode can be deployed to a
channel, the members of the channel need to agree on a chaincode definition that
establishes chaincode governance. When the required number of organizations
agree, the chaincode definition can be committed to the channel, and the
chaincode is ready to be used.

After you have used the `network.sh` to create a channel, you can start a
chaincode on the channel using the following command:
```
./network.sh deployCC
```
The `deployCC` subcommand will install the **asset-transfer (basic)** chaincode on
``peer0.org1.example.com`` and ``peer0.org2.example.com`` and then deploy
the chaincode on the channel specified using the channel flag (or `mychannel`
if no channel is specified).  If you are deploying a chaincode for the first
time, the script will install the chaincode dependencies. By default, The script
installs the Go version of the asset-transfer (basic) chaincode. However, you can use the
language flag, `-l`, to install the typescript or javascript versions of the chaincode.
You can find the asset-transfer (basic) chaincode in the `asset-transfer-basic` folder of the `fabric-samples`
directory. This folder contains sample chaincode that are provided as examples and
used by tutorials to highlight Fabric features.

## Interacting with the network

After you bring up the test network, you can use the `peer` CLI to interact
with your network. The `peer` CLI allows you to invoke deployed smart contracts,
update channels, or install and deploy new smart contracts from the CLI.

Make sure that you are operating from the `test-network` directory. If you
followed the instructions to [install the Samples, Binaries and Docker Images](install.html),
You can find the `peer` binaries in the `bin` folder of the `fabric-samples`
repository. Use the following command to add those binaries to your CLI Path:
```
export PATH=${PWD}/../bin:$PATH
```
You also need to set the `FABRIC_CFG_PATH` to point to the `core.yaml` file in
the `fabric-samples` repository:
```
export FABRIC_CFG_PATH=$PWD/../config/
```
You can now set the environment variables that allow you to operate the `peer`
 CLI as Org1:
```
# Environment variables for Org1

export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="Org1MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_ADDRESS=localhost:7051
```

The `CORE_PEER_TLS_ROOTCERT_FILE` and `CORE_PEER_MSPCONFIGPATH` environment
variables point to the Org1 crypto material in the `organizations` folder.

If you used `./network.sh deployCC` to install and start the asset-transfer (basic) chaincode, you can invoke the `InitLedger` function of the (Go) chaincode to put an initial list of assets on the ledger (if using typescript or javascript `./network.sh deployCC -l javascript` for example, you will invoke the `initLedger` function of the respective chaincodes).

Run the following command to initialize the ledger with assets:
```
peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n basic --peerAddresses localhost:7051 --tlsRootCertFiles ${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses localhost:9051 --tlsRootCertFiles ${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt -c '{"function":"InitLedger","Args":[]}'
```
If successful, you should see similar output to below:
```
-> INFO 001 Chaincode invoke successful. result: status:200
```
You can now query the ledger from your CLI. Run the following command to get the list of assets that were added to your channel ledger:
```
peer chaincode query -C mychannel -n basic -c '{"Args":["GetAllAssets"]}'
```
If successful, you should see the following output:
```
[
  {"ID": "asset1", "color": "blue", "size": 5, "owner": "Tomoko", "appraisedValue": 300},
  {"ID": "asset2", "color": "red", "size": 5, "owner": "Brad", "appraisedValue": 400},
  {"ID": "asset3", "color": "green", "size": 10, "owner": "Jin Soo", "appraisedValue": 500},
  {"ID": "asset4", "color": "yellow", "size": 10, "owner": "Max", "appraisedValue": 600},
  {"ID": "asset5", "color": "black", "size": 15, "owner": "Adriana", "appraisedValue": 700},
  {"ID": "asset6", "color": "white", "size": 15, "owner": "Michel", "appraisedValue": 800}
]
```
Chaincodes are invoked when a network member wants to transfer or change an asset on the ledger. Use the following command to change the owner of an asset on the ledger by invoking the asset-transfer (basic) chaincode:
```
peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n basic --peerAddresses localhost:7051 --tlsRootCertFiles ${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses localhost:9051 --tlsRootCertFiles ${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt -c '{"function":"TransferAsset","Args":["asset6","Christopher"]}'
```

If the command is successful, you should see the following response:
```
2019-12-04 17:38:21.048 EST [chaincodeCmd] chaincodeInvokeOrQuery -> INFO 001 Chaincode invoke successful. result: status:200
```
Because the endorsement policy for the asset-transfer (basic) chaincode requires the transaction
to be signed by Org1 and Org2, the chaincode invoke command needs to target both
`peer0.org1.example.com` and `peer0.org2.example.com` using the `--peerAddresses`
flag. Because TLS is enabled for the network, the command also needs to reference
the TLS certificate for each peer using the `--tlsRootCertFiles` flag.

After we invoke the chaincode, we can use another query to see how the invoke
changed the assets on the blockchain ledger. Since we already queried the Org1
peer, we can take this opportunity to query the chaincode running on the Org2
peer. Set the following environment variables to operate as Org2:
```
# Environment variables for Org2

export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="Org2MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
export CORE_PEER_ADDRESS=localhost:9051
```

You can now query the asset-transfer (basic) chaincode running on `peer0.org2.example.com`:
```
peer chaincode query -C mychannel -n basic -c '{"Args":["ReadAsset","asset6"]}'
```

The result will show that `"asset6"` was transferred to Christopher:
```
{"ID":"asset6","color":"white","size":15,"owner":"Christopher","appraisedValue":800}
```

## Bring down the network

When you are finished using the test network, you can bring down the network
with the following command:
```
./network.sh down
```

The command will stop and remove the node and chaincode containers, delete the
organization crypto material, and remove the chaincode images from your Docker
Registry. The command also removes the channel artifacts and docker volumes from
previous runs, allowing you to run `./network.sh up` again if you encountered
any problems.

## Next steps

Now that you have used the test network to deploy Hyperledger Fabric on your
local machine, you can use the tutorials to start developing your own solution:

- Learn how to deploy your own smart contracts to the test network using the
[Deploying a smart contract to a channel](deploy_chaincode.html) tutorial.
- Visit the [Writing Your First Application](write_first_app.html) tutorial
to learn how to use the APIs provided by the Fabric SDKs to invoke smart
contracts from your client applications.
- If you are ready to deploy a more complicated smart contract to the network, follow
the [commercial paper tutorial](tutorial/commercial_paper.html) to explore a
use case in which two organizations use a blockchain network to trade commercial
paper.

You can find the complete list of Fabric tutorials on the [tutorials](tutorials.html)
page.

## Bring up the network with Certificate Authorities

Hyperledger Fabric uses public key infrastructure (PKI) to verify the actions of
all network participants. Every node, network administrator, and user submitting
transactions needs to have a public certificate and private key to verify their
identity. These identities need to have a valid root of trust, establishing
that the certificates were issued by an organization that is a member of the
network. The `network.sh` script creates all of the cryptographic material
that is required to deploy and operate the network before it creates the peer
and ordering nodes.

By default, the script uses the cryptogen tool to create the certificates
and keys. The tool is provided for development and testing, and can quickly
create the required crypto material for Fabric organizations with a valid root
of trust. When you run `./network.sh up`, you can see the cryptogen tool creating
the certificates and keys for Org1, Org2, and the Orderer Org.

```
creating Org1, Org2, and ordering service organization with crypto from 'cryptogen'

/Usr/fabric-samples/test-network/../bin/cryptogen

##########################################################
##### Generate certificates using cryptogen tool #########
##########################################################

##########################################################
############ Create Org1 Identities ######################
##########################################################
+ cryptogen generate --config=./organizations/cryptogen/crypto-config-org1.yaml --output=organizations
org1.example.com
+ res=0
+ set +x
##########################################################
############ Create Org2 Identities ######################
##########################################################
+ cryptogen generate --config=./organizations/cryptogen/crypto-config-org2.yaml --output=organizations
org2.example.com
+ res=0
+ set +x
##########################################################
############ Create Orderer Org Identities ###############
##########################################################
+ cryptogen generate --config=./organizations/cryptogen/crypto-config-orderer.yaml --output=organizations
+ res=0
+ set +x
```

However, the test network script also provides the option to bring up the network using
Certificate Authorities (CAs). In a production network, each organization
operates a CA (or multiple intermediate CAs) that creates the identities that
belong to their organization. All of the identities created by a CA run by the
organization share the same root of trust. Although it takes more time than
using cryptogen, bringing up the test network using CAs provides an introduction
to how a network is deployed in production. Deploying CAs also allows you to enroll
client identities with the Fabric SDKs and create a certificate and private key
for your applications.

If you would like to bring up a network using Fabric CAs, first run the following
command to bring down any running networks:
```
./network.sh down
```

You can then bring up the network with the CA flag:
```
./network.sh up -ca
```

After you issue the command, you can see the script bringing up three CAs, one
for each organization in the network.
```
##########################################################
##### Generate certificates using Fabric CA's ############
##########################################################
Creating network "net_default" with the default driver
Creating ca_org2    ... done
Creating ca_org1    ... done
Creating ca_orderer ... done
```

It is worth taking time to examine the logs generated by the `./network.sh`
script after the CAs have been deployed. The test network uses the Fabric CA
client to register node and user identities with the CA of each organization. The
script then uses the enroll command to generate an MSP folder for each identity.
The MSP folder contains the certificate and private key for each identity, and
establishes the identity's role and membership in the organization that operated
the CA. You can use the following command to examine the MSP folder of the Org1
admin user:
```
tree organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/
```
The command will reveal the MSP folder structure and configuration file:
```
organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/
└── msp
    ├── IssuerPublicKey
    ├── IssuerRevocationPublicKey
    ├── cacerts
    │   └── localhost-7054-ca-org1.pem
    ├── config.yaml
    ├── keystore
    │   └── 58e81e6f1ee8930df46841bf88c22a08ae53c1332319854608539ee78ed2fd65_sk
    ├── signcerts
    │   └── cert.pem
    └── user
```
You can find the certificate of the admin user in the `signcerts` folder and the
private key in the `keystore` folder. To learn more about MSPs, see the [Membership Service Provider](membership/membership.html)
concept topic.

Both cryptogen and the Fabric CAs generate the cryptographic material for each organization
in the `organizations` folder. You can find the commands that are used to set up the
network in the `registerEnroll.sh` script in the `organizations/fabric-ca` directory.
To learn more about how you would use the Fabric CA to deploy a Fabric network,
visit the [Fabric CA operations guide](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html).
You can learn more about how Fabric uses PKI by visiting the [identity](identity/identity.html)
and [membership](membership/membership.html) concept topics.

## What's happening behind the scenes?

If you are interested in learning more about the sample network, you can
investigate the files and scripts in the `test-network` directory. The steps
below provide a guided tour of what happens when you issue the command of
`./network.sh up`.

- `./network.sh` creates the certificates and keys for two peer organizations
  and the orderer organization. By default, the script uses the cryptogen tool
  using the configuration files located in the `organizations/cryptogen` folder.
  If you use the `-ca` flag to create Certificate Authorities, the script uses
  Fabric CA server configuration files and `registerEnroll.sh` script located in
  the `organizations/fabric-ca` folder. Both cryptogen and the Fabric CAs create
  the crypto material and MSP folders for all three organizations in the
  `organizations` folder.

- The script uses configtxgen tool to create the system channel genesis block.
  Configtxgen consumes the `TwoOrgsOrdererGenesis`  channel profile in the
  `configtx/configtx.yaml` file to create the genesis block. The block is stored
  in the `system-genesis-block` folder.

- Once the organization crypto material and the system channel genesis block have
  been generated, the `network.sh` can bring up the nodes of the network. The
  script uses the ``docker-compose-test-net.yaml`` file in the `docker` folder
  to create the peer and orderer nodes. The `docker` folder also contains the
  ``docker-compose-e2e.yaml`` file that brings up the nodes of the network
  alongside three Fabric CAs. This file is meant to be used to run end-to-end
  tests by the Fabric SDK. Refer to the [Node SDK](https://github.com/hyperledger/fabric-sdk-node)
  repo for details on running these tests.

- If you use the `createChannel` subcommand, `./network.sh` runs the
  `createChannel.sh` script in the `scripts` folder to create a channel
  using the supplied channel name. The script uses the `configtx.yaml` file to
  create the channel creation transaction, as well as two anchor peer update
  transactions. The script uses the peer cli to create the channel, join
  ``peer0.org1.example.com`` and ``peer0.org2.example.com`` to the channel, and
  make both of the peers anchor peers.

- If you issue the `deployCC` command, `./network.sh` runs the ``deployCC.sh``
  script to install the **asset-transfer (basic)** chaincode on both peers and then define then
  chaincode on the channel. Once the chaincode definition is committed to the
  channel, the peer cli initializes the chaincode using the `Init` and invokes
  the chaincode to put initial data on the ledger.

## Troubleshooting

If you have any problems with the tutorial, review the following:

-  You should always start your network fresh. You can use the following command
   to remove the artifacts, crypto material, containers, volumes, and chaincode
   images from previous runs:
   ```
   ./network.sh down
   ```
   You **will** see errors if you do not remove old containers, images, and
   volumes.

-  If you see Docker errors, first check your Docker version ([Prerequisites](prereqs.html)),
   and then try restarting your Docker process. Problems with Docker are
   oftentimes not immediately recognizable. For example, you may see errors
   that are the result of your node not being able to access the crypto material
   mounted within a container.

   If problems persist, you can remove your images and start from scratch:
   ```
   docker rm -f $(docker ps -aq)
   docker rmi -f $(docker images -q)
   ```

-  If you see errors on your create, approve, commit, invoke or query commands,
   make sure you have properly updated the channel name and chaincode name.
   There are placeholder values in the supplied sample commands.

-  If you see the error below:
   ```
   Error: Error endorsing chaincode: rpc error: code = 2 desc = Error installing chaincode code mycc:1.0(chaincode /var/hyperledger/production/chaincodes/mycc.1.0 exits)
   ```

   You likely have chaincode images (e.g. ``dev-peer1.org2.example.com-asset-transfer-1.0`` or
   ``dev-peer0.org1.example.com-asset-transfer-1.0``) from prior runs. Remove them and try
   again.
   ```
   docker rmi -f $(docker images | grep dev-peer[0-9] | awk '{print $3}')
   ```

-  If you see the below error:

   ```
   [configtx/tool/localconfig] Load -> CRIT 002 Error reading configuration: Unsupported Config Type ""
   panic: Error reading configuration: Unsupported Config Type ""
   ```

   Then you did not set the ``FABRIC_CFG_PATH`` environment variable properly. The
   configtxgen tool needs this variable in order to locate the configtx.yaml. Go
   back and execute an ``export FABRIC_CFG_PATH=$PWD/configtx/configtx.yaml``,
   then recreate your channel artifacts.

-  If you see an error stating that you still have "active endpoints", then prune
   your Docker networks. This will wipe your previous networks and start you with a
   fresh environment:
   ```
   docker network prune
   ```

   You will see the following message:
   ```
   WARNING! This will remove all networks not used by at least one container.
   Are you sure you want to continue? [y/N]
   ```
   Select ``y``.

-  If you see an error similar to the following:
   ```
   /bin/bash: ./scripts/createChannel.sh: /bin/bash^M: bad interpreter: No such file or directory
   ```

   Ensure that the file in question (**createChannel.sh** in this example) is
   encoded in the Unix format. This was most likely caused by not setting
   ``core.autocrlf`` to ``false`` in your Git configuration (see
    [Windows extras](prereqs.html#windows-extras)). There are several ways of fixing this. If you have
   access to the vim editor for instance, open the file:
   ```
   vim ./fabric-samples/test-network/scripts/createChannel.sh
   ```

   Then change its format by executing the following vim command:
   ```
   :set ff=unix
   ```

- If your orderer exits upon creation or if you see that the create channel
  command fails due to an inability to connect to your ordering service, use
  the `docker logs` command to read the logs from the ordering node. You may see
  the following message:
  ```
  PANI 007 [channel system-channel] config requires unsupported orderer capabilities: Orderer capability V2_0 is required but not supported: Orderer capability V2_0 is required but not supported
  ```
  This occurs when you are trying to run the network using Fabric version 1.4.x
  docker images. The test network needs to run using Fabric version 2.x.

If you continue to see errors, share your logs on the **fabric-questions**
channel on [Hyperledger Rocket Chat](https://chat.hyperledger.org/home) or on
[StackOverflow](https://stackoverflow.com/questions/tagged/hyperledger-fabric).

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
