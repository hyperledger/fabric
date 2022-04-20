# Running External Chaincode Builders

Fabric v2.4.1 external chaincode builders provide a practical approach to running smart contracts by enabling the peer to run external (to itself) commands to manage chaincode. By comparison, the earlier [deploying a smart contract to a channel](deploy_chaincode.html) method required the peer to orchestrate the complete lifecycle of the chaincode. This required the peer to have access to the Docker Daemon to create images and to start containers. Java, Node.js and Go chaincode frameworks were explicitly known to the peer, including how they should be built and started.

As a result, the traditional chaincode deployment method made it challenging to deploy chaincode into Kubernetes (K8s), or other environments where access to the Docker Daemon is restricted, and to run chaincode in any form of debug mode. Additionally, the code was usually rebuilt by the peer, requiring an external internet connection and introducing some uncertainty about which dependencies had been installed.

The chaincode as a service method does require an administrator to orchestrate the chaincode build and deployment phase. Although this creates an additional step, it provides administrators with control over the process. The peer still requires a 'chaincode package' to be installed, but with no code - only information about where the chaincode is hosted is installed (such as hostname, port, and TLS configuration).

## Fabric v2.4.1 Improvements

The test network uses the latest Fabric release (v2.4.1), which facilitates using chaincode as a service:

- The Docker image for the peer contains a preconfigured builder for chaincode-as-a-service, named 'ccaasbuilder'. This removes the prior requirement to build your own external builder and repackage and configure the peer.
- The `ccaasbuilder` applications are included in the binary tgz archive download for use in other circumstances. The `sampleconfig/core.yaml` is updated to refer to 'ccaasbuilder'.
- The Fabric v2.4.1 Java chaincode removes the requirement to write a custom bootstrap main class (as implemented in the Node.js chaincode and planned for the go chaincode).

**NOTE:** This core functionality is also available in earlier releases, but with the requirements of writing the external chaincode code builder binaries and configuring the core.yaml correctly.

## End-to-end with the test-network

The `test-network` and some of the chaincodes have been updated to support running chaincode-as-a-service. The commands below require the latest fabric-samples, along with the latest Fabric Docker images.

Begin by opening two terminal windows, one for starting the Fabric test-network, and another for monitoring the Docker containers. In the 'monitoring' window, run the following bash scripts to watch activity from the Docker containers on the `fabric_test` network; this will monitor all Docker containers that are added to the `fabric-test` network.

The test-network is typically created by running the `./network.sh up` command, so delay running the bash scripts until the network is created. (Note the network can be created in advance using `docker network create fabric-test`.)

```bash
# from the fabric-samples repo
./test-network/monitordocker.sh
```

In the 'Fabric Network' window, start the test network:

```bash
cd test-network
./network.sh up createChannel
```

Variants of the next command, such as to use CouchDB or CAs, can be used without affecting the chaincode-as-a-service feature. The three keys steps are as follows, in no required order:

1. Build a Docker image of the chaincode package, which contains information for determining where the chaincode containers (hosting one or more contracts) are running. Both `/asset-transfer-basic/chaincode-typescript` and `/asset-transfer-basic/chaincode-java` have been updated with Docker files.
2. Install, approve, and commit a chaincode definition; these commands are run regardless of whether external chaincode builders are used. The chaincode package contains connection information (hostname, port, TLS certificates) only, with no code.
3. Start the Docker container(s) containing the contract.

The containers must be running before the first transaction is committed by the peer. This could be on the `commit` if the `initRequired` flag is set.

This sequence can be run as follows:

```bash
./network.sh deployCCAAS  -ccn basicts -ccp ../asset-transfer-basic/chaincode-typescript
```

This is similar to the `deployCC` command in that it specifies the name and path. Because each container is on the `fabric-test` network, changing the port can avoid collisions with other chaincode containers. If you run multiple services, the ports will need to change.

If successful to this point, the smart contract (chaincode) should be starting in the monitoring window. There should be two containers running, one for `org1` and one for `org2`. The container names contain the organization, peer, and chaincode name.

As a test, run the 'Contract Metadata' function as shown below. (For details on testing as different organizations, see [Interacting with the network](https://hyperledger-fabric.readthedocs.io/en/latest/test_network.html#interacting-with-the-network).

```bash
# Environment variables for Org1

export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="Org1MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/tlsca/tlsca.org1.example.com-cert.pem
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_ADDRESS=localhost:7051
export PATH=${PWD}/../bin:$PATH
export FABRIC_CFG_PATH=${PWD}/../config

# invoke the function
peer chaincode query -C mychannel -n basicts -c '{"Args":["org.hyperledger.fabric:GetMetadata"]}' | jq
```

Note that `| jq` can be omitted if `jq` is not installed. However, the metadata shows details of the deployed contract in JSON, so `jq` provides legibility. To confirm that the smart contract is working, repeat the prior commands for `org2`.

To run the Java example, change the `deployCCAAS` command as follows, which will create two new containers:

```bash
./network.sh deployCCAAS  -ccn basicj -ccp ../asset-transfer-basic/chaincode-java
```

### Troubleshooting

If a passed JSON structure is not well-formatted, the peer log will include the following error:

```
::Error: Failed to unmarshal json: cannot unmarshal string into Go value of type map[string]interface {} command=build
```

## How to configure each language

Each language can function in the '-as-a-service' mode. The following approaches are based on the latest libraries at the time of publication. When starting the image, any TLS options or additional logging options for the respective chaincode libraries can be specified.

### Java

With the Fabric v2.4.1 Java chaincode libraries, there are no code changes or build changes to implement. The '-as-a-service' mode will be used if the environment variable `CHAINCODE_SERVER_ADDRESS` is set.

The following sample Docker run command shows the two required variables, `CHAINCODE_SERVER_ADDRESS` and `CORE_CHAICODE_ID_NAME`:

```bash
    docker run --rm -d --name peer0org1_assettx_ccaas  \
                  --network fabric_test \
                  -e CHAINCODE_SERVER_ADDRESS=0.0.0.0:9999 \
                  -e CORE_CHAINCODE_ID_NAME=<use package id here> \
                   assettx_ccaas_image:latest
```

### Node.js

For Node.js (JavaScript or TypeScript) chaincode, `package.json` typically has `fabric-chaincode-node start` as the main start command. To run in the '-as-a-service' mode change this start command to `fabric-chaincode-node server --chaincode-address=$CHAINCODE_SERVER_ADDRESS --chaincode-id=$CHAINCODE_ID`.

## Debugging the Chaincode

Running in '-as-a-service' mode provides options, similar to Fabric 'dev' mode for debugging code. The restrictions of 'dev' mode do not apply to '-as-a-service'.

The `-ccaasdocker false` option can be provided with the `deployCCAAS` command to _not_ build the Docker image or start a Docker container. The option outputs the commands that would have run.

Command output is similar to the following example:

```bash
./network.sh deployCCAAS  -ccn basicj -ccp ../asset-transfer-basic/chaincode-java -ccaasdocker false
#....
Not building docker image; this the command we would have run
docker build -f ../asset-transfer-basic/chaincode-java/Dockerfile -t basicj_ccaas_image:latest --build-arg CC_SERVER_PORT=9999 ../asset-transfer-basic/chaincode-java
#....
Not starting docker containers; these are the commands we would have run
    docker run --rm -d --name peer0org1_basicj_ccaas                    --network fabric_test                   -e CHAINCODE_SERVER_ADDRESS=0.0.0.0:9999                   -e CHAINCODE_ID=basicj_1.0:59dcd73a14e2db8eab7f7683343ce27ac242b93b4e8075605a460d63a0438405 -e CORE_CHAINCODE_ID_NAME=basicj_1.0:59dcd73a14e2db8eab7f7683343ce27ac242b93b4e8075605a460d63a0438405                     basicj_ccaas_image:latest
```

**Note**: The previous commands may require adjustments depending on the directory location or debugging requirements.

### Building the Docker image

The first requirement for debugging chaincode is building the Docker image. As long as the peer can connect to the `hostname:port` specified in `connection.json` the actual packaging of the chaincode is not important to the peer. The Docker files specified below can be relocated.

Manually build the Docker image for `asset-transfer-basic/chaincode-java`:

```bash
docker build -f ../asset-transfer-basic/chaincode-java/Dockerfile -t basicj_ccaas_image:latest --build-arg CC_SERVER_PORT=9999 ../asset-transfer-basic/chaincode-java
```

### Starting the Docker container

Next, the Docker container must be started. In Node.js, for example, the container could be started as follows:

```bash
 docker run --rm -it -p 9229:9229 --name peer0org2_basic_ccaas --network fabric_test -e DEBUG=true -e CHAINCODE_SERVER_ADDRESS=0.0.0.0:9999 -e CHAINCODE_ID=basic_1.0:7c7dff5cdc43c77ccea028c422b3348c3c1fb5a26ace0077cf3cc627bd355ef0 -e CORE_CHAINCODE_ID_NAME=basic_1.0:7c7dff5cdc43c77ccea028c422b3348c3c1fb5a26ace0077cf3cc627bd355ef0 basic_ccaas_image:latest
```

In Java, for example, the Docker container could be started as follows:

```bash
 docker run --rm -it --name peer0org1_basicj_ccaas -p 8000:8000 --network fabric_test -e DEBUG=true -e CHAINCODE_SERVER_ADDRESS=0.0.0.0:9999 -e CHAINCODE_ID=basicj_1.0:b014a03d8eb1898535e25b4dfeeb3f8244c9f07d91a06aec03e2d19174c45e4f -e CORE_CHAINCODE_ID_NAME=basicj_1.0:b014a03d8e
b1898535e25b4dfeeb3f8244c9f07d91a06aec03e2d19174c45e4f  basicj_ccaas_image:latest
```

### Debugging Prerequisites

The following prerequisites apply to debugging all languages:

- The container name must match the name in the peer's `connection.json`.
- The peer is connecting to the chaincode container via the Docker network. Therefore, port 9999 does not need to be forwarded to the host.
- Single stepping in a debugger is likely to trigger the default Fabric transaction timeout value of 30 seconds. Increase the time that the chaincode has to complete transactions, to 300 seconds, by adding  `CORE_CHAINCODE_EXECUTETIMEOUT=300s` to the environment options for each peer in the `test-network/docker/docker-composer-test-net.yml` file.
- In the `docker run` command in the previous section, the test-network `-d` default option has been replaced with `-it`. This change runs the Docker container in the foreground and not in detached mode.

The following prerequisites apply to debugging Node.js:

- Port 9229 is forwarded. However, this is the debug port used by Node.js.
- `-e DEBUG=true` will trigger the node runtime to be started in debug mode. This is encoded in the `docker/docker-entrypoint.sh` script, which **for security purposes, should be considered for removal from production images**.
- If you are using TypeScript, ensure that the TypeScript has been compiled with `sourcemaps`; otherwise, a debugger will have difficulty matching up the source code.

The following prerequisites apply to debugging Java:

- Port 8000 is forwarded, which is the debug port for the JVM.
- `-e DEBUG=true` will trigger the node runtime to be started in debug mode. This is an example encoded in the `docker/docker-entrypoint.sh` script, which **for security purposes, should be considered for removal from production images**.
- The `java` command option to start the debugger is `java -agentlib:jdwp=transport=dt_socket,server=y,suspend=n,address=0.0.0.0:8000 -jar /chaincode.jar`.  Note `0.0.0.0`, as the debug port, must be bound to all network adapters so the debugger can be attached from outside the container.

## Running with multiple peers

In the earlier method, each peer that the chaincode is approved on will have a container running the chaincode. The '-as-a-service' approach requires achieving the same architecture.

The `connection.json` contains the address of the running chaincode container, so it can be updated to ensure that each peer connects to a different container. However, as with the `connection.json` in the chaincode package, Fabric mandates that the package ID be consistent across all peers in an organization. To achieve this, the external builder supports a template capability. The context from this template is taken from the environment variable `CHAINCODE_AS_A_SERVICE_BUILDER_CONFIG` set on each peer.

Define the address to be a template in `connection.json` as follows:

```json
{
  "address": "{{.peername}}_assettransfer_ccaas:9999",
  "dial_timeout": "10s",
  "tls_required": false
}
```

In the peer's environment configuration, set the following variable for org1's peer1:

```bash
CHAINCODE_AS_A_SERVICE_BUILDER_CONFIG="{\"peername\":\"org1peer1\"}"
```

The external builder will then resolve this address to be `org1peer1_assettransfer_ccaas:9999` for the peer to use.

Each peer can have its own separate configuration, and therefore a unique address. The JSON string that is set can have any structure, as long as the templates (in golang template syntax) match.

Any value in `connection.json` can be templated, but only the values and not the keys.

<!---
Licensed under Creative Commons Attribution 4.0 International License https://creativecommons.org/licenses/by/4.0/
-->
