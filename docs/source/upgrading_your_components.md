# Upgrading your components

*Audience: network administrators, node administrators*

For information about special considerations for the latest release of Fabric,
check out [Upgrading to the latest release of Fabric](./upgrade_to_newest_version.html).

This topic will only cover the process for upgrading components. For information
about how to edit a channel to change the capability level of your channels,
check out [Updating a channel capability](./updating_capabilities.html).

Note: when we use the term “upgrade” in Hyperledger Fabric, we’re referring to changing
the version of a component (for example, going from one version of a binary to
the next version). The term “update,” on the other hand, refers not to versions
but to configuration changes, such as updating a channel configuration or a
deployment script. As there is no data migration, technically speaking, in Fabric,
we will not use the term "migration" or "migrate" here.

## Overview

At a high level, upgrading the binary level of your nodes is a two step process:

1. Backup the ledger and MSPs.
2. Upgrade binaries to the latest version.

If you own both ordering nodes and peers, it is a best practice to upgrade the
ordering nodes first. If a peer falls behind or is temporarily unable to process
certain transactions, it can always catch up. If enough ordering nodes go down,
by comparison, a network can effectively cease to function.

This topic presumes that these steps will be performed using CLI commands. If
you are utilizing a different deployment method (Rancher, Kubernetes, OpenShift,
etc) consult their documentation on how to use their CLI.

Note that for native and production deployments, you will also need to update
the YAML configuration file for the nodes (for example, the `orderer.yaml` file)
with the one from the release artifacts.

To do this, backup the `orderer.yaml` or `core.yaml` file (for the peer) and
replace it with the `orderer.yaml` or `core-yaml` file from the release artifacts.
Then port any modified variables from the backed up `orderer.yaml` or `core.yaml`
to the new one. Using a utility like `diff` may be helpful. Note that updating
the YAML file from the release rather than updating your old YAML file **is the
recommended way to update your node YAML files**, as it reduces the likelihood
of making errors.

We will talk more about these configuration files in the relevant steps.

### Ledger backup and restore

While we will demonstrate the process for backing up ledger data in this tutorial,
it is not strictly required to backup the ledger data of a peer or an ordering
node (assuming the node is part of a larger group of nodes in an ordering service).
This is because, even in the worst case of catastrophic failure of a peer (such
as a disk failure), the peer can be brought up with no ledger at all. You can
then have the peer re-join the desired channels and as a result, the peer will
automatically create a ledger for each of the channels and will start receiving
the blocks via regular block transfer mechanism from either the ordering service
or the other peers in the channel. As the peer processes blocks, it will also build
up its state database.

Still, some organizations may want to be more self-reliant and want to backup
their ledger data so as to be able to recover from catastrophic failures and
resume block processing from the point of the last backup. In addition, ledger
data backups may help to expedite the addition of a new peer, which can be
achieved by backing up the ledger data from one peer and starting the new peer
with the backed up ledger data.

This tutorial presumes that the file path to the ledger data has not been changed
from the default value of `/var/hyperledger/production/<node>`. If this location
has been changed for your nodes, enter the path to the data on your ledgers in
the commands below.

Note that there will be data for both the ledger and chaincodes at this file
location. While it is a best practice to backup both, it is possible to skip
the `stateLeveldb`, `historyLeveldb`, `chains/index` folders at
`/var/hyperledger/production/<node>/ledgersData`. While skipping these folders
reduces the storage needed for the backup, the peer recovery from the backed up
data may take more time as these ledger artifacts will be re-constructed when the
peer starts.

If using CouchDB as state database, there will be no `stateLeveldb` directory,
as the state database data would stored within CouchDB instead. But similarly,
if peer starts up and finds CouchDB databases are missing or at lower block height
(based on using an older CouchDB backup), the state database will be automatically
re-constructed to catch up to current block height. Therefore, if you backup peer
ledger data and CouchDB data separately, ensure that the CouchDB backup is always
older than the peer backup.

## Prerequisites

Before attempting to upgrade components, make sure you have all of the dependencies
on your machine as described in [Prerequisites](./prereqs.html). This will not only ensure that
you have the latest binaries, but the latest version of all dependencies.

Then make sure you're in the right branch of the Fabric repo:

```
git fetch origin

git checkout <version you're upgrading to>
```

Also, make sure you have TLS enabled by issuing:

```
CORE_PEER_TLS_ENABLED=true
```

## Upgrade ordering nodes

Orderer containers should be upgraded in a rolling fashion (one at a time). At a
high level, the ordering node upgrade process goes as follows:

1. Stop the ordering node.
2. Back up the ordering node's ledger and MSP.
3. Restart the ordering node with the latest images.

Repeat this process for each node in your ordering service until the entire
ordering service has been upgraded.

### Set environment variables

Export the following environment variables before attempting to upgrade your
ordering nodes. Note that the use of `CORE_PEER` in these environment variables
is not an error.

* `ORDERER_ADDRESS`: the url of your ordering node. Note that you will
  need to export this variable for each node when upgrading it.
* `LEDGERS_BACKUP`: the place in your local filesystem where you want to store
  the ledger being backed up. As you will see below, each node being backed up
  will have its own subfolder containing its ledger. You will need to create this
  folder.
* `IMAGE_TAG`: The version you are upgrading to. This should have a prefix of
  `(go env GOARCH)`. For example `export IMAGE_TAG=$(go env GOARCH)-<version you are upgrading to>`.

### Upgrade containers

If you have a native deployment, you should copy your `orderer.yaml` file now.
Later, you will need this file to compare against the `orderer.yaml` file that
is bundled with the release.

Let’s begin the upgrade process by **bringing down the orderer**:

```
docker stop $ORDERER_ADDRESS
```

Once the orderer is down, you'll want to **backup its ledger and MSP**:

```
docker cp $ORDERER_ADDRESS:/var/hyperledger/production/orderer/ ./$LEDGERS_BACKUP/$ORDERER_ADDRESS
```

Now **restart the ordering node** with our new image.

For native deployments, this is where you will need to compare the `orderer.yaml`
file bundled with the release against the `orderer.yaml` you copied earlier. A
tool like `diff` can be helpful here. Once you have copied over the relevant
differences, you can start the node:

```
peer node start
```

Note: as with the environment variables, the use of `peer` is correct here.

Once the ordering node comes up, verify that it is working correctly by querying
the ledger. The ordering node should detect that it has been down and pull any
blocks it has missed.

## Upgrade the peers

Peers should, like the ordering nodes, be upgraded in a rolling fashion (one at
a time). As mentioned during the ordering node upgrade, ordering nodes and peers may be upgraded in parallel, but for
the purposes of this tutorial we’ve separated the processes out. At a high level,
we will perform the following steps:

1. Stop the peer.
2. Back up the peer’s ledger and MSP.
3. Remove chaincode containers and images.
4. Restart the peer with latest image.
5. Verify upgrade completion.

### Environment variables

Export the following environment variables before attempting to upgrade your peers.

* `CORE_PEER_ADDRESS`: the url of your peer. Note that you will need to set this
  variable for each node.
* `LEDGERS_BACKUP`: the place in your local filesystem where you want to store
  the ledger being backed up. As you will see below, each node being backed up
  will have its own subfolder containing its ledger. You will need to create this
  folder.
* `IMAGE_TAG`: the version you are upgrading to.

Repeat this process for each of your peers until every node has been upgraded.

### Upgrade containers

If you have a native deployment, you should copy your `core.yaml` file now.
Later, you will need this file to compare against the `core.yaml` file that
is bundled with the release.

Let’s **bring down the first peer** with the following command:

```
docker stop $CORE_PEER_ADDRESS
```

We can then **backup the peer’s ledger and MSP**:

```
docker cp $CORE_PEER_ADDRESS:/var/hyperledger/production ./$LEDGERS_BACKUP/$CORE_PEER_ADDRESS
```

With the peer stopped and the ledger backed up, **remove the peer chaincode
containers**:

```
CC_CONTAINERS=$(docker ps | grep dev-$CORE_PEER_ADDRESS | awk '{print $1}')
if [ -n "$CC_CONTAINERS" ] ; then docker rm -f $CC_CONTAINERS ; fi
```

And the peer chaincode images:

```
CC_IMAGES=$(docker images | grep dev-$PEER | awk '{print $1}')
if [ -n "$CC_IMAGES" ] ; then docker rmi -f $CC_IMAGES ; fi
```

Now we'll re-launch the peer using the relevant image tag.

For native deployments, this is where you will need to compare the `core.yaml`
file bundled with the release against the `core.yaml` you copied earlier. A tool
like `diff` can be helpful here. Once you have copied over the relevant differences,
you can start the node.

```
peer node start
```

You do not need to relaunch the chaincode container. When the peer gets
a request for a chaincode, (invoke or query), it first checks if it has
a copy of that chaincode running. If so, it uses it. Otherwise, as in
this case, the peer launches the chaincode (rebuilding the image if required).

### Verify peer upgrade completion

It's a best practice to ensure the upgrade has been completed properly with a
chaincode invoke. Note that it should be possible to verify that a single peer
has been successfully updated by querying one of the ledgers hosted on the peer.
If you want to verify that multiple peers have been upgraded, and are updating
your chaincode as part of the upgrade process, you should wait until peers from
enough organizations to satisfy the endorsement policy have been upgraded.

Before you attempt this, you may want to upgrade peers from enough organizations
to satisfy your endorsement policy. However, this is only mandatory if you are
updating your chaincode as part of the upgrade process. If you are not updating
your chaincode as part of the upgrade process, it is possible to get endorsements
from peers running at different Fabric versions.

## Upgrade your CAs

To learn how to upgrade your Fabric CA server, click over to the
[CA documentation](http://hyperledger-fabric-ca.readthedocs.io/en/latest/users-guide.html#upgrading-the-server).

## Upgrade Node SDK clients

Upgrade Fabric and Fabric CA before upgrading Node SDK clients.
Fabric and Fabric CA are tested for backwards compatibility with
older SDK clients. While newer SDK clients often work with older
Fabric and Fabric CA releases, they may expose features that
are not yet available in the older Fabric and Fabric CA releases,
and are not tested for full compatibility.

Use NPM to upgrade any `Node.js` client by executing these commands in the
root directory of your application:

```
npm install fabric-client@latest

npm install fabric-ca-client@latest
```

These commands install the new version of both the Fabric client and Fabric-CA
client and write the new versions to `package.json`.

## Upgrading CouchDB

If you are using CouchDB as state database, you should upgrade the peer's
CouchDB at the same time the peer is being upgraded.

To upgrade CouchDB:

1. Stop CouchDB.
2. Backup CouchDB data directory.
3. Install the latest CouchDB binaries or update deployment scripts to use a new
   Docker image.
4. Restart CouchDB.

## Upgrade Node chaincode shim

To move to the new version of the Node chaincode shim a developer would need to:

1. Change the level of `fabric-shim` in their chaincode `package.json` from their
   old level to the new one.
2. Repackage this new chaincode package and install it on all the endorsing peers
   in the channel.
3. Perform an upgrade to this new chaincode. To see how to do this, check out
   [Peer chaincode commands](./commands/peerchaincode.html).

## Upgrade Chaincodes with vendored shim

For information about upgrading the Go chaincode shim specific to the v2.0 release,
check out [Chaincode shim changes](./upgrade_to_newest_version.html#chaincode-shim-changes).

A number of third party tools exist that will allow you to vendor a chaincode
shim. If you used one of these tools, use the same one to update your vendored
chaincode shim and re-package your chaincode.

If your chaincode vendors the shim, after updating the shim version, you must install
it to all peers which already have the chaincode. Install it with the same name, but
a newer version. Then you should execute a chaincode upgrade on each channel where
this chaincode has been deployed to move to the new version.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
