## Setting Up a Network

This document covers setting up a network on your local machine for various development and testing activities. Unless you are intending to contribute to the development of the Hyperledger Fabric project, you'll probably want to follow the more commonly used approach below - [leveraging published Docker images](#leveraging-published-docker-images) for the various Hyperledger Fabric components, directly. Otherwise, skip down to the [secondary approach](#building-your-own-images) below.

### Leveraging published Docker images

This approach simply leverages the Docker images that the Hyperledger Fabric project publishes to [DockerHub](https://hub.docker.com/u/hyperledger/) and either Docker commands or Docker Compose descriptions of the network one wishes to create.

#### Installing Docker

**Note:** When running Docker _natively_ on Mac and Windows, there is no IP forwarding support available. Hence, running more than one fabric-peer image is not advised because you do not want to have multiple processes binding to the same port. For most application and chaincode development/testing running with a single fabric peer should not be an issue unless you are interested in performance and resilience testing the fabric's capabilities, such as consensus. For more advanced testing, we strongly recommend using the fabric's Vagrant [development environment](../dev-setup/devenv.md).

With this approach, there are multiple choices as to how to run Docker: using [Docker Toolbox](https://docs.docker.com/toolbox/overview/) or one of the new native Docker runtime environments for [Mac OSX](https://docs.docker.com/engine/installation/mac/) or [Windows](https://docs.docker.com/engine/installation/windows/). There are some subtle differences between how Docker runs natively on Mac and Windows versus in a virtualized context on Linux. We'll call those out where appropriate below, when we get to the point of actually running the various components.

#### Pulling the images from DockerHub

Once you have Docker (1.11 or greater) installed and running,
prior to starting any of the fabric components, you will need to first pull the fabric images from DockerHub.

```
  docker pull hyperledger/fabric-peer:latest
  docker pull hyperledger/fabric-membersrvc:latest
```

### Building your own images

**Note:** _This approach is not necessarily recommended for most users_. If you have pulled images from DockerHub as described in the previous section, you may proceed to the [next step](#starting-up-validating-peers).

The second approach would be to leverage the [development environment](../dev-setup/devenv.md) setup (which we will assume you have already established) to build and deploy your own binaries and/or Docker images from a clone of the [hyperledger/fabric](https://github.com/hyperledger/fabric) GitHub repository. This approach is suitable for developers that might wish to contribute directly to the Hyperledger Fabric project, or that wish to deploy from a fork of the Hyperledger code base.

The following commands should be run from _within_ the Vagrant environment described in [Setting Up Development Environment](../dev-setup/devenv.md).

To create the Docker image for the `hyperledger/fabric-peer`:

```
cd $GOPATH/src/github.com/hyperledger/fabric
make peer-image
```

To create the Docker image for the `hyperledger/fabric-membersrvc`:

```
make membersrvc-image
```

### Starting up validating peers

Check the available images again with `docker images`. You should see `hyperledger/fabric-peer` and `hyperledger/fabric-membersrvc` images. For example,

```
$ docker images
REPOSITORY                      TAG                 IMAGE ID            CREATED             SIZE
hyperledger/fabric-membersrvc   latest              7d5f6e0bcfac        12 days ago         1.439 GB
hyperledger/fabric-peer         latest              82ef20d7507c        12 days ago         1.445 GB
```
If you don't see these, go back to the previous step.

With the relevant Docker images in hand, we can start running the peer and membersrvc services.

#### Determine value for CORE_VM_ENDPOINT variable

Next, we need to determine the address of your docker daemon for the CORE_VM_ENDPOINT. If you are working within the Vagrant development environment, or a Docker Toolbox environment, you can determine this with the `ip add` command. For example,

```
$ ip add

<<< detail removed >>>

3: docker0: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc noqueue state DOWN group default
    link/ether 02:42:ad:be:70:cb brd ff:ff:ff:ff:ff:ff
    inet 172.17.0.1/16 scope global docker0
       valid_lft forever preferred_lft forever
    inet6 fe80::42:adff:febe:70cb/64 scope link
       valid_lft forever preferred_lft forever
```

Your output might contain something like `inet 172.17.0.1/16 scope global docker0`. That means the docker0 interface is on IP address 172.17.0.1. Use that IP address for the `CORE_VM_ENDPOINT` option. For more information on the environment variables, see `core.yaml` configuration file in the `fabric` repository.

If you are using the native Docker for Mac or Windows, the value for `CORE_VM_ENDPOINT` should be set to `unix:///var/run/docker.sock`. \[TODO] double check this. I believe that `127.0.0.1:2375` also works.

#### Assigning a value for CORE_PEER_ID

The ID value of `CORE_PEER_ID` must be unique for each validating peer, and it must be a lowercase string. We often use a convention of naming the validating peers vpN where N is an integer starting with 0 for the root node and incrementing N by 1 for each additional peer node started. e.g. vp0, vp1, vp2, ...

#### Consensus

By default, we are using a consensus plugin called `NOOPS`, which doesn't really do consensus. If you are running a single peer node, running anything other than `NOOPS` makes little sense. If you want to use some other consensus plugin in the context of multiple peer nodes, please see the [Using a Consensus Plugin](#using-a-consensus-plugin) section, below.

#### Docker Compose

We'll be using Docker Compose to launch our various Fabric component containers, as this is the simplest approach. You should have it installed from the initial setup steps. Installing Docker Toolbox or any of the native Docker runtimes should have installed Compose.

#### Start up a validating peer:

Let's launch the first validating peer (the root node). We'll set CORE_PEER_ID to vp0 and CORE_VM_ENDPOINT as above. Here's the docker-compose.yml for launching a single container within the **Vagrant** [development environment](../dev-setup/devenv.md):

```
vp0:
  image: hyperledger/fabric-peer
  environment:
    - CORE_PEER_ID=vp0
    - CORE_PEER_ADDRESSAUTODETECT=true
    - CORE_VM_ENDPOINT=http://172.17.0.1:2375
    - CORE_LOGGING_LEVEL=DEBUG
  command: peer node start
```
You can launch this Compose file as follows, from the same directory as the docker-compose.yml file:

```
$ docker-compose up
```

Here's the corresponding Docker command:
```
$ docker run --rm -it -e CORE_VM_ENDPOINT=http://172.17.0.1:2375 -e CORE_LOGGING_LEVEL=DEBUG -e CORE_PEER_ID=vp0 -e CORE_PEER_ADDRESSAUTODETECT=true hyperledger/fabric-peer peer node start
```

If you are running Docker for Mac or Windows, we'll need to explicitly map the ports, and we will need a different value for CORE_VM_ENDPOINT as we discussed above.

Here's the docker-compose.yml for Docker on Mac or Windows:

```
vp0:
  image: hyperledger/fabric-peer
  ports:
    - "7050:7050"
    - "7051:7051"
    - "7052:7052"
  environment:
    - CORE_PEER_ADDRESSAUTODETECT=true
    - CORE_VM_ENDPOINT=unix:///var/run/docker.sock
    - CORE_LOGGING_LEVEL=DEBUG
  command: peer node start
```

This single peer configuration, running the `NOOPS` 'consensus' plugin, should satisfy many development/test scenarios. `NOOPS` is not really providing consensus, it is essentially a no-op that simulates consensus. For instance, if you are simply developing and testing chaincode; this should be adequate unless your chaincode is leveraging membership services for identity, access control, confidentiality and privacy.

#### Running with the CA

If you want to take advantage of security (authentication and authorization), privacy and confidentiality, then you'll need to run the Fabric's certificate authority (CA). Please refer to the [CA Setup](ca-setup.md) instructions.

#### Start up additional validating peers:

Following the pattern we established [above](#assigning-a-value-for-core_peer_id) we'll use `vp1` as the ID for the second validating peer. If using Docker Compose, we can simply link the two peer nodes.
Here's the docker-compse.yml for a **Vagrant** environment with two peer nodes - vp0 and vp1:
```
vp0:
  image: hyperledger/fabric-peer
  environment:
    - CORE_PEER_ADDRESSAUTODETECT=true
    - CORE_VM_ENDPOINT=http://172.17.0.1:2375
    - CORE_LOGGING_LEVEL=DEBUG
  command: peer node start
vp1:
  extends:
    service: vp0
  environment:
    - CORE_PEER_ID=vp1
    - CORE_PEER_DISCOVERY_ROOTNODE=vp0:7051
  links:
    - vp0
```

If we wanted to use the docker command line to launch another peer, we need to get the IP address of the first validating peer, which will act as the root node to which the new peer(s) will connect. The address is printed out on the terminal window of the first peer (e.g. 172.17.0.2) and should be passed in with the `CORE_PEER_DISCOVERY_ROOTNODE` environment variable.

```
docker run --rm -it -e CORE_VM_ENDPOINT=http://172.17.0.1:2375 -e CORE_PEER_ID=vp1 -e CORE_PEER_ADDRESSAUTODETECT=true -e CORE_PEER_DISCOVERY_ROOTNODE=172.17.0.2:7051 hyperledger/fabric-peer peer node start
```
<!-- This needs to be sorted out with a revamped security section

Again, the validating peer `enrollID` and `enrollSecret` (`vp1` and `vp1_secret`) has to be added to [membersrvc.yaml](https://github.com/hyperledger/fabric/blob/master/membersrvc/membersrvc.yaml).

You can start up a few more validating peers in a similar manner if you wish. Remember to change the peer ID and add the enrollID/enrollSecret to the [membersrvc.yaml](https://github.com/hyperledger/fabric/blob/master/membersrvc/membersrvc.yaml).

### Enroll/Login a test user (if security is enabled):
If security is enabled, you must enroll a user with the certificate authority before sending requests. Choose a user that is already registered, i.e. added to the [membersrvc.yaml](https://github.com/hyperledger/fabric/blob/master/membersrvc/membersrvc.yaml). Then, execute the command below to log in the user on the target validating peer. `CORE_PEER_ADDRESS` specifies the target validating peer for which the user is to be logged in.

```
CORE_PEER_ADDRESS=172.17.0.2:7051 peer network login jim
```

**Note:** The certificate authority allows the enrollID and enrollSecret credentials to be used only *once*. Therefore, login by the same user from any other validating peer will result in an error. Currently, the application layer is responsible for duplicating the crypto material returned from the CA to other peer nodes. If you want to test secure transactions from more than one peer node without replicating the returned key and certificate, you can log in with a different user on other peer nodes.

### Deploy, Invoke, and Query a Chaincode


**Note:** When security is enabled, modify the CLI commands to deploy, invoke, or query a chaincode to pass the username of a logged in user. To log in a registered user through the CLI, execute the login command from the section above. On the CLI the username is passed with the -u parameter.

We can use the sample chaincode to test the network. You may find the chaincode here `$GOPATH/src/github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02`.

Deploy the chaincode to the network. We can deploy to any validating peer by specifying `CORE_PEER_ADDRESS`:

```
CORE_PEER_ADDRESS=172.17.0.2:7051 peer chaincode deploy -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -c '{"Function":"init", "Args": ["a","100", "b", "200"]}'
```

With security enabled, modify the command as follows:

```
CORE_PEER_ADDRESS=172.17.0.2:7051 CORE_SECURITY_ENABLED=true CORE_SECURITY_PRIVACY=true peer chaincode deploy -u jim -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -c '{"Function":"init", "Args": ["a","100", "b", "200"]}'
```

You can watch for the message "Received build request for chaincode spec" on the output screen of all validating peers.

**Note:** If your GOPATH environment variable contains more than one element, the chaincode must be found in the first one or deployment will fail.

On successful completion, the above command will print the "name" assigned to the deployed chaincode. This "name" is used as the value of the "-n" parameter in invoke and query commands described below. For example the value of "name" could be

    bb540edfc1ee2ac0f5e2ec6000677f4cd1c6728046d5e32dede7fea11a42f86a6943b76a8f9154f4792032551ed320871ff7b7076047e4184292e01e3421889c

In a script the name can be captured for subsequent use. For example, run

    NAME=`CORE_PEER_ADDRESS=172.17.0.2:7051 CORE_SECURITY_ENABLED=true CORE_SECURITY_PRIVACY=true peer chaincode deploy ...`

and then replace `<name_value_returned_from_deploy_command>` in the examples below with `$NAME`.

We can run an invoke transaction to move 10 units from the value of `a` to the value of `b`:

```
CORE_PEER_ADDRESS=172.17.0.2:7051 peer chaincode invoke -n <name_value_returned_from_deploy_command> -c '{"Function": "invoke", "Args": ["a", "b", "10"]}'
```

With security enabled, modify the command as follows:

```
CORE_PEER_ADDRESS=172.17.0.2:7051 CORE_SECURITY_ENABLED=true CORE_SECURITY_PRIVACY=true peer chaincode invoke -u jim -n <name_value_returned_from_deploy_command> -c '{"Function": "invoke", "Args": ["a", "b", "10"]}'
```

We can also run a query to see the current value `a` has:

```
CORE_PEER_ADDRESS=172.17.0.2:7051 peer chaincode query -l golang -n <name_value_returned_from_deploy_command> -c '{"Function": "query", "Args": ["a"]}'
```

With security enabled, modify the command as follows:

```
CORE_PEER_ADDRESS=172.17.0.2:7051 CORE_SECURITY_ENABLED=true CORE_SECURITY_PRIVACY=true peer chaincode query -u jim -l golang -n <name_value_returned_from_deploy_command> -c '{"Function": "query", "Args": ["a"]}'
```
-->

### Using a Consensus Plugin
A consensus plugin might require some specific configuration that you need to set up. For example, to use the Practical Byzantine Fault Tolerant (PBFT) consensus plugin provided as part of the fabric, perform the following configuration:

1. In `core.yaml`, set the `peer.validator.consensus` value to `pbft`
2. In `core.yaml`, make sure the `peer.id` is set sequentially as `vpN` where `N` is an integer that starts from `0` and goes to `N-1`. For example, with 4 validating peers, set the `peer.id` to`vp0`, `vp1`, `vp2`, `vp3`.
3. In `consensus/pbft/config.yaml`, set the `general.mode` value to `batch` and the `general.N` value to the number of validating peers on the network, also set `general.batchsize` to the number of transactions per batch.
4. In `consensus/pbft/config.yaml`, optionally set timer values for the batch period (`general.timeout.batch`), the acceptable delay between request and execution (`general.timeout.request`), and for view-change (`general.timeout.viewchange`)

See `core.yaml` and `consensus/pbft/config.yaml` for more detail.

All of these setting may be overridden via the command line environment variables, e.g. `CORE_PEER_VALIDATOR_CONSENSUS_PLUGIN=pbft` or `CORE_PBFT_GENERAL_MODE=batch`

### Logging control

See [Logging Control](logging-control.md) for information on controlling
logging output from the `peer` and deployed chaincodes.

<!--
**Note:** When running with security enabled, follow the security setup instructions described in [Chaincode Development](../Setup/Chaincode-setup.md#security-setup-optional) to set up the CA server and log in registered users before sending chaincode transactions. In this case peers started using Docker images need to point to the correct CA address (default is localhost). CA addresses have to be specified in `peer/core.yaml` variables paddr of eca, tca and tlsca. Furthermore, if you are enabling security and privacy on the peer process with environment variables, it is important to include these environment variables in the command when executing all subsequent peer operations (e.g. deploy, invoke, or query).
-->
