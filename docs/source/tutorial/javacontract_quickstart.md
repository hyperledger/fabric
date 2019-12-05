# Quick Start - Java Smart Contract

## Aim and Objectives

**Aim** to get up and working with a Java Smart Contract as quickly as possible.

**Objectives**

- Start up a basic development Fabric network
- Install and instantiate a Java Smart Contract
- Execute transaction functions
- Monitor the running Contract, and demonstrate how to debug a contract that fails to start

## Assumptions

That you have:
 - _git_ installed
 - _docker_ and _docker-compose_ installed
 - you've created a 'working directory' to clone the example repository 
 
You do not need to have Java, Node or the Fabric `peer` commands installed, but for development you will at some point need these tools.

## Steps:

### Get the source 

Clone the `fabric-samples` repository

```bash
git clone git@github.com:hyperledger/fabric-samples.git
```

### Start and monitor Fabric
Change to the `fabric-samples/basic-network`
Start the Fabric Network - brings up the peers, orderers, ca, and couchdb that will be needed

```
./start.sh
```

If you issue `docker ps` this is the expected output:

```
➜  basic-network git:(release-1.4) docker ps
CONTAINER ID        IMAGE                        COMMAND                  CREATED             STATUS              PORTS                                            NAMES
f9cb3ea067da        hyperledger/fabric-peer      "peer node start"        3 minutes ago       Up 3 minutes        0.0.0.0:7051->7051/tcp, 0.0.0.0:7053->7053/tcp   peer0.org1.example.com
92447f306949        hyperledger/fabric-ca        "sh -c 'fabric-ca-se…"   4 minutes ago       Up 3 minutes        0.0.0.0:7054->7054/tcp                           ca.example.com
d0d43f35afb5        hyperledger/fabric-couchdb   "tini -- /docker-ent…"   4 minutes ago       Up 3 minutes        4369/tcp, 9100/tcp, 0.0.0.0:5984->5984/tcp       couchdb
80fdbab89811        hyperledger/fabric-orderer   "orderer"                4 minutes ago       Up 3 minutes        0.0.0.0:7050->7050/tcp                           orderer.example.com
```

These containers are all attached to a docker network, `docker network ls` will list all the docker networks. If you have done other docker work your list of networks may differ, but you are looking for the `net_basic`

```
➜  basic-network git:(release-1.4) docker network ls      
NETWORK ID          NAME                DRIVER              SCOPE
a6c8a8c03f79        bridge              bridge              local
4796b5cc349d        host                host                local
00bc8097f7eb        net_basic           bridge              local
fbd4128b6d8c        none                null                local
```

This next step is not required, but is extremely useful for working out problems with chaincode.
Start another console window, and in the working directory use the following script to start up a `logspout` this will track and display all the docker output. This is useful as some containers are created purely for the purposes of starting chaincode. These may well only exist a short time but their output is very useful for debugging.

There is a `monitordocker.sh` script within the 'commercial-paper' sample, you can run this script from it's original location or copy it to your main working directory.
```
cp ./commercial-paper/organization/digibank/configuration/cli/monitordocker.sh .
# if you're not sure where it is
find . -name monitordocker.sh
```

Run this script in your newly created console, 
```
./monitordocker.sh net_basic
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
This will start to list all the docker containers, you won't see much at first. (it can be helpful to make this window wide, with a smallish font!)


### Run some chaincode!

One of the containers that was started is a cli container. This has all the Fabric cli tools installed and can be a good alternative to installing the tools locally. 
Issue this to get into the container.

```
docker-compose up -d cli   
docker exec -it cli bash
```

In this container, we can change to the directory that been mounted to the example chaincodes.

```
➜  basic-network git:(release-1.4) docker exec -it cli bash
root@64a680191017:/opt/gopath/src/github.com/hyperledger/fabric/peer# cd /opt/gopath/src/github.com
root@64a680191017:/opt/gopath/src/github.com# ls
abac  chaincode_example02  fabcar  hyperledger  marbles02  marbles02_private  sacc
```

We'll use fabcar here for testing..

```
cd fabcar/java
```

First step is to install the chaincode (watch your second window for some output)

```
root@64a680191017:/opt/gopath/src/github.com/fabcar/java# peer chaincode install -n fabcar -v 1 -p $(pwd) -l java
2019-10-14 09:36:21.181 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 001 Using default escc
2019-10-14 09:36:21.181 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 002 Using default vscc
2019-10-14 09:36:21.261 UTC [chaincodeCmd] install -> INFO 003 Installed remotely response:<status:200 payload:"OK" > 
```
The next, and perhaps most important step is to 'instantiate' the chaincode. At this stage, the code is rebuild and put into a running docker container. If there are problems with the code in the chaincode, or something other issue then this is where it might occur. 

You'll start to see output like this in the second window

```
frosty_mclean|Gradle build
frosty_mclean|Starting a Gradle Daemon, 1 incompatible and 1 stopped Daemons could not be reused, use --status for details
frosty_mclean|Download https://plugins.gradle.org/m2/com/github/johnrengelman/shadow/com.github.johnrengelman.shadow.gradle.plugin/2.0.4/com.github.johnrengelman.shadow.gradle.plugin-2.0.4.pom
frosty_mclean|Download https://plugins.gradle.org/m2/com/github/jengelman/gradle/plugins/shadow/2.0.4/shadow-2.0.4.pom
frosty_mclean|Download https://plugins.gradle.org/m2/org/codehaus/groovy/groovy-backports-compat23/2.4.4/groovy-backports-compat23-2.4.4.pom
frosty_mclean|Download https://plugins.gradle.org/m2/com/github/jengelman/gradle/plugins/shadow/2.0.4/shadow-2.0.4.jar
frosty_mclean|Download https://plugins.gradle.org/m2/org/codehaus/groovy/groovy-backports-compat23/2.4.4/groovy-backports-compat23-2.4.4.jar

```

The name of the container is chosen by docker at random.. Eventually you'll see
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

And the chaincode is started, the command in the cli container will have started. If you briefly open another terminal, and issue `docker ps` the following will now be added to the list of containers

```
165b7e9dee60        dev-peer0.org1.example.com-fabcar-1-55704394a74399d4b31ca5022fb02e80783229e680285eedddb37ddbff5c2e79   "/root/chaincode-jav…"   2 minutes ago       Up 2 minutes                                                         dev-peer0.org1.example.com-fabcar-1
```

This is your running chaincode container - with the FabCar Smart Contract deployed within it.

### Execute a transaction function

As this is a test contract, we'll 'bootstrap' some data into the ledger, also this is a good example of issuing a transaction to the contract.

```
root@64a680191017:/opt/gopath/src/github.com/fabcar/java# peer chaincode invoke -o orderer.example.com:7050 --channelID mychannel --name fabcar -c '{"Args":["initLedger"]}'
2019-10-14 09:44:47.138 UTC [chaincodeCmd] chaincodeInvokeOrQuery -> INFO 001 Chaincode invoke successful. result: status:200 
```

Now let's issue some commands to show cars 

```
root@64a680191017:/opt/gopath/src/github.com/fabcar/java# peer chaincode invoke -o orderer.example.com:7050 --channelID mychannel --name fabcar -c '{"Args":["queryAllCars"]}'
2019-10-14 10:19:40.463 UTC [chaincodeCmd] chaincodeInvokeOrQuery -> INFO 001 Chaincode invoke successful. result: status:200 payload:"[{\"owner\":\"Tomoko\",\"color\":\"blue\",\"model\":\"Prius\",\"make\":\"Toyota\"},{\"owner\":\"Brad\",\"color\":\"red\",\"model\":\"Mustang\",\"make\":\"Ford\"},{\"owner\":\"Jin Soo\",\"color\":\"green\",\"model\":\"Tucson\",\"make\":\"Hyundai\"},{\"owner\":\"Max\",\"color\":\"yellow\",\"model\":\"Passat\",\"make\":\"Volkswagen\"},{\"owner\":\"Adrian\",\"color\":\"black\",\"model\":\"S\",\"make\":\"Tesla\"},{\"owner\":\"Michel\",\"color\":\"purple\",\"model\":\"205\",\"make\":\"Peugeot\"},{\"owner\":\"Aarav\",\"color\":\"white\",\"model\":\"S22L\",\"make\":\"Chery\"},{\"owner\":\"Pari\",\"color\":\"violet\",\"model\":\"Punto\",\"make\":\"Fiat\"},{\"owner\":\"Valeria\",\"color\":\"indigo\",\"model\":\"nano\",\"make\":\"Tata\"},{\"owner\":\"Shotaro\",\"color\":\"brown\",\"model\":\"Barina\",\"make\":\"Holden\"}]" 
```

[Aside if you want a slightly more readable output format, a little shell script works wonders.

```
peer chaincode invoke -o orderer.example.com:7050 --channelID mychannel --name fabcar -c '{"Args":["queryAllCars"]}' 2>&1 | cut -d: -f6- | jq  'fromjson'
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
I've cut off the output  to save space. This works by piping through `cut` to just get the payload output, and use jq for formatting

Note that the `peer chaincode query` returns just payload.. if you want to do the same with an `peer chaincode invoke` you can but change to pipe to jq part of the command to `2>&1 | cut -d: -f6- | jq  'fromjson' `

]

### Execute more transaction functions

Let's run some of the other transaction functions to create a new car, and change the owner. (if you have any problems remember to keep a watch on the second console window)
```
peer chaincode invoke -o orderer.example.com:7050 --channelID mychannel --name fabcar -c '{"Args":["createCar","KITT","Knight Rider","Unique","black","Knight Foundation"]}'
```

And to retrieve the details:
```
peer chaincode invoke -o orderer.example.com:7050 --channelID mychannel --name fabcar -c '{"Args":["queryCar","KITT"]}'  
2019-10-14 10:48:17.191 UTC [chaincodeCmd] chaincodeInvokeOrQuery -> INFO 001 Chaincode invoke successful. result: status:200 payload:"{\"owner\":\"Knight Foundation\",\"color\":\"black\",\"model\":\"Unique\",\"make\":\"Knight Rider\"}" 
```

Finally to update the owner.

```
peer chaincode invoke -o orderer.example.com:7050 --channelID mychannel --name fabcar -c '{"Args":["changeCarOwner","KITT","Michael Knight"]}'  
2019-10-14 10:50:48.312 UTC [chaincodeCmd] chaincodeInvokeOrQuery -> INFO 001 Chaincode invoke successful. result: status:200 payload:"{\"owner\":\"Michael Knight\",\"color\":\"black\",\"model\":\"Unique\",\"make\":\"Knight Rider\"}" 
```

### Debugging a failing contract

As an example of how the log of the docker output is useful, say in the last command, `changeCarOwner` is mistyped as `changeOwner` the following output will be in the console.

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

You can see the `Undefined contract method called` and just above `changeOwner [KITT, Michael Knight]`

But how to know what transactions are available.

If you issue 

```
peer chaincode query -o orderer.example.com:7050 --channelID mychannel --name fabcar -c '{"Args":["org.hyperledger.fabric:GetMetadata"]}'  | jq
```

You'll get a detailed JSON document listing all the contracts available and information about their arguments.

