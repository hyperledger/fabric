## Prerequisites and setup

* [Go](https://golang.org/) - most recent version
* [Docker](https://www.docker.com/products/overview) - v1.13 or higher
* [Docker Compose](https://docs.docker.com/compose/overview/) - v1.8 or higher
* [Node.js & npm](https://nodejs.org/en/download/) - node v6.9.5 and npm v3.10.10
* [xcode](https://developer.apple.com/xcode/) - only required for OS X users
* [nvm](https://github.com/creationix/nvm/blob/master/README.markdown) - if you want to use `nvm install` command
If you already have node on your machine, use the node website to install v6.9.5 or
issue the following command in your terminal:
```bash
nvm install v6.9.5
```
then execute the following to see your versions:
```bash
# should be 6.9.5
node -v
```
AND
```bash
# should be 3.10.10
npm -v
```

## Curl the source code to create network entities

* Download the [cURL](https://curl.haxx.se/download.html) tool if not already installed.
* Determine a location on your local machine where you want to place the Fabric artifacts and application code.
```bash
mkdir -p <my_dev_workspace>/hackfest
cd <my_dev_workspace>/hackfest
```
Next, execute the following command:
```bash
curl -L https://raw.githubusercontent.com/hyperledger/fabric/master/examples/sfhackfest/sfhackfest.tar.gz -o sfhackfest.tar.gz 2> /dev/null;  tar -xvf sfhackfest.tar.gz
```
This command pulls and extracts all of the necessary artifacts to set up your
network - Docker Compose script, channel generate/join script, crypto material
for identity attestation, etc.  In the `/src/github.com/example_cc` directory you
will find the chaincode that will be deployed.

Your directory should contain the following:
```bash
JDoe-mbp: JohnDoe$ pwd
/Users/JohnDoe
JDoe-mbp: JohnDoe$ ls
sfhackfest.tar.gz   channel_test.sh   src
ccenv	  docker-compose-gettingstarted.yml	 tmp
```

## Using Docker

You do not need to manually pull any images.  The images for - `fabric-peer`,
`fabric-orderer`, `fabric-ca`, and `cli` are specified in the .yml file and will
automatically download, extract, and run when you execute the `docker-compose` command.

## Commands

The channel commands are:

* `create` - create and name a channel in the `orderer` and get back a genesis
block for the channel.  The genesis block is named in accordance with the channel name.
* `join` - use the genesis block from the `create` command to issue a join
request to a peer.

## Use Docker to spawn network entities & create/join a channel

Ensure the hyperledger/fabric-ccenv image is tagged as latest:
```bash
docker-compose -f docker-compose-gettingstarted.yml build
```
Create network entities, create channel, join peers to channel:
```bash
docker-compose -f docker-compose-gettingstarted.yml up -d
```
Behind the scenes this started six containers (3 peers, a "solo" orderer, cli and CA)
in detached mode.  A script - `channel_test.sh` - embedded within the
`docker-compose-gettingstarted.yml` issued the create channel and join channel
commands within the CLI container.  In the end, you are left with a network and
a channel containing three peers - peer0, peer1, peer2.

View your containers:
```bash
# if you have no other containers running, you will see six
docker ps
```
Ensure the channel has been created and peers have successfully joined:
```bash
docker exec -it cli bash
```
You should see the following in your terminal:
```bash
/opt/gopath/src/github.com/hyperledger/fabric/peer #
```
To view results for channel creation/join:
```bash
more results.txt
```
You're looking for:
```bash
SUCCESSFUL CHANNEL CREATION
SUCCESSFUL JOIN CHANNEL on PEER0
SUCCESSFUL JOIN CHANNEL on PEER1
SUCCESSFUL JOIN CHANNEL on PEER2
```

To view genesis block:
```bash
more myc1.block
```

Exit the cli container:
```bash
exit
```

## Curl the application source code and SDK modules

* Prior to issuing the command, make sure you are in the same working directory
where you curled the network code.  AND make sure you have exited the cli container.
* Execute the following command:
```bash
curl -OOOOOO https://raw.githubusercontent.com/hyperledger/fabric-sdk-node/v1.0-alpha/examples/balance-transfer/{config.json,deploy.js,helper.js,invoke.js,query.js,package.json}
```

This command pulls the javascript code for issuing your deploy, invoke and query calls.
It also retrieves dependencies for the node SDK modules.

* Install the node modules:
```bash
# You may be prompted for your root password at one or more times during this process.
npm install
```
You now have all of the necessary prerequisites and Fabric artifacts.
