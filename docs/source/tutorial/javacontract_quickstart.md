# Quick Start - Java Smart Contract

**Audience:** Java application and smart contract developers, network administrators

The purpose of this tutorial is to show how you can quickly deploy and run a Java smart contract on a sample Fabric network by using the Java version of the Fabcar sample chaincode. It also includes some tips for monitoring the running chaincode and information for how to debug a running smart contract. The tutorial includes the following steps:

* [Setup your environment](#prerequisites).
* [Download the Fabric samples](#download-the-fabric-samples).
* [Start a Fabric network](#start-a-fabric-network) that can be used for testing a java smart contract.
* (Optional) [Setup Logspout](#setup-logspout).
* [Install and instantiate the Fabcar chaincode](#install-and-instantiate-the-fabcar-chaincode).
* [Execute transaction functions](#execute-a-transaction-functions).
* [Update the ledger](#update-the-ledger).
* [Debugging a smart contract](#debugging-a-smart-contract).

## Prerequisites

Before you begin, you need to install some prerequisite technology required by the tutorial.

 * [**git**](https://git-scm.com/downloads) Download the latest version of git if you do not already have it installed.
 * [**Docker and Docker Compose**]((../prereqs.html)) Docker help developers and administrators create standard environments for building and running applications and smart contracts.

**Note:** You do not need to have Java, Node or the Fabric `peer` commands installed, but for development you will at some point need these tools.

## Tutorial steps:

The following set of steps will download the Fabric samples that include a basic Fabric network as well as the Fabcar chaincode that we will deploy to the Fabric network. Then you can issue transactions to query and update the blockchain ledger.

### Download the Fabric samples

The Java smart contract tutorial is one of the Hyperledger Fabric [samples](https://github.com/hyperledger/fabric-samples) held in a public
[GitHub](https://www.github.com) repository called `fabric-samples`. As you're going to run the tutorial on your machine, your first task is to download the
`fabric-samples` repository.  

* Create a local 'working directory' to clone the example repository.
* Follow the instructions to [Install Samples, Binaries, and Docker images](../install.html).

### Start a Fabric network

Before you can run and test the smart contract, you will need a Fabric network.  Run the following commands to start the Fabric Network using the `basic-network` sample. These commands deploy and start the peer, orderer, CA, and CouchDB containers that are required in order to install and run the smart contract. It also creates a channel named `mychannel` that is used later in the tutorial.

Change to the `fabric-samples/basic-network` directory.

```
$ cd fabric-samples//basic-network
./start.sh
```

After the commands complete, run `docker ps` to see the Docker containers that are started. Your output should be similar to the following example:

```
➜  basic-network git:(release-1.4) docker ps
CONTAINER ID        IMAGE                        COMMAND                  CREATED             STATUS              PORTS                                            NAMES
f9cb3ea067da        hyperledger/fabric-peer      "peer node start"        3 minutes ago       Up 3 minutes        0.0.0.0:7051->7051/tcp, 0.0.0.0:7053->7053/tcp   peer0.org1.example.com
92447f306949        hyperledger/fabric-ca        "sh -c 'fabric-ca-se…"   4 minutes ago       Up 3 minutes        0.0.0.0:7054->7054/tcp                           ca.example.com
d0d43f35afb5        hyperledger/fabric-couchdb   "tini -- /docker-ent…"   4 minutes ago       Up 3 minutes        4369/tcp, 9100/tcp, 0.0.0.0:5984->5984/tcp       couchdb
80fdbab89811        hyperledger/fabric-orderer   "orderer"                4 minutes ago       Up 3 minutes        0.0.0.0:7050->7050/tcp                           orderer.example.com
```

These containers are all attached to a Docker network. Running the command `docker network ls` will list all the Docker networks. If you have done other Docker work, your list of networks may differ, but you are looking for the `net_basic` network. We will refer to this `net_basic` network in the next section of the tutorial when we discuss monitoring the chaincode.

```
➜  basic-network git:(release-1.4) docker network ls      
NETWORK ID          NAME                DRIVER              SCOPE
a6c8a8c03f79        bridge              bridge              local
4796b5cc349d        host                host                local
00bc8097f7eb        net_basic           bridge              local
fbd4128b6d8c        none                null                local
```

### Setup Logspout

This step is not required, but is extremely useful for troubleshooting chaincode. To monitor the logs of the smart contract, an administrator can view the aggregated output from a set of Docker containers using the `logspout` [tool](https://logdna.com/what-is-logspout/). It collects the different output streams into one place, making it easy to see what's happening
from a single window. This can be really helpful for administrators when installing smart contracts or for developers when invoking smart contracts, for example. A script to install and configure Logspout is already included in the Commercial Paper tutorial, so we can leverage it here as well.

Start another console window in the same the working directory and use the `monitordocker.sh` script to start up a `Logspout` router that will track and display all the Docker container output. This will be important because some containers are created purely for the purposes of starting a smart contract and may only exist for a short time; however, their output is very useful for debugging.

You can run this script from it's original location or copy it to your main working directory. For ease of use we will copy the `monitordocker.sh` script from the `commercial-paper` sample to your working directory:
```
cp ./commercial-paper/organization/digibank/configuration/cli/monitordocker.sh .
# if you're not sure where it is
find . -name monitordocker.sh
```

Run this script in your newly created console against the `net_basic` Docker network that you saw earlier:
```
./monitordocker.sh net_basic
```
You should see output similar to the following:
```
Starting monitoring on all containers on the network net_basic
Unable to find image 'gliderlabs/logspout:latest' locally
latest: Pulling from gliderlabs/logspout
4fe2ade4980c: Pull complete
decca452f519: Pull complete
ad60f6b6c009: Pull complete
Digest: sha256:374e06b17b004bddc5445525796b5f7adb8234d64c5c5d663095fccafb6e4c26
Status: Downloaded newer image for gliderlabs/logspout:latest
1f99d130f15cf01706eda3e1f040496ec885036d485cb6bcc0da4a567ad84361

```
This command lists output from all the Docker containers, you won't see much at first. **Note:** It can be helpful to make this window wide, with a small font.

### Install and instantiate the Fabcar chaincode

One of the containers that was started is a CLI container that includes all of the Fabric CLI tools already installed and is a good alternative to installing the tools locally.
Run the following command  to get into the tools container:

```
docker-compose up -d cli   
docker exec -it cli bash
```

Now change to the directory that been mounted to the example chaincode.

```
➜  basic-network git:(release-1.4) docker exec -it cli bash
root@64a680191017:/opt/gopath/src/github.com/hyperledger/fabric/peer# cd /opt/gopath/src/github.com
root@64a680191017:/opt/gopath/src/github.com# ls
abac  chaincode_example02  fabcar  hyperledger  marbles02  marbles02_private  sacc
```

For this tutorial, we'll use Java version of the Fabcar chaincode for testing.

```
cd fabcar/java
```

First you need to to install the java version of the chaincode by runnning the following command. It is the `-l` flag that causes Java version of the chaincode to be used. Be sure to watch your second window for the output:

```
root@64a680191017:/opt/gopath/src/github.com/fabcar/java# peer chaincode install -n fabcar -v 1 -p $(pwd) -l java
2019-10-14 09:36:21.181 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 001 Using default escc
2019-10-14 09:36:21.181 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 002 Using default vscc
2019-10-14 09:36:21.261 UTC [chaincodeCmd] install -> INFO 003 Installed remotely response:<status:200 payload:"OK" >
```

The next step is to `instantiate` the chaincode which initializes the chaincode on the channel. As part of this process, the chaincode is rebuilt and put into a running Docker container. If there are problems with the chaincode, or something other issue then it is likely to surface when the chaincode is instantiated.

You should see output similar to the following in your second window:

```
frosty_mclean|Gradle build
frosty_mclean|Starting a Gradle Daemon, 1 incompatible and 1 stopped Daemons could not be reused, use --status for details
frosty_mclean|Download https://plugins.gradle.org/m2/com/github/johnrengelman/shadow/com.github.johnrengelman.shadow.gradle.plugin/2.0.4/com.github.johnrengelman.shadow.gradle.plugin-2.0.4.pom
frosty_mclean|Download https://plugins.gradle.org/m2/com/github/jengelman/gradle/plugins/shadow/2.0.4/shadow-2.0.4.pom
frosty_mclean|Download https://plugins.gradle.org/m2/org/codehaus/groovy/groovy-backports-compat23/2.4.4/groovy-backports-compat23-2.4.4.pom
frosty_mclean|Download https://plugins.gradle.org/m2/com/github/jengelman/gradle/plugins/shadow/2.0.4/shadow-2.0.4.jar
frosty_mclean|Download https://plugins.gradle.org/m2/org/codehaus/groovy/groovy-backports-compat23/2.4.4/groovy-backports-compat23-2.4.4.jar

```

The name of the container is chosen by Docker at random,  eventually you'll see something like:
```
frosty_mclean|:processTestResources NO-SOURCE
frosty_mclean|:testClasses
frosty_mclean|:checkstyleTest
frosty_mclean|:jacocoTestCoverageVerification SKIPPED
frosty_mclean|:jacocoTestReport SKIPPED
frosty_mclean|:check
frosty_mclean|:build
frosty_mclean|:shadowJar
frosty_mclean|
frosty_mclean|BUILD SUCCESSFUL in 54s
frosty_mclean|6 actionable tasks: 6 executed
```

After the chaincode is started, the command in the CLI container is also started. If you open another terminal and issue `docker ps`, a Docker container for the fabcar chaincode is added to the list of containers

```
165b7e9dee60        dev-peer0.org1.example.com-fabcar-1-55704394a74399d4b31ca5022fb02e80783229e680285eedddb37ddbff5c2e79   "/root/chaincode-jav…"   2 minutes ago       Up 2 minutes                                                         dev-peer0.org1.example.com-fabcar-1
```

This is your running chaincode container - with the Fabcar chaincode deployed within it.

### Execute transaction functions

As this is a test smart contract, we can 'bootstrap' some data into the ledger by using the `initLeger` method. The following `peer chaincode` command is also a good example of how to issue a transaction to a smart contract using an `invoke`.

```
root@64a680191017:/opt/gopath/src/github.com/fabcar/java# peer chaincode invoke -o orderer.example.com:7050 --channelID mychannel --name fabcar -c '{"Args":["initLedger"]}'
```
If this is successful, you will see results similar to:
```
2019-10-14 09:44:47.138 UTC [chaincodeCmd] chaincodeInvokeOrQuery -> INFO 001 Chaincode invoke successful. result: status:200
```

Now we can run the invoke command again, this time passing the `queryAllCars` method to list the cars on the ledger:

```
root@64a680191017:/opt/gopath/src/github.com/fabcar/java# peer chaincode invoke -o orderer.example.com:7050 --channelID mychannel --name fabcar -c '{"Args":["queryAllCars"]}'
```
You should see results similar to:
```
2019-10-14 10:19:40.463 UTC [chaincodeCmd] chaincodeInvokeOrQuery -> INFO 001 Chaincode invoke successful. result: status:200 payload:"[{\"owner\":\"Tomoko\",\"color\":\"blue\",\"model\":\"Prius\",\"make\":\"Toyota\"},{\"owner\":\"Brad\",\"color\":\"red\",\"model\":\"Mustang\",\"make\":\"Ford\"},{\"owner\":\"Jin Soo\",\"color\":\"green\",\"model\":\"Tucson\",\"make\":\"Hyundai\"},{\"owner\":\"Max\",\"color\":\"yellow\",\"model\":\"Passat\",\"make\":\"Volkswagen\"},{\"owner\":\"Adrian\",\"color\":\"black\",\"model\":\"S\",\"make\":\"Tesla\"},{\"owner\":\"Michel\",\"color\":\"purple\",\"model\":\"205\",\"make\":\"Peugeot\"},{\"owner\":\"Aarav\",\"color\":\"white\",\"model\":\"S22L\",\"make\":\"Chery\"},{\"owner\":\"Pari\",\"color\":\"violet\",\"model\":\"Punto\",\"make\":\"Fiat\"},{\"owner\":\"Valeria\",\"color\":\"indigo\",\"model\":\"nano\",\"make\":\"Tata\"},{\"owner\":\"Shotaro\",\"color\":\"brown\",\"model\":\"Barina\",\"make\":\"Holden\"}]"
```

**Tip:** If you want a slightly more readable output format, you can use a shell script to format it. The following command works by piping the output through `cut` to get just the payload output, and then using `jq` for formatting the JSON:

```
peer chaincode invoke -o orderer.example.com:7050 --channelID mychannel --name fabcar -c '{"Args":["queryAllCars"]}' 2>&1 | cut -d: -f6- | jq  'fromjson'
```

Your output should now resemble the following truncated output:
```
root@64a680191017:/opt/gopath/src/github.com/fabcar/java# peer chaincode query -o orderer.example.com:7050 --channelID mychannel --name fabcar -c '{"Args":["queryAllCars"]}' | jq
[
  {
    "owner": "Tomoko",
    "color": "blue",
    "model": "Prius",
    "make": "Toyota"
  },
  {
    "owner": "Brad",
    "color": "red",
    "model": "Mustang",
    "make": "Ford"
  },
  {
    "owner": "Jin Soo",
    "color": "green",
    "model": "Tucson",
    "make": "Hyundai"
  },
......
```


Note that the `peer chaincode query` command returns just the payload. If you want to do the same with a `peer chaincode invoke` command, change the pipe to jq part of the command to `2>&1 | cut -d: -f6- | jq  'fromjson' `


### Update the ledger

We can run some of the other transaction functions to create a new car (`createCar`), and change the owner (`changeCarOwner`). If you encounter any issues,  remember to monitor the second console window:
```
peer chaincode invoke -o orderer.example.com:7050 --channelID mychannel --name fabcar -c '{"Args":["createCar","KITT","Knight Rider","Unique","black","Knight Foundation"]}'
```

And to confirm the car was successfully added to the ledger, we can query for it:
```
peer chaincode invoke -o orderer.example.com:7050 --channelID mychannel --name fabcar -c '{"Args":["queryCar","KITT"]}'  
```
You should see results similar to:
```
2019-10-14 10:48:17.191 UTC [chaincodeCmd] chaincodeInvokeOrQuery -> INFO 001 Chaincode invoke successful. result: status:200 payload:"{\"owner\":\"Knight Foundation\",\"color\":\"black\",\"model\":\"Unique\",\"make\":\"Knight Rider\"}"
```

Finally, we can update the owner of the car we just added from "Knight Foundation" to "Michael" by running the invoke command and passing the `changeCarOwner` method:

```
peer chaincode invoke -o orderer.example.com:7050 --channelID mychannel --name fabcar -c '{"Args":["changeCarOwner","KITT","Michael Knight"]}'  
```
You should see results similar to:
```
2019-10-14 10:50:48.312 UTC [chaincodeCmd] chaincodeInvokeOrQuery -> INFO 001 Chaincode invoke successful. result: status:200 payload:"{\"owner\":\"Michael Knight\",\"color\":\"black\",\"model\":\"Unique\",\"make\":\"Knight Rider\"}"
```

### Debugging a smart contract

To illustrate how the Docker output logs are useful consider the following example. If in the last command, `changeCarOwner` was mistyped as `changeOwner`, the command output would be similar to the following:

```
dev-peer0.org1.example.com-fabcar-1|10:50:37:861 INFO    org.hyperledger.fabric.contract.ContractRouter processRequest                    Got invoke routing request
dev-peer0.org1.example.com-fabcar-1|10:50:37:862 INFO    org.hyperledger.fabric.contract.ContractRouter processRequest                    Got the invoke request for:changeOwner [KITT, Michael Knight]
dev-peer0.org1.example.com-fabcar-1|10:50:37:865 INFO    org.hyperledger.fabric.contract.ContractRouter processRequest                    Got routing:unknownTransaction:org.hyperledger.fabric.samples.fabcar.FabCar
dev-peer0.org1.example.com-fabcar-1|10:50:37:867 SEVERE  org.hyperledger.fabric.Logger error                                              Undefined contract method calledorg.hyperledger.fabric.shim.ChaincodeException: Undefined contract method called
dev-peer0.org1.example.com-fabcar-1|	at org.hyperledger.fabric.contract.ContractInterface.unknownTransaction(ContractInterface.java:74)
dev-peer0.org1.example.com-fabcar-1|	at sun.reflect.NativeMethodAccessorImpl.invoke0(Native Method)
dev-peer0.org1.example.com-fabcar-1|	at sun.reflect.NativeMethodAccessorImpl.invoke(NativeMethodAccessorImpl.java:62)
dev-peer0.org1.example.com-fabcar-1|	at sun.reflect.DelegatingMethodAccessorImpl.invoke(DelegatingMethodAccessorImpl.java:43)
dev-peer0.org1.example.com-fabcar-1|	at java.lang.reflect.Method.invoke(Method.java:498)
dev-peer0.org1.example.com-fabcar-1|	at org.hyperledger.fabric.contract.execution.impl.ContractExecutionService.executeRequest(ContractExecutionService.java:57)
dev-peer0.org1.example.com-fabcar-1|	at org.hyperledger.fabric.contract.ContractRouter.processRequest(ContractRouter.java:87)
dev-peer0.org1.example.com-fabcar-1|	at org.hyperledger.fabric.contract.ContractRouter.invoke(ContractRouter.java:98)
dev-peer0.org1.example.com-fabcar-1|	at org.hyperledger.fabric.shim.impl.Handler.lambda$handleTransaction$1(Handler.java:320)
dev-peer0.org1.example.com-fabcar-1|	at java.lang.Thread.run(Thread.java:748)
dev-peer0.org1.example.com-fabcar-1|
```

After the output `Got the invoke request for:changeOwner [KITT, Michael Knight]`, you can see the error `Undefined contract method called`.

**Tip:** To see a list of the available transactions for a chaincode, you can run the following command to get a detailed JSON document that lists all the contracts available and information about their associated arguments.

```
peer chaincode query -o orderer.example.com:7050 --channelID mychannel --name fabcar -c '{"Args":["org.hyperledger.fabric:GetMetadata"]}'  | jq
```
