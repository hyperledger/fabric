# Fabric Starter Kit

This section describes how to set up a self-contained environment for
application development with the Hyperledger fabric. The setup
uses **Docker** to provide a controlled environment with all the necessary
Hyperledger fabric components to support a Node.js application built with
the fabric's Node.js SDK, and chaincode written in Go.

There are three Docker images that, when run, will provide a basic
network environment. There is an image to run a single `peer`, one to run
the `membersrvc`, and one to run both your Node.js application and your
chaincode. See [Application Developer's Overview](../nodeSDK/app-overview.md) on how the
components running within the containers will communicate.

The starter kit comes with a sample Node.js application ready to execute and
sample chaincode. The starter kit will be running in chaincode developer mode.
In this mode, the chaincode is built and started prior to the application
making a call to deploy it.

**Note:** The deployment of chaincode in network mode requires that the
Hyperledger fabric Node.js SDK has access to the chaincode source code and all
of its dependencies, in order to properly build a deploy request. It also
requires that the `peer` have access to the Docker daemon to be able to build
and deploy the new Docker image that will run the chaincode. *This is a more
complicated configuration and not suitable to an introduction to the
Hyperledger fabric.* We recommend first running in chaincode development mode.

## Further exploration

If you wish, there are a number of chaincode examples near by.
```
   cd ../../chaincode
```
## Getting started

**Note:** This sample was prepared using Docker for Mac 1.12.0

* Prerequisite software to install:

  * [Docker](https://www.docker.com/products/overview)
  * docker-compose (may be packaged with Docker)

* Copy our [docker-compose.yml](https://raw.githubusercontent.com/hyperledger/fabric/master/examples/sdk/node/docker-compose.yml) file to a local directory:

```
   curl -o docker-compose.yml https://raw.githubusercontent.com/hyperledger/fabric/master/examples/sdk/node/docker-compose.yml
```

  The docker-compose environment uses three Docker images. Two are published to
  DockerHub. However, with the third, we provide you the source to build your own,
  so that you can customize it to inject your application code for development. The following    [Dockerfile](https://raw.githubusercontent.com/hyperledger/fabric/master/examples/sdk/node/Dockerfile)
  is used to build the base **fabric-starter-kit** image and may be used as
  a starting point for your own customizations.

```
   curl -o Dockerfile https://raw.githubusercontent.com/hyperledger/fabric/master/examples/sdk/node/Dockerfile
   docker build -t hyperledger/fabric-starter-kit:latest .
```

* Start the fabric network environment using docker-compose. From a terminal
session that has the working directory of where the above *docker-compose.yml*
is located, execute one of following `docker-compose` commands.

   * to run as detached containers:

     ```
       docker-compose up -d
     ```

     **note:** to see the logs for the `peer` container use the
     `docker logs peer` command

   * to run in the foreground and see the log output in the current terminal
   session:

     ```
       docker-compose up
     ```

  Both commands will start three Docker containers. To view the container
  status use the `docker ps` command. The first time this is run, the Docker
  images will be downloaded. This may take 10 minutes or more depending on the
  network connections of the system running the command.

```
   docker ps
```

   You should see something like the following:

   ```
    CONTAINER ID    IMAGE                           COMMAND                  CREATED              STATUS              PORTS  NAMES
    bb01a2fa96ef    hyperledger/fabric-starter-kit  "sh -c 'sleep 20; /op"   About a minute ago   Up 59 seconds           starter
    ec7572e65f12    hyperledger/fabric-peer         "sh -c 'sleep 10; pee"   About a minute ago   Up About a minute          peer
    118ef6da1709    hyperledger/fabric-membersrvc   "membersrvc"             About a minute ago   Up About a minute          membersrvc
   ```

* Start a terminal session in the **starter** container. This is where the
Node.js application is located.

  **note:** Be sure to wait 20 seconds after starting the network using the
  `docker-compose up` command before executing the following command to allow
  the network to initialize:

```
   docker exec -it starter /bin/bash
```

* From the terminal session in the **starter** container execute the standalone
Node.js application. The Docker terminal session should be in the working
directory of the sample application called **app.js**  (*/opt/gopath/src/github.com/hyperledger/fabric/examples/sdk/node*). Execute
the following Node.js command to run the application:

```
   node app
```
   In another terminal session on the host you can view the logs for the peer
   by executing the following command (not in the docker shell above, in a new
   terminal session of the real system):

```
   docker logs peer
```

* If you wish to run your own Node.js application using the pre-built Docker
images:
   * use the directories in the `volumes` tag under **starter** in the
   `docker-compose.yml` file as a place to store your programs from the host
   system into the docker container. The first path is the top level system
   (host system) and the second is created in the Docker container. If you wish
   to use a host location that is not under the `/Users` directory (`~` is
     under `/Users') then you must add that to the Docker file sharing
     under Docker preferences.

```yaml
  volumes:
    - ~/mytest:/user/mytest
```
   * copy or create and edit your application in the `~/mytest` directory as
   stated in the `docker-compose.yml` `volumes` tag under **starter** container.
   * run npm to install Hyperledger fabric Node.js SDK in the `mytest` directory:

```
     npm install /opt/gopath/src/github.com/hyperledger/fabric/sdk/node
```
   * run the application from within the **starter** Docker container using the
     following commands:

```
   docker exec -it starter /bin/bash
```
   once in the shell, and assuming your Node.js application is called `app.js`:

```
   cd /user/mytest
   node app
```
* To shutdown the environment, execute the following **docker-compose** command
in the directory where the *docker-compose.yml* is located. Any changes you made
to the sample application or deployment of a chaincode will be lost. Only
changes made to the shared area defined in the 'volumes' tag of the **starter**
container will persist.  This will shutdown each of the containers and remove
the containers from Docker:

```
   docker-compose down
```
   or if you wish to keep your changes and just stop the containers, which will
   be restarted on the next `up` command:

```
   docker-compose kill
```
