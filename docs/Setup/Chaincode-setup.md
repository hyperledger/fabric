## Writing, Building, and Running Chaincode in a Development Environment

Chaincode developers need a way to test and debug their chaincode without having
to set up a complete peer network. By default, when you want to interact with chaincode,
you need to first `Deploy` it using the CLI, REST API, gRPC API, or SDK. Upon receiving
this request, the peer node would typically spin up a Docker container with the
relevant chaincode. This can make things rather complicated for debugging chaincode
under development, because of the turnaround time with the `launch chaincode -
debug docker container - fix problem - launch chaincode - lather - rinse - repeat`
cycle. As such, the fabric peer has a `--peer-chaincodedev` flag that can be passed
on start-up to instruct the peer node not to deploy the chaincode as a Docker container.

The following instructions apply to _developing_ chaincode in Go or Java. They do
not apply to running in a production environment. However, if _developing_ chaincode
in Java, please see the [Java chaincode setup](https://github.com/hyperledger/fabric/blob/master/docs/Setup/JAVAChaincode.md)
instructions first, to be sure your environment is properly configured.

**Note:** We have added support for [System chaincode](https://github.com/hyperledger/fabric/blob/master/docs/SystemChaincode-noop.md).

## Choices

Once again, you have the choice of using one of the following approaches:

- [Option 1](#option-1-docker-for-mac-or-windows) using Docker for Mac or Windows
- [Option 2](#option-2-docker-toolbox) using Docker toolbox
- [Option 3](#option-3-vagrant-development-environment) using the **Vagrant** [development environment](https://github.com/hyperledger/fabric/blob/master/docs/dev-setup/devenv.md)
that is used for developing the fabric itself

A Docker approach provides several advantages, highlighted through its simplicity.  
By using options *1* or *2*, from above, you avoid having to build everything from
scratch, and there's no need to keep a synchronized clone of the Hyperledger fabric
codebase. Instead, you can simply pull and run the `fabric-peer` and `fabric-membersrvc`
images directly from DockerHub.  There is no need to manually start the peer and
member service nodes, rather a single `docker-compose up` command will spin up a
live network on your machine.  Additionally, you are able to operate from a single
terminal, and you avoid the extra layer of abstraction and virtualization which
arises when using the Vagrant environment.

For more information on using Docker Compose, and customizing your Docker environment,
see the [Docker Setup Guide](Setup/Docker-setup.md).  If you are not familiar with
Docker and/or chaincode development, it's recommended to go through this section first.

## Option 1 Docker for Mac or Windows

The Docker images for `fabric-peer` and `fabric-membersrvc` are continuously built
and tested through the Hyperledger fabric CI (continuous integration). To run these
fabric components on your Mac or Windows laptop/server using the Docker for [Mac](https://docs.docker.com/engine/installation/mac/) or [Windows](https://docs.docker.com/engine/installation/windows/) platform, follow
these steps. If using [Docker Toolbox](https://docs.docker.com/toolbox/overview/),
please skip to [Option 2](#option-2-docker-toolbox), below.

### Pull images from DockerHub

You DO NOT need to manually pull the `fabric-peer`, `fabric-membersrvc` or `fabric-baseimage`
images published by the Hyperledger Fabric project from DockerHub.  These images
are specified in the docker-compose.yaml and will be automatically downloaded and
extracted when you run `docker-compose up`.  However, you do need to ensure that
the image tags correspond correctly to your platform.

Identify your platform and check the image tags.  Use the __Tags__ tab in the
`hyperledger/fabric-baseimage` repository on [DockerHub](https://hub.docker.com/r/hyperledger/fabric-baseimage/tags/) to browse
the available images.  For example, if you are running Docker natively on Linux
or OSX then you will want:

```
hyperledger/fabric-baseimage:x86_64-0.2.0
```

### Retrieve the docker-compose file

Now you need to retrieve a Docker Compose file to spin up your network.  There
are two standard Docker Compose files available.  One is for a single node + CA
network, and the second is for a four node + CA network.  Identify or create a
working directory where you want the Docker Compose file(s) to reside; this can
be anywhere on your machine (the below directory is simply an example).  Then
execute a `cURL` to retrieve the .yaml file.  For example, to retrieve the .yaml
file for a single node + CA network:

```
mkdir -p $HOME/hyperledger/docker-compose
cd $HOME/hyperledger/docker-compose
curl https://raw.githubusercontent.com/hyperledger/fabric/v0.6/examples/docker-compose/single-peer-ca.yaml -o single-peer-ca.yaml 2>/dev/null
```

OR to retrieve the .yaml file for a four node + CA network:

```
mkdir -p $HOME/hyperledger/docker-compose
cd $HOME/hyperledger/docker-compose
curl https://raw.githubusercontent.com/hyperledger/fabric/v0.6/examples/docker-compose/four-peer-ca.yaml -o four-peer-ca.yaml 2>/dev/null
```

If you want to configure your network to use specific `fabric-peer` or `fabric-membersrvc`
images from [Hyperledger Docker Hub](https://hub.docker.com/u/hyperledger/), use
the __Tags__ tab in the corresponding image repository to browse the available
versions.  Then add the tag in your Docker Compose .yaml file.  For example, in
the `single-peer-ca.yaml` you might alter the `hyperledger/fabric-peer` image from:
```
vp0:
    image: hyperledger/fabric-peer
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
```
to
```
vp0:
    image: hyperledger/fabric-peer:x86_64-0.6.1-preview
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
```

### Running the Peer and CA

To run the `fabric-peer`and `fabric-membersrvc` images, you will use [Docker Compose](https://docs.docker.com/compose/) against one of your .yaml files.  You
specify the file after the `-f` argument on the command line.  Therefore, to
spin up the single node + CA network you first navigate to the working directory
where your compose file(s) reside, and then execute `docker-compose up` from the
command line:

```
cd $HOME/hyperledger/docker-compose
docker-compose -f single-peer-ca.yaml up
```

OR for a four node + CA network:

```
cd $HOME/hyperledger/docker-compose
docker-compose -f four-node-ca.yaml up
```

Now, you are ready to start [running the chaincode](#running-the-chaincode).

## Option 2 Docker Toolbox

If you are using [Docker Toolbox](https://docs.docker.com/toolbox/overview/),
please follow these instructions.

__Note__: Docker will not run natively on older versions of macOS or any Windows
versions prior to Windows 10.  If either scenario describes your OS, you must use
Docker Toolbox.

Docker Toolbox bundles Docker Engine, Docker Machine and Docker Compose, and by
means of a VirtualBox, provides you with an environment to run Docker processes.
You initialize the Docker host simply by launching the Docker Quick Start Terminal.
Once the host is initialized, you can run all of the Docker commands and Docker
Compose commands from the toolbox as if you were running them on the command line.
Once you are in the toolbox, it is the same experience as if you were running on
a Linux machine with Docker & Docker Compose installed.

Start up the default Docker host by clicking on the Docker Quick Start Terminal.
It will open a new terminal window and initialize the Docker host. Once the
startup process is complete, you will see the Docker whale together with the IP
address of the Docker host, as shown below. In this example the IP address of the
Docker host is 192.168.99.100. Take note of this IP address as you will need it
later to connect to your Docker containers.

If you need to retrieve an IP address for one of your peers, use the `docker inspect`
command.  For more information on useful Docker commands, refer to the [Docker documentation](https://docs.docker.com).

```
                        ##         .
                  ## ## ##        ==
               ## ## ## ## ##    ===
        /"""""""""""""""""\___/ ===
   ~~~ {~~ ~~~~ ~~~ ~~~~ ~~~ ~ /  ===- ~~~
        \______ o           __/
         \    \         __/
          \____\_______/

docker is configured to use the default machine with IP 192.168.99.100
For help getting started, check out the docs at https://docs.docker.com
```
### Pull images from DockerHub

You DO NOT need to manually pull the `fabric-peer`, `fabric-membersrvc` or
`fabric-baseimage` images published by the Hyperledger Fabric project from DockerHub.  
These images are specified in the docker-compose.yaml and will be automatically
downloaded and extracted when you run `docker-compose up`.  However, you do need
to ensure that the image tags correspond correctly to your platform.

Identify your platform and check the image tags.  Use the __Tags__ tab in the
`hyperledger/fabric-baseimage` repository on [DockerHub](https://hub.docker.com/r/hyperledger/fabric-baseimage/tags/) to browse
the available images.  If you are using Docker toolbox, then you will want:

```
hyperledger/fabric-baseimage:x86_64-0.2.0
```

### Retrieve the docker-compose file

Now you need to retrieve a Docker Compose file to spin up your network.  There
are two standard Docker Compose files available.  One is for a single node + CA
network, and the second is for a four node + CA network.  Identify or create a
working directory where you want the Docker Compose file(s) to reside.  Then
execute a `cURL` to retrieve the .yaml file.  For example, to retrieve the .yaml
file for a single node + CA network:

```
mkdir -p $HOME/hyperledger/docker-compose
cd $HOME/hyperledger/docker-compose
curl https://raw.githubusercontent.com/hyperledger/fabric/master/examples/docker-compose/single-peer-ca.yaml -o single-peer-ca.yaml 2>/dev/null
```

OR to retrieve the .yaml file for a four node + CA network:

```
mkdir -p $HOME/hyperledger/docker-compose
cd $HOME/hyperledger/docker-compose
curl https://raw.githubusercontent.com/hyperledger/fabric/master/examples/docker-compose/four-peer-ca.yaml
-o four-peer-ca.yaml 2>dev/null
```

If you want to configure your network to use specific `fabric-peer` or `fabric-membersrvc`
images from [Hyperledger Docker Hub](https://hub.docker.com/u/hyperledger/), use
the __Tags__ tab in the corresponding image repository to browse the available
versions.  Then add the tag in your Docker Compose .yaml file.  For example, in
the `single-peer-ca.yaml` you might alter the `hyperledger/fabric-peer` image from:
```
vp0:
    image: hyperledger/fabric-peer
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
```
to
```
vp0:
    image: hyperledger/fabric-peer:x86_64-0.6.1-preview
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
```

### Running the Peer and CA

To run the `fabric-peer`and `fabric-membersrvc` images, you will use [Docker Compose](https://docs.docker.com/compose/) against one of your .yaml files.  You
specify the file through the `-f` argument on the command line.  Therefore, to
spin up the single node + CA network you first navigate to the working directory
where your compose file(s) reside, and then execute `docker-compose up` from the
command line:

```
cd $HOME/hyperledger/docker-compose
docker-compose -f single-peer-ca.yaml up
```

OR for a four node + CA network:

```
cd $HOME/hyperledger/docker-compose
docker-compose -f four-node-ca.yaml up
```

Now, you are ready to start [running the chaincode](#running-the-chaincode).

## Option 3 Vagrant development environment

You will need multiple terminal windows - essentially one for each component.
One runs the validating peer; another  runs the chaincode; the third runs the CLI
or REST API commands to execute transactions. Finally, when running with security
enabled, an additional fourth window is required to run the **Certificate Authority
(CA)** server. Detailed instructions are provided in the sections below.

__Note__: Using the Vagrant environment results in a more complicated scenario
due to an extra layer of virtualization and the need for multiple terminals.  
Running Docker natively or using Docker Toolbox are the recommended approaches.

### Security Setup (optional)

From the `devenv` subdirectory of your fabric workspace environment, `ssh` into
Vagrant:

```
cd $GOPATH/src/github.com/hyperledger/fabric/devenv
vagrant ssh
```

To set up the local development environment with security enabled, you must first
build and run the **Certificate Authority (CA)** server:

```
cd $GOPATH/src/github.com/hyperledger/fabric
make membersrvc && membersrvc
```

Running the above commands builds and runs the CA server with the default setup,
which is defined in the [membersrvc.yaml](https://github.com/hyperledger/fabric/blob/master/membersrvc/membersrvc.yaml)
configuration file. The default configuration includes multiple users who are
already registered with the CA; these users are listed in the `eca.users` section
of the configuration file. To register additional users with the CA for testing,
modify the `eca.users` section of the [membersrvc.yaml](https://github.com/hyperledger/fabric/blob/master/membersrvc/membersrvc.yaml)
file to include additional `enrollmentID` and `enrollmentPW` pairs. Note the
integer that precedes the `enrollmentPW`. That integer indicates the role of the
user, where 1 = client, 2 = non-validating peer, 4 = validating peer, and
8 = auditor.

### Running the validating peer

**Note:** To run with security enabled, first modify the [core.yaml](https://github.com/hyperledger/fabric/blob/master/peer/core.yaml)
configuration file to set the `security.enabled` value to `true` before building
the peer executable. Alternatively, you can enable security by running the peer
with the following environment variable: `CORE_SECURITY_ENABLED=true`. To enable
privacy and confidentiality of transactions (which requires security to also be
enabled), modify the [core.yaml](https://github.com/hyperledger/fabric/blob/master/peer/core.yaml) configuration file to set the `security.privacy` value to `true` as well.
Alternatively, you can enable privacy by running the peer with the following
environment variable: `CORE_SECURITY_PRIVACY=true`. If you are enabling security
and privacy on the peer process with environment variables, it is important to
include these environment variables in the command when executing all subsequent
peer operations (e.g. deploy, invoke, or query).

In a **new** terminal window, from the `devenv` subdirectory of your fabric
workspace environment, `ssh` into Vagrant:

```
cd $GOPATH/src/github.com/hyperledger/fabric/devenv
vagrant ssh
```

Build and run the peer process to enable security and privacy after setting
`security.enabled` and `security.privacy` settings to `true`.

```
cd $GOPATH/src/github.com/hyperledger/fabric
make peer
peer node start --peer-chaincodedev
```

Alternatively, rather than tweaking the `core.yaml` and rebuilding, you can
enable security and privacy on the peer with environment variables:

```
CORE_SECURITY_ENABLED=true CORE_SECURITY_PRIVACY=true peer node start --peer-chaincodedev
```

Now, you are ready to start [running the chaincode](#running-the-chaincode).

## Running the chaincode

### Docker or Docker Toolbox

Start a **new** terminal window.  If you ran spun up your Docker containers in
detached mode - `docker-compose up -d` - you can remain in the same terminal.

If you are using either [Option 1](#option-1-docker-for-mac-or-windows) or
[Option 2](#option-2-docker-toolbox), you'll need to  download the sample
chaincode. The chaincode project must be placed somewhere under the `src`
directory in your local `$GOPATH` as shown below.

```
mkdir -p $GOPATH/src/github.com/chaincode_example02/
cd $GOPATH/src/github.com/chaincode_example02
curl GET https://raw.githubusercontent.com/hyperledger/fabric/master/examples/chaincode/go/chaincode_example02/chaincode_example02.go > chaincode_example02.go
```

Next, you'll need to clone the Hyperledger fabric to your local $GOPATH, so that
you can build your chaincode. **Note:** this is a temporary stop-gap until we
can provide an independent package for the chaincode shim.

```
mkdir -p $GOPATH/src/github.com/hyperledger
cd $GOPATH/src/github.com/hyperledger
git clone http://gerrit.hyperledger.org/r/fabric
```

Now, you should be able to build your chaincode.

```
cd $GOPATH/src/github.com/chaincode_example02
go build
```

When you are ready to start creating your own Go chaincode, create a new
subdirectory under $GOPATH/src. You can copy the **chaincode_example02** file to
the new directory and modify it.

### Vagrant

Start a **new** terminal window.

If you are using [Option 3](#option-3-vagrant-development-environment), you'll
need to `ssh` to Vagrant.

```
cd $GOPATH/src/github.com/hyperledger/fabric/devenv
vagrant ssh
```

Next, we'll build the **chaincode_example02** code, which is provided in the
Hyperledger fabric source code repository. If you are using [Option 3](#option-3-vagrant-development-environment), then you can do this from your
clone of the fabric repository.

```
cd $GOPATH/src/github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02
go build
```

### Starting and registering the chaincode

Run the following chaincode command to start and register the chaincode with the
validating peer:

```
CORE_CHAINCODE_ID_NAME=mycc CORE_PEER_ADDRESS=0.0.0.0:7051 ./chaincode_example02
```

The chaincode console will display the message "Received REGISTERED, ready for
invocations", which indicates that the chaincode is ready to receive requests.
Follow the steps below to send a chaincode deploy, invoke or query transaction.
If the "Received REGISTERED" message is not displayed, then an error has occurred
during the deployment; revisit the previous steps to resolve the issue.

**Note**: These instructions relate to writing, building, and running chaincode
in "development" mode.  This means that if you are using Docker, you will not
see additional Docker containers after you have deployed your chaincode.  Rather,
the chaincode is directly registered with the peer as outlined in the above command.  
See the [Docker Setup Guide](Setup/Docker-setup.md).

## Running the CLI or REST API

  * [chaincode deploy via CLI and REST](#chaincode-deploy-via-cli-and-rest)
  * [chaincode invoke via CLI and REST](#chaincode-invoke-via-cli-and-rest)
  * [chaincode query via CLI and REST](#chaincode-query-via-cli-and-rest)

If you were running with security enabled, see
[Removing temporary files when security is enabled](#removing-temporary-files-when-security-is-enabled)
to learn how to clean up the temporary files.

See the
[logging control](https://github.com/hyperledger/fabric/blob/master/docs/Setup/logging-control.md)
reference for information on controllinglogging output from the `peer` and chaincodes.

### Terminal 3 (CLI or REST API)

#### **Note on REST API port**

The default REST interface port is `7050`. It can be configured in [core.yaml](https://github.com/hyperledger/fabric/blob/master/peer/core.yaml)
using the `rest.address` property. If using Vagrant, the REST port mapping is
defined in
[Vagrantfile](https://github.com/hyperledger/fabric/blob/master/devenv/Vagrantfile).

#### **Note on security functionality**

Current security implementation assumes that end user authentication takes place
at the application layer and is not handled by the fabric. Authentication may be
performed through any means considered appropriate for the target application.
Upon successful user authentication, the application will perform user registration
with the CA exactly once. If registration is attempted a second time for the same
user, an error will result. During registration, the application sends a request
to the certificate authority to verify the user registration and if successful,
the CA responds with the user certificates and keys. The enrollment and transaction
certificates received from the CA will be stored locally inside
`/var/hyperledger/production/crypto/client/` directory. This directory resides
on a specific peer node which allows the user to transact only through this
specific peer while using the stored crypto material. If the end user needs to
perform transactions through more then one peer node, the application is responsible
for replicating the crypto material to other peer nodes. This is necessary as
registering a given user with the CA a second time will fail.

With security enabled, the CLI commands and REST payloads must be modified to
include the `enrollmentID` of a registered user who is logged in; otherwise an
error will result. A registered user can be logged in through the CLI or the
REST API by following the instructions below. To log in through the CLI, issue
the following commands, where `username` is one of the `enrollmentID` values
listed in the `eca.users` section of the [membersrvc.yaml](https://github.com/hyperledger/fabric/blob/master/membersrvc/membersrvc.yaml)
file.

From your command line terminal, move to the `devenv` subdirectory of your workspace
environment. Log into a Vagrant terminal by executing the following command:

```
    vagrant ssh
```

Register the user though the CLI, substituting for `<username>` appropriately:

```
    cd $GOPATH/src/github.com/hyperledger/fabric/peer
    peer network login <username>
```

The command will prompt for a password, which must match the `enrollmentPW`
listed for the target user in the `eca.users` section of the [membersrvc.yaml](https://github.com/hyperledger/fabric/blob/master/membersrvc/membersrvc.yaml)
file. If the password entered does not match the `enrollmentPW`, an error will
result.

To log in through the REST API, send a POST request to the `/registrar` endpoint,
containing the `enrollmentID` and `enrollmentPW` listed in the `eca.users` section
of the
[membersrvc.yaml](https://github.com/hyperledger/fabric/blob/master/membersrvc/membersrvc.yaml) file.

**REST Request:**

```
POST localhost:7050/registrar

{
  "enrollId": "jim",
  "enrollSecret": "6avZQLwcUe9b"
}
```

**REST Response:**

```
200 OK
{
    "OK": "Login successful for user 'jim'."
}
```

#### chaincode deploy via CLI and REST

First, send a chaincode deploy transaction, only once, to the validating peer.
The CLI connects to the validating peer using the properties defined in the
core.yaml file. **Note:** The deploy transaction typically requires a `path`
parameter to locate, build, and deploy the chaincode. However, because these
instructions are specific to local development mode and the chaincode is deployed
manually, the `name` parameter is used instead.
```
peer chaincode deploy -n mycc -c '{Args": ["init", "a","100", "b", "200"]}'
```

Alternatively, you can run the chaincode deploy transaction through the REST API.

**REST Request:**

```
POST <host:port>/chaincode

{
  "jsonrpc": "2.0",
  "method": "deploy",
  "params": {
    "type": 1,
    "chaincodeID":{
        "name": "mycc"
    },
    "ctorMsg": {
        "args":["init", "a", "100", "b", "200"]
    }
  },
  "id": 1
}
```

**REST Response:**

```
{
    "jsonrpc": "2.0",
    "result": {
        "status": "OK",
        "message": "mycc"
    },
    "id": 1
}
```

**Note:** When security is enabled, modify the CLI command and the REST API
payload to pass the `enrollmentID` of a logged in user. To log in a registered
user through the CLI or the REST API, follow the instructions in the
[note on security functionality](#note-on-security-functionality). On the CLI,
the `enrollmentID` is passed with the `-u` parameter; in the REST API, the
`enrollmentID` is passed with the `secureContext` element. If you are enabling
security and privacy on the peer process with environment variables, it is
important to include these environment variables in the command when executing
all subsequent peer operations (e.g. deploy, invoke, or query).
```
  CORE_SECURITY_ENABLED=true CORE_SECURITY_PRIVACY=true peer chaincode deploy -u
  jim -n mycc -c '{"Args": ["init", "a","100", "b", "200"]}'
```
**REST Request:**

```
POST <host:port>/chaincode

{
  "jsonrpc": "2.0",
  "method": "deploy",
  "params": {
    "type": 1,
    "chaincodeID":{
        "name": "mycc"
    },
    "ctorMsg": {
        "args":["init", "a", "100", "b", "200"]
    },
    "secureContext": "jim"
  },
  "id": 1
}
```

The deploy transaction initializes the chaincode by executing a target
initializing function. Though the example shows "init", the name could be
arbitrarily chosen by the chaincode developer. You should see the following
output in the chaincode window:
```
	<TIMESTAMP_SIGNATURE> Received INIT(uuid:005dea42-d57f-4983-803e-3232e551bf61),
  initializing chaincode Aval = 100, Bval = 200
```

#### Chaincode invoke via CLI and REST

Run the chaincode invoking transaction on the CLI as many times as desired. The
`-n` argument should match the value provided in the chaincode window
(started in Vagrant terminal 2):

```
	peer chaincode invoke -l golang -n mycc -c '{Args": ["invoke", "a", "b", "10"]}'
```

Alternatively, run the chaincode invoking transaction through the REST API.

**REST Request:**

```
POST <host:port>/chaincode

{
  "jsonrpc": "2.0",
  "method": "invoke",
  "params": {
      "type": 1,
      "chaincodeID":{
          "name":"mycc"
      },
      "ctorMsg": {
         "args":["invoke", "a", "b", "10"]
      }
  },
  "id": 3
}
```

**REST Response:**

```
{
    "jsonrpc": "2.0",
    "result": {
        "status": "OK",
        "message": "5a4540e5-902b-422d-a6ab-e70ab36a2e6d"
    },
    "id": 3
}
```

**Note:** When security is enabled, modify the CLI command and REST API payload
to pass the `enrollmentID` of a logged in user. To log in a registered user
through the CLI or the REST API, follow the instructions in the [note on security functionality](#note-on-security-functionality). On the CLI, the `enrollmentID`
is passed with the `-u` parameter; in the REST API, the `enrollmentID` is passed
with the `secureContext` element. If you are enabling security and privacy on
the peer process with environment variables, it is important to include these
environment variables in the command when executing all subsequent peer operations
(e.g. deploy, invoke, or query).
```
 	  CORE_SECURITY_ENABLED=true CORE_SECURITY_PRIVACY=true peer chaincode invoke
    -u jim -l golang -n mycc -c '{"Function": "invoke", "Args": ["a", "b", "10"]}'
```
**REST Request:**

```
POST <host:port>/chaincode

{
  "jsonrpc": "2.0",
  "method": "invoke",
  "params": {
      "type": 1,
      "chaincodeID":{
          "name":"mycc"
      },
      "ctorMsg": {
         "args":["invoke", "a", "b", "10"]
      },
      "secureContext": "jim"
  },
  "id": 3
}
```

The invoking transaction runs the specified chaincode function name "invoke"
with the arguments. This transaction transfers 10 units from A to B. You should
see the following output in the chaincode window:

```
	<TIMESTAMP_SIGNATURE> Received RESPONSE. Payload 200,
  Uuid 075d72a4-4d1f-4a1d-a735-4f6f60d597a9 Aval = 90, Bval = 210
```

#### Chaincode query via CLI and REST

Run a query on the chaincode to retrieve the desired values. The `-n` argument
should match the value provided in the chaincode window
(started in Vagrant terminal 2):

```
    peer chaincode query -l golang -n mycc -c '{"Args": ["query", "b"]}'
```

The response should be similar to the following:

```
    {"Name":"b","Amount":"210"}
```

If a name other than "a" or "b" is provided in a query sent to `chaincode_example02`,
you should see an error response similar to the following:

```
    {"Error":"Nil amount for c"}
```

Alternatively, run the chaincode query transaction through the REST API.

**REST Request:**
```
POST <host:port>/chaincode

{
  "jsonrpc": "2.0",
  "method": "query",
  "params": {
      "type": 1,
      "chaincodeID":{
          "name":"mycc"
      },
      "ctorMsg": {
         "args":["query", "a"]
      }
  },
  "id": 5
}
```

**REST Response:**
```
{
    "jsonrpc": "2.0",
    "result": {
        "status": "OK",
        "message": "90"
    },
    "id": 5
}
```

**Note:** When security is enabled, modify the CLI command and REST API payload
to pass the `enrollmentID` of a logged in user. To log in a registered user through
the CLI or the REST API, follow the instructions in the [note on security functionality](#note-on-security-functionality). On the CLI, the `enrollmentID`
is passed with the `-u` parameter; in the REST API, the `enrollmentID` is passed
with the `secureContext` element. If you are enabling security and privacy on
the peer process with environment variables, it is important to include these
environment variables in the command when executing all subsequent peer operations
(e.g. deploy, invoke, or query).

```
 	  CORE_SECURITY_ENABLED=true CORE_SECURITY_PRIVACY=true peer chaincode query
    -u jim -l golang -n mycc -c '{Args": ["query", "b"]}'
```

**REST Request:**
```
POST <host:port>/chaincode

{
  "jsonrpc": "2.0",
  "method": "query",
  "params": {
      "type": 1,
      "chaincodeID":{
          "name":"mycc"
      },
      "ctorMsg": {
         "args":["query", "a"]
      },
      "secureContext": "jim"
  },
  "id": 5
}
```

#### Removing temporary files when security is enabled

**Note:** this step applies **ONLY** if you were using Option 1 above. For
Option 2 or 3, the cleanup is handled by Docker.

After the completion of a chaincode test with security enabled, remove the
temporary files that were created by the CA server process. To remove the client
enrollment certificate, enrollment key, transaction certificate chain, etc., run
the following commands. Note, that you must run these commands if you want to
register a user who has already been registered previously.

From your command line terminal, `ssh` into Vagrant:

```
cd $GOPATH/src/github.com/hyperledger/fabric/devenv
vagrant ssh
```

And then run:

```
rm -rf /var/hyperledger/production
```
