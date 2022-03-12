# Running chaincode in development mode

**Audience:** Smart contract developers that want to iteratively develop and test their chaincode packages without the overhead of the smart contract lifecycle process for every update.

During smart contract development, a developer needs a way to quickly and iteratively test a chaincode package without having to run the chaincode lifecycle commands for every modification. This tutorial uses the Fabric binaries and starts the peer in development mode ("DevMode") and then connects the chaincode to the peer. It allows you to start a chaincode without ever installing the chaincode on the peer and after the chaincode is initially committed to a channel, you can bypass the peer lifecycle chaincode commands.  This allows for rapid deployment, debugging, and update without the overhead of reissuing the peer lifecycle chaincode commands every time you make an update.

**Note:** In order to use the DevMode flag on a peer, TLS communications must be disabled on all the nodes in the network. And because TLS communications are strongly recommended for production networks, you should never run a production peer in DevMode. The network used in this tutorial should not be used as a template for any form of a production network. See [Deploying a production network](deployment_guide_overview.html) for instructions on how to deploy a production network. You can also refer to the [Fabric test network](test_network.html) to learn more about how to deploy and update a smart contract package on a channel using the Fabric chaincode lifecycle process.


Throughout this tutorial, all commands are performed from the `fabric/` folder. It uses all the default settings for the peer and orderer and overrides the configurations using environment variables from the command line as needed. No edits are required to the default peer `core.yaml` or orderer `orderer.yaml` files.

## Set up environment

1. Clone the Fabric repository from [GitHub](https://github.com/hyperledger/fabric). Select the release branch according to your needs.
2. Run the following commands to build the binaries for orderer, peer, and configtxgen:
    ```
    make orderer peer configtxgen
    ```
    When successful you should see results similar to:
    ```
    Building build/bin/orderer
    GOBIN=/testDevMode/fabric/build/bin go install -tags "" -ldflags "-X github.com/hyperledger/fabric/common/metadata.Version=2.3.0 -X github.com/hyperledger/fabric/common/metadata.CommitSHA=298695ae2 -X github.com/hyperledger/fabric/common/metadata.BaseDockerLabel=org.hyperledger.fabric -X github.com/hyperledger/fabric/common/metadata.DockerNamespace=hyperledger" github.com/hyperledger/fabric/cmd/orderer
    Building build/bin/peer
    GOBIN=/testDevMode/fabric/build/bin go install -tags "" -ldflags "-X github.com/hyperledger/fabric/common/metadata.Version=2.3.0 -X github.com/hyperledger/fabric/common/metadata.CommitSHA=298695ae2 -X github.com/hyperledger/fabric/common/metadata.BaseDockerLabel=org.hyperledger.fabric -X github.com/hyperledger/fabric/common/metadata.DockerNamespace=hyperledger" github.com/hyperledger/fabric/cmd/peer
    Building build/bin/configtxgen
    GOBIN=/testDevMode/fabric/build/bin go install -tags "" -ldflags "-X github.com/hyperledger/fabric/common/metadata.Version=2.3.0 -X github.com/hyperledger/fabric/common/metadata.CommitSHA=298695ae2 -X github.com/hyperledger/fabric/common/metadata.BaseDockerLabel=org.hyperledger.fabric -X github.com/hyperledger/fabric/common/metadata.DockerNamespace=hyperledger" github.com/hyperledger/fabric/cmd/configtxgen
    ```
3. Set the `PATH` environment variable to include orderer and peer binaries:
    ```
    export PATH=$(pwd)/build/bin:$PATH
    ```
4. Set the `FABRIC_CFG_PATH` environment variable to point to the `sampleconfig` folder:
    ```
    export FABRIC_CFG_PATH=$(pwd)/sampleconfig
    ```
5. Create the `hyperledger` subdirectory in the `/var` directory. This is the default location Fabric uses to store blocks as defined in the orderer `orderer.yaml` and peer `core.yaml` files. To create the `hyperledger` subdirectory, execute these commands, replacing the question marks with your username:

    ```
    sudo mkdir /var/hyperledger
    sudo chown ????? /var/hyperledger
    ```
6. Generate the genesis block for the ordering service. Run the following command to generate the genesis block and store it in `$(pwd)/sampleconfig/genesisblock` so that it can be used by the orderer in the next step when the orderer is started.
    ```
    configtxgen -profile SampleDevModeSolo -channelID syschannel -outputBlock genesisblock -configPath $FABRIC_CFG_PATH -outputBlock "$(pwd)/sampleconfig/genesisblock"
    ```

    When successful you should see results similar to:
    ```
    2020-09-14 17:36:37.295 EDT [common.tools.configtxgen] doOutputBlock -> INFO 004 Generating genesis block
    2020-09-14 17:36:37.296 EDT [common.tools.configtxgen] doOutputBlock -> INFO 005 Writing genesis block
    ```
## Start the orderer

Run the following command to start the orderer with the `SampleDevModeSolo` profile and start the ordering service:

```
ORDERER_GENERAL_GENESISPROFILE=SampleDevModeSolo orderer
```
When it is successful you should see results similar to:
```
2020-09-14 17:37:20.258 EDT [orderer.common.server] Main -> INFO 00b Starting orderer:
 Version: 2.3.0
 Commit SHA: 298695ae2
 Go version: go1.15
 OS/Arch: darwin/amd64
2020-09-14 17:37:20.258 EDT [orderer.common.server] Main -> INFO 00c Beginning to serve requests
```

## Start the peer in DevMode

Open another terminal window and set the required environment variables to override the peer configuration and start the peer node.

**Note:** If you intend to keep the orderer and peer in the same environment (not in separate containers), only then set the `CORE_OPERATIONS_LISTENADDRESS` environment variable (port can be anything except 9443).
```
export CORE_OPERATIONS_LISTENADDRESS=127.0.0.1:9444
```

Starting the peer with the `--peer-chaincodedev=true` flag puts the peer into DevMode.
```
export PATH=$(pwd)/build/bin:$PATH
export FABRIC_CFG_PATH=$(pwd)/sampleconfig
FABRIC_LOGGING_SPEC=chaincode=debug CORE_PEER_CHAINCODELISTENADDRESS=0.0.0.0:7052 peer node start --peer-chaincodedev=true
```

**Reminder:** When running in `DevMode`, TLS cannot be enabled.

When it is successful you should see results similar to:
```
2020-09-14 17:38:45.324 EDT [nodeCmd] serve -> INFO 00e Running in chaincode development mode
...
2020-09-14 17:38:45.326 EDT [nodeCmd] serve -> INFO 01a Started peer with ID=[jdoe], network ID=[dev], address=[192.168.1.134:7051]
```

## Create channel and join peer

Open another terminal window and run the following commands to generate the channel creation transaction using the `configtxgen` tool. This command creates the channel `ch1` with the `SampleSingleMSPChannel` profile:

```
export PATH=$(pwd)/build/bin:$PATH
export FABRIC_CFG_PATH=$(pwd)/sampleconfig
configtxgen -channelID ch1 -outputCreateChannelTx ch1.tx -profile SampleSingleMSPChannel -configPath $FABRIC_CFG_PATH
peer channel create -o 127.0.0.1:7050 -c ch1 -f ch1.tx
```

When it is successful you should see results similar to:
```
2020-09-14 17:42:56.931 EDT [cli.common] readBlock -> INFO 002 Received block: 0
```

Now join the peer to the channel by running the following command:

```
peer channel join -b ch1.block
```
When it is successful, you should see results similar to:
```
2020-09-14 17:43:34.976 EDT [channelCmd] executeJoin -> INFO 002 Successfully submitted proposal to join channel
```

The peer has now joined channel `ch1`.

## Build the chaincode

We use the **simple** chaincode from the `fabric/integration/chaincode` directory to demonstrate how to run a chaincode package in DevMode. In the same terminal window as the previous step, run the following command to build the chaincode:

```
go build -o simpleChaincode ./integration/chaincode/simple/cmd
```

## Start the chaincode

When `DevMode` is enabled on the peer, the `CORE_CHAINCODE_ID_NAME` environment variable must be set to `<CHAINCODE_NAME>`:`<CHAINCODE_VERSION>` otherwise, the peer is unable to find the chaincode.  For this example, we set it to `mycc:1.0`. Run the following command to start the chaincode and connect it to the peer:

```
CORE_CHAINCODE_LOGLEVEL=debug CORE_PEER_TLS_ENABLED=false CORE_CHAINCODE_ID_NAME=mycc:1.0 ./simpleChaincode -peer.address 127.0.0.1:7052
```

Because we set debug logging on the peer when we started it, you can confirm that the chaincode registration is successful. In your peer logs, you should see results similar to:
```
2020-09-14 17:53:43.413 EDT [chaincode] sendReady -> DEBU 045 Changed to state ready for chaincode mycc:1.0
```
## Approve and commit the chaincode definition

Now you need to run the following Fabric chaincode lifecycle commands to approve and commit the chaincode definition to the channel:

```
peer lifecycle chaincode approveformyorg  -o 127.0.0.1:7050 --channelID ch1 --name mycc --version 1.0 --sequence 1 --init-required --signature-policy "OR ('SampleOrg.member')" --package-id mycc:1.0
peer lifecycle chaincode checkcommitreadiness -o 127.0.0.1:7050 --channelID ch1 --name mycc --version 1.0 --sequence 1 --init-required --signature-policy "OR ('SampleOrg.member')"
peer lifecycle chaincode commit -o 127.0.0.1:7050 --channelID ch1 --name mycc --version 1.0 --sequence 1 --init-required --signature-policy "OR ('SampleOrg.member')" --peerAddresses 127.0.0.1:7051
```

You should see results similar to:
```
2020-09-14 17:56:30.820 EDT [chaincodeCmd] ClientWait -> INFO 001 txid [f22b3c25dfea7fe0b28af9ee818056db81e29a9421c83fe00eb22fa41d1d1e21] committed with status (VALID) at
Chaincode definition for chaincode 'mycc', version '1.0', sequence '1' on channel 'ch1' approval status by org:
SampleOrg: true
2020-09-14 17:57:43.295 EDT [chaincodeCmd] ClientWait -> INFO 001 txid [fb803e8b0b4eae6b3a9ed35668f223753e1a34ffd2a7042f9e5bb516a383eb32] committed with status (VALID) at 127.0.0.1:7051
```

## Next steps

You can issue CLI commands to invoke and query the chaincode as needed to verify your smart contract logic. For this example, we issue three commands. The first one initializes the smart contract, the second command moves `10` from asset `a` to asset `b`. And the final command queries the value of `a` to verify it was successfully changed from `100` to `90`.

```
CORE_PEER_ADDRESS=127.0.0.1:7051 peer chaincode invoke -o 127.0.0.1:7050 -C ch1 -n mycc -c '{"Args":["init","a","100","b","200"]}' --isInit
CORE_PEER_ADDRESS=127.0.0.1:7051 peer chaincode invoke -o 127.0.0.1:7050 -C ch1 -n mycc -c '{"Args":["invoke","a","b","10"]}'
CORE_PEER_ADDRESS=127.0.0.1:7051 peer chaincode invoke -o 127.0.0.1:7050 -C ch1 -n mycc -c '{"Args":["query","a"]}'
```

You should see results similar to:
```
2020-09-14 18:15:00.034 EDT [chaincodeCmd] chaincodeInvokeOrQuery -> INFO 001 Chaincode invoke successful. result: status:200
2020-09-14 18:16:29.704 EDT [chaincodeCmd] chaincodeInvokeOrQuery -> INFO 001 Chaincode invoke successful. result: status:200
2020-09-14 18:17:42.101 EDT [chaincodeCmd] chaincodeInvokeOrQuery -> INFO 001 Chaincode invoke successful. result: status:200 payload:"90"
```

The benefit of running the peer in DevMode is that you can now iteratively make updates to your smart contract, save your changes, [build](#build-the-chaincode) the chaincode, and then [start](#start-the-chaincode) it again using the steps above. You do not need to run the peer lifecycle commands to update the chaincode every time you make a change.
