
# Performance Traffic Engine - PTE

The Performance Traffic Engine (PTE) uses [Hyperledger Fabric Client (HFC) Node SDK](https://fabric-sdk-node.github.io/index.html)
to interact with a [Hyperledger Fabric](http://hyperledger-fabric.readthedocs.io/en/latest/) network.

## Table Of Contents:
- [Prerequisites](#prerequisites)
- [Setup](#setup)
- [Running PTE](#running-pte)
     - [Usage](#usage)
     - [Transaction Execution](#transaction-execution)
     - [Transaction Type](#transaction-type)
     - [Sample Use Cases](#sample-use-cases)
     - [Chaincodes](#chaincodes)
     - [Output](#output)
 - [Reference](#reference)
    - [User Input File](#user-input-file)
    - [Service Credentials File](#service-credentials-file)
    - [Creating a Local Fabric Network](#creating-a-local-fabric-network)

---

### Code Base for v1.0.0-alpha2

- Fabric commit level: `6b6bfcfbd1e798a8a08fa9c3bf4dc0ff766a6b87`
- fabric-sdk-node commit level: `f13f4b42e7155ec0dc3d7485b202bb6a6ca73aed`
- fabric-ca commit level: `97ca16ca4f883a65072da860fce9eb76da269d62`
- PTE commit level: `latest`

### Code Base for v1.0.0-alpha
For v1.0.0-alpha support, use v1performance commit level `aa73747ccf5f511fbcd10a962dd1e588bde1a8b0`.
Below is the v1.0.0-alpha commit levels.

- Fabric commit level: `fa3d88cde177750804c7175ae000e0923199735c`
- fabric-sdk-node commit level: `196d0484c884ab894374c73df89bfe047bcc9f00`
- fabric-ca commit level: `29385879bc2931cce9ec833acf796129908b72fb`
- PTE v1performance commit level: `aa73747ccf5f511fbcd10a962dd1e588bde1a8b0`

### Future items

- PTE needs to supports any number of organizations in a channel. PTE supports two organizations per channel now (FAB-3809).
- PTE can only send transactions to the anchor peer of an organization. It will need to be able to send transactions to any peer.
- Endorsement policy is not supported yet.
- Replace `git clone https://github.com/hyperledger/fabric-sdk-node.git` with fabric-client and fabric-ca-client.
- Post-alpha2, remove v1performance info.

## Prerequisites
To build and test the following prerequisites must be installed first:

- node and npm lts/boron release (v6.10.x and v3.10.x)
    -  node v7 is not currently supported
- gulp command
    - `npm install -g gulp`
- go (v1.7 or later)
    - refer to [Go - Getting Started](https://golang.org/doc/install)
- others:
    - in Ubuntu: `apt install -y build-essential python libltdl-dev`
    - or refer to your distribution's repository

If planning to run your Fabric network locally, you'll need docker and a bit more. See [Hyperledger Fabric - Getting Started](http://hyperledger-fabric.readthedocs.io/en/latest/getting_started.html) for details.

## Setup
1. Download fabric sources and checkout appropriate commit levels (v1.0.0-alpha2 shown here):
    - `go get -d github.com/hyperledger/fabric`
    - `cd $GOPATH/src/github.com/hyperledger/fabric/`
    - `git checkout v1.0.0-alpha2`
    - Optional: `make docker`
    - `go get -d github.com/hyperledger/fabric-ca`
    - `cd $GOPATH/src/github.com/hyperledger/fabric-ca/`
    - `git checkout v1.0.0-alpha2`
    - Optional: `make docker`
    - `go get -d github.com/hyperledger/fabric-sdk-node`
    - `cd $GOPATH/src/github.com/hyperledger/fabric-sdk-node`
    - `git checkout v1.0.0-alpha2`

    If `make docker` is skipped, the assumption is that the user will either acquire docker images from another source, or PTE will run against a remote Fabric network. See [Creating a local Fabric network](#creating-a-local-fabric-network) for additional information on this.

2. Install node packages and build the ca libraries:
    - `npm install`
        -  you should be able to safely ignore any warnings
    -  `gulp ca`

3. Clone PTE
    **Note:** This will not be necessary in future releases (post-alpha2) as PTE has been merged into Fabric.
    - If testing v1-alpha or v1-alpha2, clone the v1performance repo:
        - `cd test`
        - `git clone https://github.com/dongmingh/v1performance`
        - `cd v1performance`
        - If testing v1.0.0-alpha: `git reset --hard aa73747ccf5f511fbcd10a962dd1e588bde1a8b0`
    - If testing against latest Fabric commit, copy from Fabric repo:
        - `cp -r $GOPATH/src/github.com/hyperledger/fabric/test/tools/PTE .`
        - `cd PTE`

4. Create Service Credentials file(s) for your Fabric network:
    - See the examples in `SCFiles` and change the address to your own Fabric addresses and credentials. Add a block for each organization and peer, ensuring correctness.

5. Specify run scenarios:
    - Create your own version of runCases.txt and User Input json files, according to the test requirements. Use the desired chaincode name, channel name, organizations, etc. Using the information in your own network profiles, remember to "create" all channels, and "join" and "install" for each org, to ensure all peers are set up correctly. Additional information can be found below.

## Running PTE
Before attempting to run PTE ensure your network is running!
If you do not have access to a Fabric network, please see the section on [Creating a local Fabric network](#creating-a-local-fabric-network).

### Usage
`./pte_driver.sh <run cases file>`
### Example
`./pte_driver.sh userInputs/runCases.txt`

`userInputs/runCases.txt` contains the list of user specified test cases to be executed. Each line is a test case and includes two parameters: **SDK type** and **user input file**.  

For instance, a run cases file containing two test cases using the node SDK would be:
```
sdk=node userInputs/samplecc-chan1-i.json
sdk=node userInputs/samplecc-chan2-i.json
```

**Note:** Available SDK types are node, go, python and java; however, only the node SDK is currently supported.

See [User Input file](#user-input-file) in the Reference section below for more information about these files.

### Transaction Execution
A single test case is described by a user input file. User input files define all the parameters for executing a test; including transaction type, number of threads, number of transactions, duration, etc. All threads in one test case will concurrently execute the specified transaction. Different transactions may be used in different test cases and then combined into a single run cases file, making it possible to create more complicated scenarios. For example, in a single run of PTE, a user could send a specific number of invokes to all peers and then query each peer separately.

There are two ways to control transaction execution:
* **transaction number**: Each thread executes the specified number of transactions specified by nRequest in the user input file.
* **run time duration**: Each thread executes the same transaction concurrently for the specified time duration specified by runDur in the user input file, note that nRequest is set to 0.

### Transaction Type
* ### Invoke (move)
    To execute invoke (move) transactions, set the transType to Invoke and invokeType to Move, and specify the network parameters and desired execution parameters:
    ```
    "invokeCheck": "TRUE",
    "transMode": "Constant",
    "transType": "Invoke",
    "invokeType": "Move",
    "nOrderer": "1",
    "nOrg": "2",
    "nPeerPerOrg": "2",
    "nProc": "4",
    "nRequest": "1000",
    "runDur": "600",
    "TLS": "Disabled",
    ```
    And set the channel name in channelOpt:
    ```
    "channelOpt": {
        "name": "testChannel1",
        "action":  "create",
        "orgName": [
            "testOrg1"
        ]
    },
    ```
* ### Invoke (query)
    To execute invoke (move) transactions, set the transType to Invoke and invokeType to Query, and specify the network parameters and desired execution parameters:
    ```
    "invokeCheck": "TRUE",
    "transMode": "Constant",
    "transType": "Invoke",
    "invokeType": "Query",
    "nOrderer": "1",
    "nOrg": "2",
    "nPeerPerOrg": "2",
    "nProc": "4",
    "nRequest": "1000",
    "runDur": "600",
    "TLS": "Disabled",
    ```
    And set the channel name in channelOpt:
    ```
    "channelOpt": {
        "name": "testChannel1",
        "action":  "create",
        "orgName": [
            "testOrg1"
        ]
    },
    ```

### Sample Use Cases
* ### Latency
    Example: `userInputs/samplecc-latency-i.json`
    Performs 1000 invokes (Move) with 1 thread on 1 network using the sample_cc chaincode. The average of the execution result (execution time (ms)/1000 transactions) represents the latency of 1 invoke (Move).
* ### Long run
    Example: `userInputs/samplecc-longrun-i.json`
    Performs invokes (Move) of various payload size ranging from 1kb-2kb with 1 threads on one network using sample_cc chaincode for 72 hours at 1 transaction per second.
* ### Concurrency
    Example: `userInputs/samplecc-concurrency-i.json`
    Performs invokes (Move) of 1kb payload with 50 threads on one 4-peer network using sample_cc chaincode for 10 minutes.
* ### Complex
    Example: `userInputs/samplecc-complex-i.json`
    Performs invokes (Move) of various payload size ranging from 10kb-500kb with 10 threads on one 4-peer network using sample_cc chaincode for 10 minutes. Each invoke (Move) is followed by an invoke (Query).
* ### More complicated scenarios
    * For multiple different chaincode deployments and transactions, configure each user input file to deploy different chaincodes and drive the transactions appropriately.
    * For a density test, configure multiple SCFiles, one for each network. Then concurrently execute the test against these multiple networks with unique workloads specified in the user input files.
    * For a stress test on a single network, set all SCFiles to same network. Then concurrent execution of the test is performed against the network but with the workload specified in each user input file.

## Additional Use Cases
Although PTE's primary use case is to drive transactions into a Fabric network, it can be used for creating and joining channels, and chaincode installation and instantiation. This gives the ability for more complete end-to-end scenarios.
* ### Channel Operations
    For any channel activities (create or join), set transType to `Channel`:
    ```
    "transMode": "Simple",
    "transType": "Channel",
    "invokeType": "Move",
    ```
    * ### Create a channel
        To create a channel, set the action in channelOpt to `create`, and set the name to the channel name. Note that orgName is ignored in this operation:
        ```
        "channelOpt": {
            "name": "testChannel1",
            "action":  "create",
            "orgName": [
                "testOrg1"
            ]
        },
        ```
    * ### Join a channel
        To join all peers in an org to a channel, set the action in channelOpt to `join`, set name to channel name, and set orgName to the list of orgs to join:
        ```
        "channelOpt": {
            "name": "testChannel1",
            "action":  "join",
            "orgName": [
                "testOrg1"
            ]
        },
        ```
* ### Chaincode Operations
    For chaincode setup (install or instantiate) set `deploy` according to the test. For example:
    ```
    "deploy": {
        "chaincodePath": "github.com/sample_cc",
        "fcn": "init",
        "args": []
    },
    ```
    * ### Install a chaincode
        To install a chaincode, set the transType to `install`:
        ```
        "transMode": "Simple",
        "transType": "install",
        "invokeType": "Move",
        ```
        And set channelOpt name to the channel name and orgName to a list of org names:
        ```
        "channelOpt": {
            "name":  "testChannel1",
            "action":  "create",
            "orgName": [
                "testOrg1"
            ]
        },
        ```
        Note that action is ignored.
    * ### Instantiate a chaincode
        To instantiate a chaincode, set the transType to `instantiate`:
        ```
        "transMode": "Simple",
        "transType": "instantiate",
        "invokeType": "Move",
        ```
        and set channelOpt name to the channel name:
        ```
        "channelOpt": {
            "name":  "testChannel1",
            "action":  "create",
            "orgName": [
                "testOrg1"
            ]
        },
        ```
        Note that action and orgName are ignored.

## Chaincodes
The following chaincodes are tested and supported:
* **example02**: This is a simple chaincode with limited capability.  This chaincode is **NOT** suitable for performance benchmark.
* **ccchecker**:  This chaincode supports variable payload sizes.
See userInput-ccchecker.json for example of userInput file. Take the following steps to install this chaincode:
  - `cd $GOPATH/src/github.com/hyperledger/fabric-sdk-node/test/fixtures/src/github.com`
  - `mkdir ccchecker`
  - download newkeyperinvoke.go into ccchecker directory
* **sample_cc**: This chaincode supports variable (randomized) payload sizes and performs encryption and decryption on the payload. Specify ccType as ccchecker when using this chaincode.
See userInput-samplecc.json for example of userInput file. Take the following steps to install this chaincode:
  - `cd $GOPATH/src/github.com/hyperledger/fabric-sdk-node/test/fixtures/src/github.com`
  - `mkdir sample_cc`
  - download chaincode_sample.go into sample_cc directory

## Output
The output includes network id, thread id, transaction type, total transactions, completed transactions, failed transactions, starting time, ending time, and elapsed time.
* For example, consider a test case that has 4 threads driving a single peer. The output shows that network 0 thread 0 executed 1000 moves with no failure in 406530 ms, network 0 thread 1 executed 1000 moves with no failure in 400421 ms, and so on.  Note that the starting and ending timestamps are provided:
    ```
    stdout: [Nid:id=0:3] eventRegister: completed 1000(1000) Invoke(Move) in 259473 ms, timestamp: start 1492024894518 end 1492025153991
    stdout: [Nid:id=0:2] eventRegister: completed 1000(1000) Invoke(Move) in 364174 ms, timestamp: start 1492024894499 end 1492025258673
    stdout: [Nid:id=0:1] eventRegister: completed 1000(1000) Invoke(Move) in 400421 ms, timestamp: start 1492024894500 end 1492025294921
    stdout: [Nid:id=0:0] eventRegister: completed 1000(1000) Invoke(Move) in 406530 ms, timestamp: start 1492024894498 end 1492025301028
    ```

## Reference

### User Input file
```
{
    "channelID": "_ch1",
    "chaincodeID": "sample_cc",
    "chaincodeVer": "v0",
    "chainID": "testchainid",
    "logLevel": "ERROR",
    "invokeCheck": "TRUE",
    "transMode": "Simple",
    "transType": "Invoke",
    "invokeType": "Move",
    "nOrderer": "1",
    "nOrg": "2",
    "nPeerPerOrg": "2",
    "nProc": "4",
    "nRequest": "0",
    "runDur": "600",
    "TLS": "enabled",
    "channelOpt": {
        "name": "testOrg1",
        "channelTX": "/root/gopath/src/github.com/hyperledger/fabric/common/tools/cryptogen/crypto-config/ordererOrganizations/testOrgsChannel1.tx",
        "action":  "create",
        "orgName": [
            "testOrg1"
        ]
    },
    "burstOpt": {
        "burstFreq0":  "500",
        "burstDur0":  "3000",
        "burstFreq1": "2000",
        "burstDur1": "10000"
    },
    "mixOpt": {
        "mixFreq": "2000"
    },
    "constantOpt": {
        "recHIST": "HIST",
        "constFreq": "1000",
        "devFreq": 300
    },
    "ccType": "general",
    "ccOpt": {
        "keyStart": "5000",
        "payLoadMin": "1024",
        "payLoadMax": "2048"
    },
    "deploy": {
        "chaincodePath": "github.com/ccchecker",
        "fcn": "init",
        "args": []
    },
    "invoke": {
        "query": {
            "fcn": "invoke",
            "args": ["get", "a"]
        },
        "move": {
            "fcn": "invoke",
            "args": ["put", "a", "string-msg"]
        }
    },   
    "SCFile": [
        {"ServiceCredentials":"SCFiles/config-local.json"}
    ]
}
```
where:
* **channelID**: channel ID for the run.
* **chaincodeID**: chaincode ID for the run.
* **chaincodeVer**: chaincode version.
* **chainID**: chain ID for the run. **DO NOT CHANGE.**
* **logLevel**: logging level for the run. Options are ERROR, DEBUG, or INFO.  Set to ERROR for performance test. The default value is ERROR.
* **invokeCheck**: if this is TRUE, then a query will be executed for the last invoke upon the receiving of the event of the last invoke. This value is ignored for query test.
* **transMode**: transaction mode
    * **Simple**: one transaction type and rate only, the subsequent transaction is sent when the response of sending transaction (not the event handler), success or failure, of the previous transaction is received
    * **Burst**: various traffic rates, see burstOpt for detailed
    * **Mix**: mix invoke and query transactions, see mixOpt for detailed
    * **Constant**: the transactions are sent by the specified rate, see constantOpt for detailed
    * **Latency**: one transaction type and rate only, the subsequent transaction is sent when the event message (ledger update is completed) of the previous transaction is received
* **transType**: transaction type
    * **Channel**: channel activities specified in channelOpt.action
    * **Install**: install chaincode
    * **Instantiate**: instantiate chaincode
    * **Invoke**: invokes transaction
* **invokeType**: invoke transaction type. This parameter is valid only if the transType is set to invoke
    * **Move**: move transaction
    * **Query**: query transaction
* **nOrderer**: number of orderers for traffic, this number shall not exceed the actual number of orderers in the network, or some transactions may fail. One orderer is assigned to one thread with round robin. PTE currently only supports 1 orderer.
* **nOrg**: number of organitzations for the test
* **nPeerPerOrg**: number of peers per organization for the test
* **nProc**: number of processes for the test
* **nRequest**: number of transactions to be executed for each thread
* **runDur**: run duration in seconds to be executed if nRequest is 0
* **TLS**: TLS setting for the test: Disabled or Enabled.
* **channelOpt**: transType channel options
    * **name**: channel name
    * **channelTX**: channel transaction file
    * **action**: channel action: create or join
    * **orgName**: name of organization for the test
* **burstOpt**: the frequencies and duration for Burst transaction mode traffic. Currently, two transaction rates are supported. The traffic will issue one transaction every burstFreq0 ms for burstDur0 ms, then one transaction every burstFreq1 ms for burstDur1 ms, then the pattern repeats. These parameters are valid only if the transMode is set to **Burst**.
    * **burstFreq0**: frequency in ms for the first transaction rate
    * **burstDur0**:  duration in ms for the first transaction rate
    * **burstFreq1**: frequency in ms for the second transaction rate
    * **burstDur1**: duration in ms for the second transaction rate
* **mixOpt**: each invoke is followed by a query on every thread. This parameter is valid only the transMode is set to **Mix**.
* **mixFreq**: frequency in ms for the transaction rate. This value should be set based on the characteristics of the chaincode to avoid the failure of the immediate query.
* **constantOpt**: the transactions are sent at the specified rate. This parameter is valid only the transMode is set to **Constant**.
    * **recHist**: This parameter indicates if brief history of the run will be saved.  If this parameter is set to HIST, then the output is saved into a file, namely ConstantResults.txt, under the current working directory.  Otherwise, no history is saved.
    * **constFreq**: frequency in ms for the transaction rate.
    * **devFreq**: deviation of frequency in ms for the transaction rate. A random frequency is calculated between constFrq-devFreq and constFrq+devFreq for the next transaction.  The value is set to default value, 0, if this value is not set in the user input json file.  All transactions are sent at constant rate if this number is set to 0.
* **ccType**: chaincode type
    * **ccchecker**: The first argument (key) in the query and invoke request is incremented by 1 for every transaction.  The prefix of the key is made of thread ID, ex, all keys issued from thread 4 will have prefix of **key3_**. And, the second argument (payload) in an invoke (Move) is a random string of size ranging between payLoadMin and payLoadMax defined in ccOpt.
    * **auction**: The first argument (key) in the query and invoke request is incremented by 1 for every transaction.  And, the invoke second argument (payload) is made of a random string with various size between payLoadMin and payLoadMax defined in ccOpt. (**to be tested**)
    * **general**: The arguments of transaction request are taken from the user input json file without any changes.
* **ccOpt**: chaincode options
    * **keyStart**: the starting transaction key index, this is used when the ccType is non general which requires a unique key for each invoke.
    * **payLoadMin**: minimum size in bytes of the payload. The payload is made of random string with various size between payLoadMin and payLoadMax.
    * **payLoadMax**: maximum size in bytes of the payload
* **deploy**: deploy transaction contents
* **invoke** invoke transaction contents
    * **query**: query content
    * **move**: move content
* **SCFile**: the service credentials json.


## Service Credentials file
The service credentials contain the information of the network and are stored in the `SCFiles` directory. The following is a sample of the service credentials json file:
```
{
    "test-network": {
            "orderer": {
                    "name": "OrdererMSP",
                    "mspid": "OrdererMSP",
                    "mspPath": "./crypto-config",
                    "adminPath": "./crypto-config/ordererOrganizations/example.com/users/Admin@example.com/msp",
                    "comName": "example.com",
                    "url": "grpcs://localhost:5005",
                    "server-hostname": "orderer0.example.com",
                    "tls_cacerts": "./crypto-config/ordererOrganizations/example.com/orderers/orderer0.example.com/msp/cacerts/ca.example.com-cert.pem"
            },
            "org1": {
                    "name": "Org1MSP",
                    "mspid": "Org1MSP",
                    "mspPath": "./crypto-config",
                    "adminPath": "./crypto-config/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp",
                    "comName": "example.com",
                    "ca": {
                         "url":"https://localhost:7054",
                         "name": "ca-org1"
                    },
                    "username": "admin",
                    "secret": "adminpw",
                    "peer1": {
                            "requests": "grpc://localhost:7061",
                            "events": "grpc://localhost:7051",
                            "server-hostname": "peer0.org1.example.com",
                            "tls_cacerts": "../fixtures/tls/peers/peer0/ca-cert.pem"
                    },
                    "peer2": {
                            "requests": "grpc://localhost:7062",
                            "events": "grpc://localhost:7052",
                            "server-hostname": "peer1.org1.example.com",
                            "tls_cacerts": "../fixtures/tls/peers/peer0/ca-cert.pem"
                    }
            },
            "org2": {
                    "name": "Org2MSP",
                    "mspid": "Org2MSP",
                    "mspPath": "./crypto-config",
                    "adminPath": "./crypto-config/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp",
                    "comName": "example.com",
                    "ca": {
                         "url":"https://localhost:8054",
                         "name": "ca-org2"
                    },
                    "username": "admin",
                    "secret": "adminpw",
                    "peer1": {
                            "requests": "grpcs://localhost:7063",
                            "events": "grpcs://localhost:7053",
                            "server-hostname": "peer0.org2.example.com",
                            "tls_cacerts": "./crypto-config/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/msp/cacerts/ca.org2.example.com-cert.pem"
                    },
                    "peer2": {
                            "requests": "grpcs://localhost:7064",
                            "events": "grpcs://localhost:7054",
                            "server-hostname": "peer1.org2.example.com",
                            "tls_cacerts": "./crypto-config/peerOrganizations/org2.example.com/peers/peer1.org2.example.com/msp/cacerts/ca.org2.example.com-cert.pem"
                    }
            }
    }
}
```


## Creating a local Fabric network
- If you do not yet have the Fabric docker images in your local docker registry, please either build them from Fabric source or download them from dockerhub.
    - `cd $GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/`
    - `sh ./download-dockerimages.sh -c x86_64-1.0.0-alpha2 -f x86_64-1.0.0-alpha2`
- If you do not have an existing network already, you can start a network using the Fabric e2e example:
    - `cd $GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/`
    - Edit `network_setup.sh` and change **COMPOSE_FILE**:
        ```
        #COMPOSE_FILE=docker-compose-cli.yaml
        COMPOSE_FILE=docker-compose-e2e.yaml
        ```
    - `./network_setup.sh up`
- Alternatively, consider using the [NetworkLauncher](https://github.com/dongmingh/v1Launcher) tool:
    - `cd $GOPATH/src/github.com/hyperledger/fabric-sdk-node/test`
    - `git clone https://github.com/dongmingh/v1Launcher`
    - `cd v1Launcher`
    - `./NetworkLauncher.sh -?`


---

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
