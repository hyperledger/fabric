# Channel create/join chain support

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
Assuming the orderer and peer have been built, and the executables are available in build/bin directory. Switch to build/bin directory.

## Create a channel
### Using CLI in Vagrant environment
_Vagrant window 1 - start orderer_

```
ORDERER_GENERAL_LOGLEVEL=debug ./orderer
```
_Vagrant window 2 - ask orderer to create a chain_

```
peer channel create -c myc1
```

On successful creation, a genesis block myc1.block is saved in build/bin directory.

### Using docker environment
TODO

## Join a channel
### Using CLI in Vagrant environment
_Vagrant window 3 - start the peer in a "chainless" mode_

```
#NOTE - clear the environment with rm -rf /var/hyperledger/* after updating fabric to get channel support.

peer node start --peer-defaultchain=false
```

```
"--peer-defaultchain=true" is the default. It allow users continue to work with the default **TEST_CHAINID** without having to join a chain.

"--peer-defaultchain=false" starts the peer with only the channels that were joined by the peer. If the peer never joined a channel it would start up without any channels. In particular, it does not have the default **TEST_CHAINID** support.

To join channels, a peer MUST be started with the "--peer-defaultchain=false" option.
```
_Vagrant window 2 - peer to join a channel_

```
peer channel join -b myc1.block
```

where myc1.block is the block that was received from the `orderer` from the create channel command.

### Using docker environment
TODO

At this point we can issue transactions.
## Use the channel to deploy and invoke chaincodes
### Using CLI in Vagrant environment
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
### Using docker environment
TODO

To reset, clear out the `fileSystemPath` directory (defined in core.yaml) and myc1.block.
