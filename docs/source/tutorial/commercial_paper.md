# Commercial paper tutorial

**Audience:** Architects, application and smart contract developers,
administrators

This tutorial will show you how to install and use a commercial paper sample
application and smart contract. It is a task-oriented topic, so it emphasizes
procedures above concepts. When you’d like to understand the concepts in more
detail, you can read the
[Developing Applications](../developapps/developing_applications.html) topic.

![commercialpaper.tutorial](./commercial_paper.diagram.1.png) *In this tutorial
two organizations, MagnetoCorp and DigiBank, trade commercial paper with each
other using PaperNet, a Hyperledger Fabric blockchain network.*

Once you've set up the test network, you'll act as Isabella, an employee of
MagnetoCorp, who will issue a commercial paper on its behalf. You'll then switch
roles to take the role of Balaji, an employee of DigiBank, who will buy this
commercial paper, hold it for a period of time, and then redeem it with
MagnetoCorp for a small profit.

You'll act as an developer, end user, and administrator, each in different
organizations, performing the following steps designed to help you understand
what it's like to collaborate as two different organizations working
independently, but according to mutually agreed rules in a Hyperledger Fabric
network.

* [Set up machine](#prerequisites) and [download samples](#download-samples)
* [Create the network](#create-the-network)
* [Examine the commercial paper smart contract](#examine-the-commercial-paper-smart-contract)
* [Deploy the smart contract to the channel](#deploy-the-smart-contract-to-the-channel)
  by approving the chaincode definition as MagnetoCorp and Digibank.
* Understand the structure of a MagnetoCorp [application](#application-structure),
  including its [dependencies](#application-dependencies)
* Configure and use a [wallet and identities](#wallet)
* Run a MagnetoCorp application to [issue a commercial paper](#issue-application)
* Understand how DigiBank uses the smart contract in their [applications](#digibank-applications)
* As Digibank, run applications that
  [buy](#buy-application) and [redeem](#redeem-application) commercial paper

This tutorial has been tested on MacOS and Ubuntu, and should work on other
Linux distributions. A Windows version is under development.

## Prerequisites

Before you start, you must install some prerequisite technology required by the
tutorial. We've kept these to a minimum so that you can get going quickly.

You **must** have the following technologies installed:

  * [**Node**](https://nodejs.org/en/about/) version 8.9.0, or higher. Node is
    a JavaScript runtime that you can use to run applications and smart
    contracts. You are recommended to use the LTS (Long Term Support) version
    of node. Install node [here](https://nodejs.org/en/).


  * [**Docker**](https://www.docker.com/get-started) version 18.06, or higher.
    Docker help developers and administrators create standard environments for
    building and running applications and smart contracts. Hyperledger Fabric is
    provided as a set of Docker images, and the PaperNet smart contract will run
    in a Docker container. Install Docker
    [here](https://www.docker.com/get-started).

You **will** find it helpful to install the following technologies:

  * A source code editor, such as
    [**Visual Studio Code**](https://code.visualstudio.com/) version 1.28, or
    higher. VS Code will help you develop and test your application and smart
    contract. Install VS Code [here](https://code.visualstudio.com/Download).

    Many excellent code editors are available including
    [Atom](https://atom.io/), [Sublime Text](http://www.sublimetext.com/) and
    [Brackets](http://www.sublimetext.com/).

You **may** find it helpful to install the following technologies as you become
more experienced with application and smart contract development. There's no
requirement to install these when you first run the tutorial:

  * [**Node Version Manager**](https://github.com/creationix/nvm). NVM helps you
    easily switch between different versions of node -- it can be really helpful
    if you're working on multiple projects at the same time. Install NVM
    [here](https://github.com/creationix/nvm#installation).

## Download samples

The commercial paper tutorial is one of the samples in the `fabric-samples`
directory. Before you begin this tutorial, ensure that you have followed the
instructions to install the Fabric [Prerequisites](../prereqs.html) and
[Download the Samples, Binaries and Docker Images](../install.html).
When you are finished, you will have cloned the `fabric-samples` repository that
contains the tutorial scripts, smart contract, and application files.

![commercialpaper.download](./commercial_paper.diagram.2.png) *Download the
`fabric-samples` GitHub repository to your local machine.*

After downloading, feel free to examine the directory structure of `fabric-samples`:

```
$ cd fabric-samples
$ ls

CODEOWNERS            basic-network			    first-network
CODE_OF_CONDUCT.md	  chaincode			        high-throughput
CONTRIBUTING.md	      chaincode-docker-devmode  interest_rate_swaps
Jenkinsfile           ci                        off_chain_data
LICENSE               ci.properties             scripts
MAINTAINERS.md        commercial-paper          test-network
README.md             docs
SECURITY.md           fabcar
```

Notice the `commercial-paper` directory -- that's where our sample is located!

You've now completed the first stage of the tutorial! As you proceed, you'll
open multiple command windows for different users and components. For example:

* To show peer, orderer and CA log output from your network.
* To approve the chaincode as an administrator from MagnetoCorp and as an
  administrator from DigiBank.
* To run applications on behalf of Isabella and Balaji, who will use the smart
  contract to trade commercial paper with each other.

We'll make it clear when you should run a command from particular command
window; for example:

```
(isabella)$ ls
```

indicates that you should run the `ls` command from Isabella's window.

## Create the network

This tutorial will deploy a smart contract using the Fabric test network.
The test network consists of two peer organizations and an ordering organization.
The two peer organizations operate one peer each, while the ordering organization
operates a single node raft ordering service. We will also use the test network
to create a single channel named `mychannel` that both peer organizations will
be members of.

![commercialpaper.network](./commercial_paper.diagram.testnet.png)
*The Fabric test network is comprised of two peer organizations, Org1 and Org2,
each with one peer and its ledger database, an ordering node. Each of these
components runs as a Docker container.*

The two peers, the peer [ledgers](../ledger/ledger.html#world-state-database-options), the
orderer and the CA each run in the their own Docker container. In production
environments, organizations typically use existing CAs that are shared with
other systems; they're not dedicated to the Fabric network.

The two organizations of the test network allow us to interact with a blockchain
ledger as two organizations that operate separate peers. In this tutorial,
we will operate Org1 of the test network as DigiBank and Org2 as MagnetoCorp.

You can start the test network and create the channel with a script provided in
the commercial paper directory. Change to the `commercial-paper` directory in
the `fabric-samples`:
```
cd fabric-samples/commercial-paper
```
Then use the script to start the test network:
```
./network-starter.sh
```
If the command is successful, you will see the test network being created in your
logs. You can use the `docker ps` command to see the Fabric nodes running on your
local machine:
```
$ docker ps

CONTAINER ID        IMAGE                               COMMAND                  CREATED             STATUS              PORTS                                        NAMES
321cc489b10f        hyperledger/fabric-peer:latest      "peer node start"        2 minutes ago       Up 2 minutes        0.0.0.0:7051->7051/tcp                       peer0.org1.example.com
ad668671f95f        hyperledger/fabric-peer:latest      "peer node start"        2 minutes ago       Up 2 minutes        7051/tcp, 0.0.0.0:9051->9051/tcp             peer0.org2.example.com
caadbe4d8592        hyperledger/fabric-couchdb          "tini -- /docker-ent…"   2 minutes ago       Up 2 minutes        4369/tcp, 9100/tcp, 0.0.0.0:7984->5984/tcp   couchdb1
ebabe52903b8        hyperledger/fabric-couchdb          "tini -- /docker-ent…"   2 minutes ago       Up 2 minutes        4369/tcp, 9100/tcp, 0.0.0.0:5984->5984/tcp   couchdb0
7c72711c6e18        hyperledger/fabric-orderer:latest   "orderer"                2 minutes ago       Up 2 minutes        0.0.0.0:7050->7050/tcp                       orderer.example.com
```

See if you can map these containers to the nodes of the test network (you may
need to horizontally scroll to locate the information):

* The Org1 peer, `peer0.org1.example.com`, is running in container `321cc489b10f`
* The Org2 peer, `peer0.org2.example.com`, is running in container `ad668671f95f`
* The CouchDB database for the Org1 peer, `couchdb0`, is running in container `ebabe52903b8`
* The CouchDB database for the Org2 peer, `couchdb1`, is running in container `caadbe4d8592`
* The ordering node `orderer.example.com` is running in container `7c72711c6e18`

These containers all form a [Docker network](https://docs.docker.com/network/)
called `net_test`. You can view the network with the `docker network` command:

```
$ docker network inspect net_test

    {
          "Name": "net_test",
          "Id": "b77b99d29e37677fac48b7ecd78383bdebf09ebdd6b00e87e3d9444252b1ce31",
          "Created": "2020-01-30T23:04:39.6157465Z",
          "Containers": {
              "321cc489b10ff46554d0b215da307d38daf35b68bbea635ae0ae3176c3ae0945": {
                  "Name": "peer0.org1.example.com",
                  "IPv4Address": "192.168.224.5/20",
              },
              "7c72711c6e18caf7bff4cf78c27efc9ef3b2359a749c926c8aba1beacfdb0211": {
                  "Name": "orderer.example.com",
                  "IPv4Address": "192.168.224.4/20",
              },
              "ad668671f95f351f0119320198e1d1e19ebbb0d75766c6c8b9bb7bd36ba506af": {
                  "Name": "peer0.org2.example.com",
                  "IPv4Address": "192.168.224.6/20",
              },
              "caadbe4d8592aa558fe14d07a424a9e04365620ede1143b6ce5902ce038c0851": {
                  "Name": "couchdb1",
                  "IPv4Address": "192.168.224.2/20",
              },
              "ebabe52903b8597d016dbc0d0ca4373ef75162d3400efbe6416975abafd08a8f": {
                  "Name": "couchdb0",
                  "IPv4Address": "192.168.224.3/20",
              }
          },
          "Labels": {}
      }
  ]

```
See how the five containers use different IP addresses, while being part of a
single Docker network. (We've abbreviated the output for clarity.)

Because we are operating the test network as DigiBank and MagnetoCorp,
`peer0.org1.example.com` will belong to the DigiBank organization while
`peer0.org2.example.com` will be operated by MagnetoCorp. Now that the test
network is up and running, we can refer our network as PaperNet from this point
forward.

To recap: you've downloaded the Hyperledger Fabric samples repository from
GitHub and you've got the test network running on your local machine. Let's now
start to play the role of MagnetoCorp, who wish to trade commercial paper.

## Monitor the network as MagnetoCorp

The commercial paper tutorial allows you to act as two organizations by
providing two separate folders for DigiBank and MagnetoCorp. The two folders
contain the smart contracts and application files for each organization. Because
the two organizations have different roles in the trading of the commercial paper,
the application files are different for each organization. Open a new window in
the `fabric-samples` repository and use the following command to change into
the MagnetoCorp directory:
```
cd commercial-paper/organization/magnetocorp
```
The first thing we are going to do as MagnetoCorp is monitor the components
of PaperNet. An administrator can view the aggregated output from a set
of Docker containers using the `logspout` [tool](https://github.com/gliderlabs/logspout#logspout).
The tool collects the different output streams into one place, making it easy
to see what's happening from a single window. This can be really helpful for
administrators when installing smart contracts or for developers when invoking
smart contracts, for example.

In the MagnetoCorp directory, run the following command to run the
`monitordocker.sh`  script and start the `logspout` tool for the containers
associated with PaperNet running on `net_test`:
```
(magnetocorp admin)$ ./configuration/cli/monitordocker.sh net_test
...
latest: Pulling from gliderlabs/logspout
4fe2ade4980c: Pull complete
decca452f519: Pull complete
(...)
Starting monitoring on all containers on the network net_test
b7f3586e5d0233de5a454df369b8eadab0613886fc9877529587345fc01a3582
```

Note that you can pass a port number to the above command if the default port in `monitordocker.sh` is already in use.
```
(magnetocorp admin)$ ./monitordocker.sh net_test <port_number>
```

This window will now show output from the Docker containers for the remainder of the
tutorial, so go ahead and open another command window. The next thing we will do is
examine the smart contract that MagnetoCorp will use to issue to the commercial
paper.

## Examine the commercial paper smart contract

`issue`, `buy` and `redeem` are the three functions at the heart of the commercial
paper smart contract. It is used by applications to submit transactions which
correspondingly issue, buy and redeem commercial paper on the ledger. Our next
task is to examine this smart contract.

Open a new terminal in the `fabric-samples` directory and change into the
MagnetoCorp folder to act as the MagnetoCorp developer.
```
cd commercial-paper/organization/magnetocorp
```
You can then view the smart contract in the `contract` directory using your chosen
editor (VS Code in this tutorial):
```
(magnetocorp developer)$ code contract
```

In the `lib` directory of the folder, you'll see `papercontract.js` file -- this
contains the commercial paper smart contract!

![commercialpaper.vscode1](./commercial_paper.vscode.papercontract.png) *An
example code editor displaying the commercial paper smart contract in `papercontract.js`*

`papercontract.js` is a JavaScript program designed to run in the Node.js
environment. Note the following key program lines:

* `const { Contract, Context } = require('fabric-contract-api');`

  This statement brings into scope two key Hyperledger Fabric classes that will
  be used extensively by the smart contract  -- `Contract` and `Context`. You
  can learn more about these classes in the
  [`fabric-shim` JSDOCS](https://hyperledger.github.io/fabric-chaincode-node/).


* `class CommercialPaperContract extends Contract {`

  This defines the smart contract class `CommercialPaperContract` based on the
  built-in Fabric `Contract` class.  The methods which implement the key
  transactions to `issue`, `buy` and `redeem` commercial paper are defined
  within this class.


* `async issue(ctx, issuer, paperNumber, issueDateTime, maturityDateTime...) {`

  This method defines the commercial paper `issue` transaction for PaperNet. The
  parameters that are passed to this method will be used to create the new
  commercial paper.

  Locate and examine the `buy` and `redeem` transactions within the smart
  contract.


* `let paper = CommercialPaper.createInstance(issuer, paperNumber, issueDateTime...);`

  Within the `issue` transaction, this statement creates a new commercial paper
  in memory using the `CommercialPaper` class with the supplied transaction
  inputs. Examine the `buy` and `redeem` transactions to see how they similarly
  use this class.


* `await ctx.paperList.addPaper(paper);`

  This statement adds the new commercial paper to the ledger using
  `ctx.paperList`, an instance of a `PaperList` class that was created when the
  smart contract context `CommercialPaperContext` was initialized. Again,
  examine the `buy` and `redeem` methods to see how they use this class.


* `return paper;`

  This statement returns a binary buffer as response from the `issue`
  transaction for processing by the caller of the smart contract.


Feel free to examine other files in the `contract` directory to understand how
the smart contract works, and read in detail how `papercontract.js` is
designed in the smart contract [topic](../developapps/smartcontract.html).

## Deploy the smart contract to the channel

Before `papercontract` can be invoked by applications, it must be installed onto
the appropriate peer nodes of the test network and then defined on the channel
using the [Fabric chaincode lifecycle](../chaincode_lifecycle.html#chaincode-lifecycle). The Fabric chaincode
lifecycle allows multiple organizations to agree to the parameters of a chaincode
before the chainocde is deployed to a channel. As a result, we need to install
and approve the chaincode as administrators of both MagnetoCorp and DigiBank.

![commercialpaper.install](./commercial_paper.diagram.install.png)  *A MagnetoCorp
administrator installs a copy of the `papercontract` onto a MagnetoCorp peer.*

Smart contracts are the focus of application development, and are contained
within a Hyperledger Fabric artifact called [chaincode](../chaincode.html). One
or more smart contracts can be defined within a single chaincode, and installing
a chaincode will allow them to be consumed by the different organizations in
PaperNet. It means that only administrators need to worry about chaincode;
everyone else can think in terms of smart contracts.

### Install and approve the smart contract as MagnetoCorp

We will first install and approve the smart contract as the MagnetoCorp admin. Make
sure that you are operating from the `magnetocorp` folder, or navigate back to that
folder using the following command:
```
cd commercial-paper/organization/magnetocorp
```

A MagnetoCorp administrator can interact with PaperNet using the `peer` CLI. However,
the administrator needs to set certain environment variables in their command
window to use the correct set of `peer` binaries, send commands to the address
of the MagnetoCorp peer, and sign requests with the correct crypto material.

You can use a script provided by the sample to set the environment variables in
your command window. Run the following command in the `magnetocorp` directory:
```
source magnetocorp.sh
```

You will see the full list of environment variables printed in your window. We
can now use this command window to interact with PaperNet as the MagnetoCorp
administrator.

The first step is to install the `papercontract` smart contract. The smart
contract can be packaged into a chaincode using the
`peer lifecycle chaincode package` command. In the MagnetoCorp administrator's
command window, run the following command to create the chaincode package:
```
(magnetocorp admin)$ peer lifecycle chaincode package cp.tar.gz --lang node --path ./contract --label cp_0
```
The MagnetoCorp admin can now install the chaincode on the MagnetoCorp peer using
the `peer lifecycle chaincode install` command:
```
(magnetocorp admin)$ peer lifecycle chaincode install cp.tar.gz
```
If the command is successful, you will see messages similar to the following
printed in your terminal:
```
2020-01-30 18:32:33.762 EST [cli.lifecycle.chaincode] submitInstallProposal -> INFO 001 Installed remotely: response:<status:200 payload:"\nEcp_0:ffda93e26b183e231b7e9d5051e1ee7ca47fbf24f00a8376ec54120b1a2a335c\022\004cp_0" >
2020-01-30 18:32:33.762 EST [cli.lifecycle.chaincode] submitInstallProposal -> INFO 002 Chaincode code package identifier: cp_0:ffda93e26b183e231b7e9d5051e1ee7ca47fbf24f00a8376ec54120b1a2a335c
```
Because the MagnetoCorp admin has set `CORE_PEER_ADDRESS=localhost:9051` to
target its commands to `peer0.org2.example.com`, the `INFO 001 Installed remotely...`
indicates that `papercontract` has been successfully installed on this peer.

After we install the smart contract, we need to approve the chaincode definition
for `papercontract` as MagnetoCorp. The first step is to find the packageID of
the chaincode we installed on our peer. We can query the packageID using the
`peer lifecycle chaincode queryinstalled` command:
```
peer lifecycle chaincode queryinstalled
```

The command will return the same package identifier as the install command. You
should see output similar to the following:
```
Installed chaincodes on peer:
Package ID: cp_0:ffda93e26b183e231b7e9d5051e1ee7ca47fbf24f00a8376ec54120b1a2a335c, Label: cp_0
```

We will need the package ID in the next step, so we will save it as an environment
variable. The package ID may not be the same for all users, so you need to
complete this step using the package ID returned from your command window.
```
export PACKAGE_ID=cp_0:ffda93e26b183e231b7e9d5051e1ee7ca47fbf24f00a8376ec54120b1a2a335c
```

The admin can now approve the chaincode definition for MagnetoCorp using the
`peer lifecycle chaincode approveformyorg` command:
```
(magnetocorp admin)$ peer lifecycle chaincode approveformyorg --orderer localhost:7050 --ordererTLSHostnameOverride orderer.example.com --channelID mychannel --name papercontract -v 0 --package-id $PACKAGE_ID --sequence 1 --tls --cafile $ORDERER_CA
```

One of the most important chaincode parameters that channel members need to
agree to using the chaincode definition is the chaincode [endorsement policy](../endorsement-policies.html).
The endorsement policy describes the set of organizations that must endorse
(execute and sign) a transaction before it can be determined to be valid. By
approving the `papercontract` chaincode without the ``--policy`` flag, the
MagnetoCorp admin agrees to using the default endorsement policy, which requires a
majority of organizations on the channel to endorse a transaction. All transactions,
whether valid or invalid, will be recorded on the [ledger blockchain](../ledger/ledger.html#blockchain),
but only valid transactions will update the [world state](../ledger/ledger.html#world-state).

### Install and approve the smart contract as DigiBank

By default, the Fabric Chaincode lifecycle requires a majority of organizations
on the channel to successfully commit the chaincode definition to the channel.
This implies that we need to approve the `papernet` chaincode as both MagnetoCorp
and DigiBank to get the required majority of 2 out of 2. Open a new terminal
window in the `fabric-samples` and navigate to the older that contains the
DigiBank smart contract and application files:
```
(digibank admin)$ cd commercial-paper/organization/digibank/
```
Use the script in the DigiBank folder to set the environment variables that will
allow you to act as the DigiBank admin:
```
source digibank.sh
```

We can now install and approve `papercontract` as the DigiBank. Run the following
command to package the chaincode:
```
(digibank admin)$ peer lifecycle chaincode package cp.tar.gz --lang node --path ./contract --label cp_0
```
The admin can now install the chaincode on the DigiBank peer:
```
(digibank admin)$ peer lifecycle chaincode install cp.tar.gz
```
We then need to query and save the packageID of the chaincode that was just
installed:
```
(digibank admin)$ peer lifecycle chaincode queryinstalled
```
Save the package ID as an environment variable. Complete this step using the
package ID returned from your console.
```
export PACKAGE_ID=cp_0:ffda93e26b183e231b7e9d5051e1ee7ca47fbf24f00a8376ec54120b1a2a335c
```

The Digibank admin can now approve the chaincode definition of `papercontract`:
```
(digibank admin)$ peer lifecycle chaincode approveformyorg --orderer localhost:7050 --ordererTLSHostnameOverride orderer.example.com --channelID mychannel --name papercontract -v 0 --package-id $PACKAGE_ID --sequence 1 --tls --cafile $ORDERER_CA
```

### Commit the chaincode definition to the channel

Now that DigiBank and MagnetoCorp have both approved the `papernet` chaincode, we
have the majority we need (2 out of 2) to commit the chaincode definition to the
channel. Once the chaincode is successfully defined on the channel, the
`CommercialPaper` smart contract inside the `papercontract` chaincode can be
invoked by client applications on the channel. Since either organization can
commit the chaincode to the channel, we will continue operating as the
DigiBank admin:

![commercialpaper.commit](./commercial_paper.diagram.commit.png)  *After the DigiBank administrator commits the definition of the `papercontract` chaincode to the channel, a new Docker chaincode container will be created to run `papercontract` on both PaperNet peers*

The DigiBank administrator uses the `peer lifecycle chaincode commit` command
to commit the chaincode definition of `papercontract` to `mychannel`:
```
(digibank admin)$ peer lifecycle chaincode commit -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --peerAddresses localhost:7051 --tlsRootCertFiles ${PEER0_ORG1_CA} --peerAddresses localhost:9051 --tlsRootCertFiles ${PEER0_ORG2_CA} --channelID mychannel --name papercontract -v 0 --sequence 1 --tls --cafile $ORDERER_CA --waitForEvent
```
The chaincode container will start after the chaincode definition has been
committed to the channel. You can use the `docker ps` command to see
`papercontract` container starting on both peers.

```
(digibank admin)$ docker ps

CONTAINER ID        IMAGE                                                                                                                                                               COMMAND                  CREATED             STATUS              PORTS                                        NAMES
d4ba9dc9c55f        dev-peer0.org1.example.com-cp_0-ebef35e7f1f25eea1dcc6fcad5019477cd7f434c6a5dcaf4e81744e282903535-05cf67c20543ee1c24cf7dfe74abce99785374db15b3bc1de2da372700c25608   "docker-entrypoint.s…"   30 seconds ago      Up 28 seconds                                                    dev-peer0.org1.example.com-cp_0-ebef35e7f1f25eea1dcc6fcad5019477cd7f434c6a5dcaf4e81744e282903535
a944c0f8b6d6        dev-peer0.org2.example.com-cp_0-1487670371e56d107b5e980ce7f66172c89251ab21d484c7f988c02912ddeaec-1a147b6fd2a8bd2ae12db824fad8d08a811c30cc70bc5b6bc49a2cbebc2e71ee   "docker-entrypoint.s…"   31 seconds ago      Up 28 seconds                                                    dev-peer0.org2.example.com-cp_0-1487670371e56d107b5e980ce7f66172c89251ab21d484c7f988c02912ddeaec
```

Notice that the containers are named to indicate the peer that started it, and
the fact that it's running `papercontract` version `0`.

Now that we have deployed the `papercontract` chaincode to the channel, we can
use the MagnetoCorp application to issue the commercial paper. Let's take a
moment to examine the application structure.

## Application structure

The smart contract contained in `papercontract` is called by MagnetoCorp's
application `issue.js`. Isabella uses this application to submit a transaction
to the ledger which issues commercial paper `00001`. Let's quickly examine how
the `issue` application works.

![commercialpaper.application](./commercial_paper.diagram.8.png) *A gateway
allows an application to focus on transaction generation, submission and
response. It coordinates transaction proposal, ordering and notification
processing between the different network components.*

Because the `issue` application submits transactions on behalf of Isabella, it
starts by retrieving Isabella's X.509 certificate from her
[wallet](../developapps/wallet.html), which might be stored on the local file
system or a Hardware Security Module
[HSM](https://en.wikipedia.org/wiki/Hardware_security_module). The `issue`
application is then able to utilize the gateway to submit transactions on the
channel. The Hyperledger Fabric SDK provides a
[gateway](../developapps/gateway.html) abstraction so that applications can
focus on application logic while delegating network interaction to the
gateway. Gateways and wallets make it straightforward to write Hyperledger
Fabric applications.

So let's examine the `issue` application that Isabella is going to use. Open a
separate terminal window for her, and in `fabric-samples` locate the MagnetoCorp
`/application` folder:

```
(isabella)$ cd commercial-paper/organization/magnetocorp/application/
(isabella)$ ls

addToWallet.js		issue.js		package.json
```

`addToWallet.js` is the program that Isabella is going to use to load her
identity into her wallet, and `issue.js` will use this identity to create
commercial paper `00001` on behalf of MagnetoCorp by invoking `papercontract`.

Change to the directory that contains MagnetoCorp's copy of the application
`issue.js`, and use your code editor to examine it:

```
(isabella)$ cd commercial-paper/organization/magnetocorp/application
(isabella)$ code issue.js
```

Examine this directory; it contains the issue application and all its
dependencies.

![commercialpaper.vscode2](./commercial_paper.vscode.issue.png) *A code editor
displaying the contents of the commercial paper application directory.*

Note the following key program lines in `issue.js`:

* `const { Wallets, Gateway } = require('fabric-network');`

  This statement brings two key Hyperledger Fabric SDK classes into scope --
  `Wallet` and `Gateway`.


* `const wallet = await Wallets.newFileSystemWallet('../identity/user/isabella/wallet');`

  This statement identifies that the application will use `isabella` wallet when
  it connects to the blockchain network channel. Because Isabella's X.509 certificate
  is in the local file system, the application creates a new `FileSystemWallet`. The
  application will select a particular identity within `isabella` wallet.


* `await gateway.connect(connectionProfile, connectionOptions);`

  This line of code connects to the network using the gateway identified by
  `connectionProfile`, using the identity referred to in `ConnectionOptions`.

  See how `../gateway/networkConnection.yaml` and `User1@org1.example.com` are
  used for these values respectively.


* `const network = await gateway.getNetwork('mychannel');`

  This connects the application to the network channel `mychannel`, where the
  `papercontract` was previously instantiated.


* `const contract = await network.getContract('papercontract');`

  This statement gives the application access to the `papercontract` chaincode.
  Once an application has issued getContract, it can submit to any smart contract
  transaction implemented within the chaincode.

* `const issueResponse = await contract.submitTransaction('issue', 'MagnetoCorp', '00001', ...);`

  This line of code submits the a transaction to the network using the `issue`
  transaction defined within the smart contract. `MagnetoCorp`, `00001`... are
  the values to be used by the `issue` transaction to create a new commercial
  paper.

* `let paper = CommercialPaper.fromBuffer(issueResponse);`

  This statement processes the response from the `issue` transaction. The
  response needs to deserialized from a buffer into `paper`, a `CommercialPaper`
  object which can interpreted correctly by the application.


Feel free to examine other files in the `/application` directory to understand
how `issue.js` works, and read in detail how it is implemented in the
application [topic](../developapps/application.html).

## Application dependencies

The `issue.js` application is written in JavaScript and designed to run in the
Node.js environment that acts as a client to the PaperNet network.
As is common practice, MagnetoCorp's application is built on many
external node packages --- to improve quality and speed of development. Consider
how `issue.js` includes the `js-yaml`
[package](https://www.npmjs.com/package/js-yaml) to process the YAML gateway
connection profile, or the `fabric-network`
[package](https://www.npmjs.com/package/fabric-network) to access the `Gateway`
and `Wallet` classes:

```JavaScript
const yaml = require('js-yaml');
const { Wallets, Gateway } = require('fabric-network');
```

These packages have to be downloaded from [npm](https://www.npmjs.com/) to the
local file system using the `npm install` command. By convention, packages must
be installed into an application-relative `/node_modules` directory for use at
runtime.

Examine the `package.json` file to see how `issue.js` identifies the packages to
download and their exact versions:

```json
  "dependencies": {
    "fabric-network": "~1.4.0",
    "fabric-client": "~1.4.0",
    "js-yaml": "^3.12.0"
  },
```

**npm** versioning is very powerful; you can read more about it
[here](https://docs.npmjs.com/getting-started/semantic-versioning).

Let's install these packages with the `npm install` command -- this may take up
to a minute to complete:

```
(isabella)$ cd commercial-paper/organization/magnetocorp/application/
(isabella)$ npm install

(           ) extract:lodash: sill extract ansi-styles@3.2.1
(...)
added 738 packages in 46.701s
```

See how this command has updated the directory:

```
(isabella)$ ls

addToWallet.js		node_modules	      	package.json
issue.js	      	package-lock.json
```

Examine the `node_modules` directory to see the packages that have been
installed. There are lots, because `js-yaml` and `fabric-network` are themselves
built on other npm packages! Helpfully, the `package-lock.json`
[file](https://docs.npmjs.com/files/package-lock.json) identifies the exact
versions installed, which can prove invaluable if you want to exactly reproduce
environments; to test, diagnose problems or deliver proven applications for
example.

## Wallet

Isabella is almost ready to run `issue.js` to issue MagnetoCorp commercial paper
`00001`; there's just one remaining task to perform! As `issue.js` acts on
behalf of Isabella, and therefore MagnetoCorp, it will use identity from her
[wallet](../developapps/wallet.html) that reflects these facts. We now need to
perform this one-time activity of adding appropriate X.509 credentials to her
wallet.

In Isabella's terminal window, run the `addToWallet.js` program to add identity
information to her wallet:

```
(isabella)$ node addToWallet.js

done
```

`addToWallet.js` is a simple file-copying program which you can examine at your
leisure. It moves an identity from the test network sample to Isabella's
wallet. Let's focus on the result of this program --- the contents of
the wallet which will be used to submit transactions to PaperNet:

```
(isabella)$ ls ../identity/user/isabella/wallet/

isabella.id
```

Isabella can store multiple identities in her wallet, though in our example, she
only uses one. The `wallet` folder contains an `isabella.id` file that provides
the information that Isabella needs to connect to the network. Other identities
used by Isabella would have their own file. You can open this file to see the
identity information that `issue.js` will use on behalf of Isabella inside a JSON
file. The output has been formatted for clarity.
```
(isabella)$  cat ../identity/user/isabella/wallet/*

{
  "credentials": {
    "certificate": "-----BEGIN CERTIFICATE-----\nMIICKTCCAdCgAwIBAgIQWKwvLG+sqeO3LwwQK6avZDAKBggqhkjOPQQDAjBzMQsw\nCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMNU2FuIEZy\nYW5jaXNjbzEZMBcGA1UEChMQb3JnMi5leGFtcGxlLmNvbTEcMBoGA1UEAxMTY2Eu\nb3JnMi5leGFtcGxlLmNvbTAeFw0yMDAyMDQxOTA5MDBaFw0zMDAyMDExOTA5MDBa\nMGwxCzAJBgNVBAYTAlVTMRMwEQYDVQQIEwpDYWxpZm9ybmlhMRYwFAYDVQQHEw1T\nYW4gRnJhbmNpc2NvMQ8wDQYDVQQLEwZjbGllbnQxHzAdBgNVBAMMFlVzZXIxQG9y\nZzIuZXhhbXBsZS5jb20wWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAAT4TnTblx0k\ngfqX+NN7F76Me33VTq3K2NUWZRreoJzq6bAuvdDR+iFvVPKXbdORnVvRSATcXsYl\nt20yU7n/53dbo00wSzAOBgNVHQ8BAf8EBAMCB4AwDAYDVR0TAQH/BAIwADArBgNV\nHSMEJDAigCDOCdm4irsZFU3D6Hak4+84QRg1N43iwg8w1V6DRhgLyDAKBggqhkjO\nPQQDAgNHADBEAiBhzKix1KJcbUy9ey5ulWHRUMbqdVCNHe/mRtUdaJagIgIgYpbZ\nXf0CSiTXIWOJIsswN4Jp+ZxkJfFVmXndqKqz+VM=\n-----END CERTIFICATE-----\n",
    "privateKey": "-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQggs55vQg2oXi8gNi8\nNidE8Fy5zenohArDq3FGJD8cKU2hRANCAAT4TnTblx0kgfqX+NN7F76Me33VTq3K\n2NUWZRreoJzq6bAuvdDR+iFvVPKXbdORnVvRSATcXsYlt20yU7n/53db\n-----END PRIVATE KEY-----\n"
  },
  "mspId": "Org2MSP",
  "type": "X.509",
  "version": 1
}
```

In the file you can notice the following:

* a `"privateKey":` used to sign transactions on Isabella's behalf, but not
  distributed outside of her immediate control.

* a `"certificate":` which contains Isabella's public key and other X.509
  attributes added by the Certificate Authority at certificate creation. This
  certificate is distributed to the network so that different actors at different
  times can cryptographically verify information created by Isabella's private key.

You can Learn more about certificates [here](../identity/identity.html#digital-certificates). In practice, the
certificate file also contains some Fabric-specific metadata such as
Isabella's organization and role -- read more in the [wallet](../developapps/wallet.html) topic.

## Issue application

Isabella can now use `issue.js` to submit a transaction that will issue
MagnetoCorp commercial paper `00001`:

```
(isabella)$ node issue.js

Connect to Fabric gateway.
Use network channel: mychannel.
Use org.papernet.commercialpaper smart contract.
Submit commercial paper issue transaction.
Process issue transaction response.{"class":"org.papernet.commercialpaper","key":"\"MagnetoCorp\":\"00001\"","currentState":1,"issuer":"MagnetoCorp","paperNumber":"00001","issueDateTime":"2020-05-31","maturityDateTime":"2020-11-30","faceValue":"5000000","owner":"MagnetoCorp"}
MagnetoCorp commercial paper : 00001 successfully issued for value 5000000
Transaction complete.
Disconnect from Fabric gateway.
Issue program complete.
```

The `node` command initializes a Node.js environment, and runs `issue.js`. We
can see from the program output that MagnetoCorp commercial paper 00001 was
issued with a face value of 5M USD.

As you've seen, to achieve this, the application invokes the `issue` transaction
defined in the `CommercialPaper` smart contract within `papercontract.js`. This
had been installed and instantiated in the network by the MagnetoCorp
administrator. It's the smart contract which interacts with the ledger via the
Fabric APIs, most notably `putState()` and `getState()`, to represent the new
commercial paper as a vector state within the world state. We'll see how this
vector state is subsequently manipulated by the `buy` and `redeem` transactions
also defined within the smart contract.

All the time, the underlying Fabric SDK handles the transaction endorsement,
ordering and notification process, making the application's logic
straightforward; the SDK uses a [gateway](../developapps/gateway.html) to
abstract away network details and
[connectionOptions](../developapps/connectoptions.html) to declare more advanced
processing strategies such as transaction retry.

Let's now follow the lifecycle of MagnetoCorp 00001 by switching our emphasis
to an employee of DigiBank, Balaji, who will buy the commercial paper using a
DigiBank application.

## Digibank applications

Balaji uses DigiBank's `buy` application to submit a transaction to the ledger
which transfers ownership of commercial paper `00001` from MagnetoCorp to
DigiBank. The `CommercialPaper` smart contract is the same as that used by
MagnetoCorp's application, however the transaction is different this time --
it's `buy` rather than `issue`. Let's examine how DigiBank's application works.

Open a separate terminal window for Balaji. In `fabric-samples`, change to the
DigiBank application directory that contains the application, `buy.js`, and open
it with your editor:

```
(balaji)$ cd commercial-paper/organization/digibank/application/
(balaji)$ code buy.js
```

As you can see, this directory contains both the `buy` and `redeem` applications
that will be used by Balaji.


![commercialpaper.vscode3](./commercial_paper.diagram.12.png) *DigiBank's
commercial paper directory containing the `buy.js` and `redeem.js`
applications.*

DigiBank's `buy.js` application is very similar in structure to MagnetoCorp's
`issue.js` with two important differences:


  * **Identity**: the user is a DigiBank user `Balaji` rather than MagnetoCorp's
    `Isabella`

    ```JavaScript
    const wallet = await Wallets.newFileSystemWallet('../identity/user/balaji/wallet');
    ```

    See how the application uses the `balaji` wallet when it connects to the
    PaperNet network channel. `buy.js` selects a particular identity within
    `balaji` wallet.


  * **Transaction**: the invoked transaction is `buy` rather than `issue`

    ```JavaScript
    const buyResponse = await contract.submitTransaction('buy', 'MagnetoCorp', '00001', ...);
    ```

    A `buy` transaction is submitted with the values `MagnetoCorp`, `00001`, ...,
    that are used by the `CommercialPaper` smart contract class to transfer
    ownership of commercial paper `00001` to DigiBank.

Feel free to examine other files in the `application` directory to understand
how the application works, and read in detail how `buy.js` is implemented in
the application [topic](../developapps/application.html).

## Run as DigiBank

The DigiBank applications which buy and redeem commercial paper have a very
similar structure to MagnetoCorp's issue application. Therefore, let’s install
their dependencies and set up Balaji's wallet so that he can use these
applications to buy and redeem commercial paper.

Like MagnetoCorp, Digibank must the install the required application packages
using the ``npm install`` command, and again, this make take a short time to
complete.

In the DigiBank administrator window, install the application dependencies:

```
(digibank admin)$ cd commercial-paper/organization/digibank/application/
(digibank admin)$ npm install

(            ) extract:lodash: sill extract ansi-styles@3.2.1
(...)
added 738 packages in 46.701s
```

In Balaji's command window, run the `addToWallet.js` program to add the identity
to his wallet:

```
(balaji)$ node addToWallet.js

done
```

The `addToWallet.js` program has added identity information for `balaji`, to his
wallet, which will be used by `buy.js` and `redeem.js` to submit transactions to
`PaperNet`.

Like Isabella, Balaji can store multiple identities in his wallet, though in our
example, he only uses one. His corresponding id file at
`digibank/identity/user/balaji/wallet/balaji.id` is very similar Isabella's ---
feel free to examine it.

## Buy application

Balaji can now use `buy.js` to submit a transaction that will transfer ownership
of MagnetoCorp commercial paper 00001 to DigiBank.

Run the `buy` application in Balaji's window:

```
(balaji)$ node buy.js

Connect to Fabric gateway.
Use network channel: mychannel.
Use org.papernet.commercialpaper smart contract.
Submit commercial paper buy transaction.
Process buy transaction response.
MagnetoCorp commercial paper : 00001 successfully purchased by DigiBank
Transaction complete.
Disconnect from Fabric gateway.
Buy program complete.
```

You can see the program output that MagnetoCorp commercial paper 00001 was
successfully purchased by Balaji on behalf of DigiBank. `buy.js` invoked the
`buy` transaction defined in the `CommercialPaper` smart contract which updated
commercial paper `00001` within the world state using the `putState()` and
`getState()` Fabric APIs. As you've seen, the application logic to buy and issue
commercial paper is very similar, as is the smart contract logic.

## Redeem application

The final transaction in the lifecycle of commercial paper 00001 is for
DigiBank to redeem it with MagnetoCorp. Balaji uses `redeem.js` to submit a
transaction to perform the redeem logic within the smart contract.

Run the `redeem` transaction in Balaji's window:

```
(balaji)$ node redeem.js

Connect to Fabric gateway.
Use network channel: mychannel.
Use org.papernet.commercialpaper smart contract.
Submit commercial paper redeem transaction.
Process redeem transaction response.
MagnetoCorp commercial paper : 00001 successfully redeemed with MagnetoCorp
Transaction complete.
Disconnect from Fabric gateway.
Redeem program complete.
```

Again, see how the commercial paper 00001 was successfully redeemed when
`redeem.js` invoked the `redeem` transaction defined in `CommercialPaper`.
Again, it updated commercial paper `00001` within the world state to reflect
that the ownership returned to MagnetoCorp, the issuer of the paper.

## Clean up

When you are finished using the Commercial Paper tutorial, you can use a script
to clean up your environment. Use a command window to navigate back to the root
directory of the commercial paper sample:
```
cd fabric-samples/commercial-paper
```
You can then bring down the network with the following command:
```
./network-clean.sh
```
This command will bring down the peers, CouchDB containers, and ordering node of the network, in addition to the logspout tool. It will also remove the identities that we created for Isabella and Balaji. Note that all of the data on the ledger will be lost. If you want to go through the tutorial again, you will start from a clean initial state.

## Further reading

To understand how applications and smart contracts shown in this tutorial work
in more detail, you'll find it helpful to read
[Developing Applications](../developapps/developing_applications.html). This
topic will give you a fuller explanation of the commercial paper scenario, the
PaperNet business network, its actors, and how the applications and smart
contracts they use work in detail.

Also feel free to use this sample to start creating your own applications and
smart contracts!

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
