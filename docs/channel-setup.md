# Multichannel Setup

This document describe the CLI for creating channels and directing peers to join channels. The CLI uses channel APIs that are also available in the SDK.

The channel commands are
* create - create a channel in the `orderer` and get back a genesis block for the channel
* join - use genesis block from create command to issue a join request to a Peer

```
NOTE - The main JIRA items for the work are
   https://jira.hyperledger.org/browse/FAB-1022
   https://jira.hyperledger.org/browse/FAB-1547

The commands are work in progress. In particular, there will be more configuration parameters to the commands. Some relevant JIRA items
    https://jira.hyperledger.org/browse/FAB-1642
    https://jira.hyperledger.org/browse/FAB-1639
    https://jira.hyperledger.org/browse/FAB-1580
```

## Using docker
Pull the latest images from https://github.com/rameshthoomu/

### Create a channel
Copy [`docker-compose-channel.yml`](docker-compose-channel.yml) to your current directory.

_Bring up peer and orderer_
```
cd docs
docker-compose -f docker-compose-channel.yml up
```

`docker ps` should show containers `orderer` and `peer0` running.

_Ask orderer to create a channel_
Start the CLI container.
```
docker-compose -f docker-compose-channel.yml run cli
```
In the above shell execute the create command
```
CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:5005 peer channel create -c myc1
```

This will create a channel genesis block file `myc1.block` to issue join commands with.
If you want to specify anchor peers, you can create anchor peer files in the following format:
peer-hostname
port
PEM file of peer certificate

See CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:5005 peer channel create -h for an anchor-peer file example
And pass the anchor peer files as a comma-separated argument with flag -a: in example:
```
CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:5005 peer channel create -c myc1 -a anchorPeer1.txt,anchorPeer2.txt
```

### Join a channel
Execute the join command to peer0 in the CLI container.

```
CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:5005 CORE_PEER_ADDRESS=peer0:7051 peer channel join -b myc1.block
```

### Use the channel to deploy and invoke chaincodes
Run the deploy command
```
CORE_PEER_ADDRESS=peer0:7051 CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:5005 peer chaincode deploy -C myc1 -n mycc -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -c '{"Args":["init","a","100","b","200"]}'
```

Run the invoke command
```
CORE_PEER_ADDRESS=peer0:7051 CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:5005 peer chaincode invoke -C myc1 -n mycc -c '{"Args":["invoke","a","b","10"]}'
```

Run the query command
```
CORE_PEER_ADDRESS=peer0:7051 CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:5005 peer chaincode query -C myc1 -n mycc -c '{"Args":["query","a"]}'
```

## Using Vagrant
Build the executables with `make orderer` and `make peer` commands. Switch to build/bin directory.

### Create a channel
_Vagrant window 1 - start orderer_

```
ORDERER_GENERAL_LOGLEVEL=debug ./orderer
```
_Vagrant window 2 - ask orderer to create a chain_

```
peer channel create -c myc1
```

On successful creation, a genesis block myc1.block is saved in build/bin directory.

### Join a channel
_Vagrant window 3 - start the peer in a "chainless" mode_

```
#NOTE - clear the environment with rm -rf /var/hyperledger/* after updating fabric to get channel support.

peer node start --peer-defaultchain=false
```

```
"--peer-defaultchain=true" is the default. It allow users continue to work with the default "testchainid" without having to join a chain.

"--peer-defaultchain=false" starts the peer with only the channels that were joined by the peer. If the peer never joined a channel it would start up without any channels. In particular, it does not have the default "testchainid" support.

To join channels, a peer MUST be started with the "--peer-defaultchain=false" option.
```
_Vagrant window 2 - peer to join a channel_

```
peer channel join -b myc1.block
```

where myc1.block is the block that was received from the `orderer` from the create channel command.

At this point we can issue transactions.
### Use the channel to deploy and invoke chaincodes
_Vagrant window 2 - deploy a chaincode to myc1_

```
peer chaincode deploy -C myc1 -n mycc -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -c '{"Args":["init","a","100","b","200"]}'
```

Note the use of `-C myc1` to target the chaincode deployment against the `myc1` channel.

Wait for the deploy to get committed (e.g., by default the `solo orderer` can take upto 10 seconds to sends a batch of transactions to be committed.)

_Vagrant window 2 - invoke chaincode_

```
peer chaincode invoke -C myc1 -n mycc -c '{"Args":["invoke","a","b","10"]}'
```

Wait for upto 10 seconds for the invoke to get committed.

_Vagrant window 2 - query chaincode_

```
peer chaincode query -C myc1 -n mycc -c '{"Args":["query","a"]}'
```

To reset, clear out the `fileSystemPath` directory (defined in core.yaml) and myc1.block.
