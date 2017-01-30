# Getting Started with v1.0 Hyperledger Fabric - App Developers
This document demonstrates an example using the Hyperledger Fabric V1.0 architecture.
The scenario will include the creation and joining of channels, client side authentication,
and the deployment and invocation of chaincode.  CLI will be used for the creation and
joining of the channel and the node SDK will be used for the client authentication,
and chaincode functions utilizing the channel.

Docker-compose will be used to create a consortium of three organizations, each
running an endorsing/committing peer, as well as a "solo" orderer and a Certificate Authority (CA).
The cryptographic material, based on standard PKI implementation, has been pre-generated
and is included in the sfhackfest.tar.gz in order to expedite the flow.  The CA, responsible for
issuing, revoking and maintaining the crypto material represents one of the organizations and
is needed by the client (node SDK) for authentication.  In an enterprise scenario, each
organization might have their own CA, with more complex security measures implemented - e.g.
cross-signing certificates, etc.

The network will be generated automatically upon execution of `docker-compose up`,
and the APIs for create channel and join channel will be explained and demonstrated;
as such, a user can go through the steps to manually generate their own network
and channel, or quickly jump to the application development phase.

## Prerequisites and setup

* [Docker](https://www.docker.com/products/overview) - v1.12 or higher
* [Docker Compose](https://docs.docker.com/compose/overview/) - v1.8 or higher
* [Node.js](https://nodejs.org/en/download/) - comes with the node package manager (npm).
If you already have npm on your machine, issue the following command to retrieve the latest package:
```bash
npm install npm@latest
```
then execute the following to see your version:
```bash
npm -v
```
You're looking for a version higher than 2.1.8.

## Curl the source code to create network entities

* Download the [cURL](https://curl.haxx.se/download.html) tool if not already installed.
* Determine a location on your local machine where you want to place the Fabric and application source.
```bash
mkdir -p <my_dev_workspace>/hackfest
cd <my_dev_workspace>/hackfest
```
Next, execute the following command:
```bash
curl -L https://raw.githubusercontent.com/hyperledger/fabric/master/examples/sfhackfest/sfhackfest.tar.gz -o sfhackfest.tar.gz 2> /dev/null;  tar -xvf sfhackfest.tar.gz
```
This command pulls and extracts all of the necessary artifacts to set up your network - docker compose script,
channel generate/join script, crypto material for identity attestation, etc.  In the /src/github.com/example_cc directory you
will find the chaincode that will be deployed.  

Your directory should contain the following:
```bash
JDoe-mbp: JohnDoe$ pwd
/Users/JohnDoe
JDoe-mbp: JohnDoe$ ls
sfhackfest.tar.gz   channel_test.sh	  src
ccenv		docker-compose-gettingstarted.yml	  tmp
```

## Using Docker

You do not need to manually pull any images.  The images for - `fabric-peer`,
`fabric-orderer`, `fabric-ca`, and `cli` are specified in the .yml file and will
automatically download, extract, and run when you execute the `docker-compose` commands.

## Commands

The channel commands are:
* `create` - create and name a channel in the `orderer` and get back a genesis
block for the channel.  The genesis block is named in accordance with the channel name.
* `join` - use the genesis block from the `create` command to issue a join request to a Peer.

## Use Docker to spawn network entities & create/join a channel

Ensure the hyperledger/fabric-ccenv image is tagged as latest:
```bash
docker-compose -f docker-compose-gettingstarted.yml build
```
Create network entities, create channel, join peers to channel:
```bash
docker-compose -f docker-compose-gettingstarted.yml up -d
```
Behind the scenes this started six containers (3 peers, "solo" orderer, CLI and CA)
in detached mode.  A script - channel_test.sh - embedded within the
docker-compose-gettingstarted.yml issued the create channel and join channel commands within the
CLI container.  In the end, you are left with a network and a channel containing three
peers - peer0, peer1, peer2.

View your containers:
```bash
# if you have no other containers running, you will see nine
docker ps
```

Ensure the channel has been created and peers have successfully joined:
```bash
docker exec -it cli sh
```
You should see the following in your terminal:
```bash
/opt/gopath/src/github.com/hyperledger/fabric/peer #
```
To view results for channel creation/join:
```bash
vi results.txt
```
To view logs:
```bash
vi log.txt
```
To view genesis block details:
```bash
vi myc1.block
```

## Curl the application source code and SDK modules

* Prior to issuing the command, make sure you are in the same working directory where you curled the network code.  
* Execute the following command:
```bash
curl -OOOOOO https://raw.githubusercontent.com/hyperledger/fabric-sdk-node/master/examples/balance-transfer/{config.json,deploy.js,helper.js,invoke.js,query.js,package.json}
```

This command pulls the javascript code for issuing your deploy, invoke and query calls.  It also
retrieves dependencies for the node SDK modules.  

* Install the node modules:
```bash
npm install
```
You may be prompted for your root password at one or more times during the `npm install`.
At this point you have installed all of the prerequisites and source code.

## Use node SDK to register/enroll user and deploy/invoke/query

The individual javascript programs will exercise the SDK APIs to register and enroll the client with
the provisioned Certificate Authority.  Once the client is properly authenticated,
the programs will demonstrate basic chaincode functionalities - deploy, invoke, and query.  Make
sure you are in the working directory where you pulled the source code.  You can explore the individual
javascript programs to better understand the various APIs.

Register and enroll the user & deploy chaincode:
```bash
GOPATH=$PWD node deploy.js
```
_if running on Windows_:
```bash
SET GOPATH=%cd%
node deploy.js
```
Issue an invoke. Move units from "a" to "b":
```bash
node invoke.js
```
Query against key value "a":
```bash
node query.js
```
You will receive a "200 response" in your terminal if each command is successful.

## Manually create and join channel (optional)

To manually exercise the createChannel and joinChannel APIs through the CLI container, you will
need to edit the Docker Compose file.  Use an editor to open docker-compose-gettingstarted.yml and
comment out the `channel_test.sh` command in your cli image.  Simply place a `#` to the left
of the command.  For example:
```bash
cli:
  container_name: cli
  <CONTENT REMOVED FOR BREVITY>
  working_dir: /opt/gopath/src/github.com/hyperledger/fabric/peer
#  command: sh -c './channel_test.sh; sleep 1000'
#  command: /bin/sh
```
Exec into the cli container:
```bash
docker exec -it cli sh
```
If successful, you should see the following in your terminal:
```bash
/opt/gopath/src/github.com/hyperledger/fabric/peer #
```
Send createChannel API to Ordering Service:
```
CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 peer channel create -c myc1
```
This will return a genesis block - myc1.block - that you can issue join commands with.
Next, send a joinChannel API to peer0 and pass in the genesis block as an argument.
The channel is defined within the genesis block:
```
CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 CORE_PEER_ADDRESS=peer0:7051 peer channel join -b myc1.block
```
To join the other peers to the channel, simply reissue the above command with peer1
or peer2 specified.  For example:
```
CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 CORE_PEER_ADDRESS=peer1:7051 peer channel join -b myc1.block
```
Once the peers have all joined the channel, you are able to issues queries against
any peer without having to deploy chaincode to each of them.

## Use cli to deploy, invoke and query (optional)

Run the deploy command.  This command is deploying a chaincode named `mycc` to
`peer0` on the Channel ID `myc1`.  The constructor message is initializing `a` and
`b` with values of 100 and 200 respectively.
```
CORE_PEER_ADDRESS=peer0:7051 CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 peer chaincode deploy -C myc1 -n mycc -p github.com/hyperledger/fabric/examples -c '{"Args":["init","a","100","b","200"]}'
```
Run the invoke command.  This invocation is moving 10 units from `a` to `b`.
```
CORE_PEER_ADDRESS=peer0:7051 CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 peer chaincode invoke -C myc1 -n mycc -c '{"function":"invoke","Args":["move","a","b","10"]}'
```
Run the query command.  The invocation transferred 10 units from `a` to `b`, therefore
a query against `a` should return the value 90.
```
CORE_PEER_ADDRESS=peer0:7051 CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 peer chaincode query -C myc1 -n mycc -c '{"function":"invoke","Args":["query","a"]}'
```
You can issue an `exit` command at any time to exit the cli container.

## Troubleshooting (optional)

If you have existing containers running you may receive an error indicating that a port is
already occupied.  If this occurs, you will need to kill the container that is using said port.

If a file cannot be located, make sure your curl commands executed successfully and make
sure you are in the directory where you pulled the source code.

Remove a specific docker container:
```bash
docker rm <containerID>
```
Force removal:
```bash
docker rm -f <containerID>
```
Remove all docker containers:
```bash
docker rm -f $(docker ps -aq)
```
This will merely kill docker containers (i.e. stop the process).  You will not lose any images.

Remove an image:
```bash
docker rmi <imageID>
```
Forcibly remove:
```bash
docker rmi -f <imageID>
```
Remove all images:
```bash
docker rmi -f $(docker images -q)
```
