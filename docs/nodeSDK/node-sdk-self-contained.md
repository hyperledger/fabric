# Self Contained Node.js Environment

This section describes how to set up a self contained environment for Node.js application development with the Hyperledger Fabric Node.js SDK. The setup uses **Docker** to provide a controlled environment with all the necessary Hyperledger fabric components to support a Node.js application. There are three **Docker** images that when run will provide a blockchain network environment. There is an image to run a single **Peer**, one to run the **Member Services** and one to run both a Node.js application and the sample chaincode. See [Application Developer's Overview](app-overview.md) on how the components running within the containers will communicate. The sample comes with a sample Node.js application ready to execute and sample chaincode. The sample will be running in developer mode where the chaincode has been built and started prior to the application call to deploy it. The deployment of chaincode in network mode requires that the Hyperledger Fabric Node.js SDK has access to the chaincode source code and all of its dependant code, in order to properly build a deploy request. It also requires that the **peer** have access to **docker** functions to be able to build and deploy the new **docker** image that will run the chaincode. This is a more complicated configuration and not suitable to an introduction to the Hyperledger Fabric Node.js SDK.

**note:** This sample was prepared using Docker for Mac 1.12.0

1. Prerequisite software to install:

  * Docker
  * docker-compose (may be packaged with Docker)

2. Create a docker-compose file called *docker-compose.yml*

   You may retrieve the docker-compose.yml file:

```
   curl -o docker-compose.yml https://raw.githubusercontent.com/hyperledger/fabric/master/examples/sdk/node/example02/docker-compose.yml
```

   docker-compose.yml:
```yaml
membersrvc:
  # try 'docker ps' to see the container status after starting this compose
  container_name: membersrvc
  image: hyperledger/fabric-membersrvc
  command: membersrvc

peer:
  container_name: peer
  image: hyperledger/fabric-peer
  environment:
    - CORE_PEER_ADDRESSAUTODETECT=true
    - CORE_VM_ENDPOINT=unix:///var/run/docker.sock
    - CORE_LOGGING_LEVEL=DEBUG
    - CORE_PEER_ID=vp0
    - CORE_SECURITY_ENABLED=true
    - CORE_PEER_PKI_ECA_PADDR=membersrvc:7054
    - CORE_PEER_PKI_TCA_PADDR=membersrvc:7054
    - CORE_PEER_PKI_TLSCA_PADDR=membersrvc:7054
    - CORE_PEER_VALIDATOR_CONSENSUS_PLUGIN=noops
  # this gives access to the docker host daemon to deploy chaincode in network mode
  volumes:
    - /var/run/docker.sock:/var/run/docker.sock
  # have the peer wait 10 sec for membersrvc to start
  #  the following is to run the peer in Developer mode - also set sample DEPLOY_MODE=dev
  command: sh -c "sleep 10; peer node start --peer-chaincodedev"
  #command: sh -c "sleep 10; peer node start"
  links:
    - membersrvc

nodesdk:
  container_name: nodesdk
  image: hyperledger/fabric-node-sdk
  volumes:
    - ~/mytest:/user/mytest
  environment:
    - MEMBERSRVC_ADDRESS=membersrvc:7054
    - PEER_ADDRESS=peer:7051
    - KEY_VALUE_STORE=/tmp/hl_sdk_node_key_value_store
    - NODE_PATH=/usr/local/lib/node_modules
    # set DEPLOY_MODE to 'dev' if peer running in Developer mode
    - DEPLOY_MODE=dev
    - CORE_CHAINCODE_ID_NAME=mycc
    - CORE_PEER_ADDRESS=peer:7051
  # the following command will start the chaincode when this container starts and ready it for deployment by the app
  command: sh -c "sleep 20; /opt/gopath/src/github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02/chaincode_example02"
  stdin_open: true
  tty: true
  links:
    - membersrvc
    - peer

```

3. Start the fabric environment using docker-compose. From a terminal session where the working directory is where the *docker-compose.yml* from step 2 is located, execute one of following **docker-compose** commands.

   * to run as detached containers, then to see the logs for the **peer** container use the `docker logs peer` command
```
   docker-compose up -d
```
   * to run in the foreground and see the log output in the current terminal session
```
   docker-compose up
```

   This will start three docker containers, to view the container status try `docker ps` command. The first time this is run the **docker** images will be downloaded. This may take 10 minutes or more depending on the network connections of the system running the command.
      * Membership services --**membersrvc**
      * Peer --               **peer**
      * Node.js SDK Application and chaincode -- **nodesdk**

4. Start a terminal session in the **nodesdk** container. This is where the Node.js application is located. 

  **Note:** Be sure to wait 20 seconds after starting the network before executing.

```
   docker exec -it nodesdk /bin/bash
```

5. From the terminal session in the **nodesdk** container execute the standalone Node.js application. The docker terminal session should be in the working directory of the sample application called **app.js**  (*/opt/gopath/src/github.com/hyperledger/fabric/examples/sdk/node/example02*). Execute the following Node.js command to run the application.

```
   node app
```
   On another terminal session on the host you can view the logs for the peer by executing the following command (not in the docker shell above, in a new terminal session of the real system)
```
   docker logs peer
```

6. This environment will have the reference documentation already built and may be access by:

```
   docker exec -it nodesdk /bin/bash
   cd /opt/gopath/src/github.com/hyperledger/fabric/sdk/node/doc
```

7. If you wish to run your own Node.js application
   * use the directories in the `volumes` tag under **nodesdk** in the `docker-compose.yml` file as a place to store your programs from the host system into the docker container. The first path is the top level system (host system) and the second is created in the docker container. If you wish to use a host location that is not under the `/Users` directory (`~` is under `/Users') then you must add that to the **docker** file sharing under **docker** preferences.

```yaml
  volumes:
    - ~/mytest:/user/mytest
```
   * copy or create and edit your application in the `~/mytest` directory as stated in the `docker-compose.yml` `volumes` tag under **nodesdk** container.
   * run npm to install Hyperledger Fabric Node.js SDK in the `mytest` directory
```
     npm install /opt/gopath/src/github.com/hyperledger/fabric/sdk/node
```
   * run the application from within the **nodesdk** docker container using the commands
```
   docker exec -it nodesdk /bin/bash
```
   once in the shell, and assuming your Node.js application is called `app.js`
```
   cd /user/mytest
   node app
```
8. To shutdown the environment, execute the following **docker-compose** command in the directory where the *docker-compose.yml* is located. Any changes you made to the sample application or deployment of a chaincode will be lost. Only changes made to the shared area defined in the 'volumes' tag of the **nodesdk** container will persist.  This will shutdown each of the containers and remove the containers from **docker**:

```
   docker-compose down
```
   or if you wish to keep your changes and just stop the containers, which will be restarted on the next `up` command

```
   docker-compose kill
```
