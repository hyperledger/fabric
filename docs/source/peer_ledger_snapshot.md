# Taking ledger snapshots and using them to join channels

For a peer to process transactions on a channel, it must contain the minimum ledger data necessary to endorse and validate transactions consistent with other peers on a channel. This includes the "world state", maintained in the state database, which represents the current value of all of the keys on the ledger (who owns a particular asset, for example) as of the most recently committed block. There are two ways for a peer to get a copy of the necessary ledger data.

1. Join the channel starting with the initial configuration block (known as the "genesis block"), and continue pulling blocks from the ordering service and processing them locally until the latest block that has been written to the ledger is reached. In this scenario, the world state is built from the blocks.
2. Join the channel from a "snapshot", which contains the minimum ledger data as of a particular block number, without needing to pull and process the individual blocks.

While the first method represents a more comprehensive way of joining a channel, because of the size of established channels (which can reach many thousands of blocks), it can take a long time for peers to pull and process all the blocks already committed to the channel. Peers that join a channel this way must also store every block since the creation of the channel, increasing storage costs for an organization. Additionally, joining by snapshot will provide a peer with the latest channel configuration, which may be important if the channel configuration has changed since the genesis block. For example, the peer may need the orderer endpoints or CA certificates from the latest channel configuration before it can successfully pull blocks from the ordering service.

In this topic, we'll describe the process for joining a peer to a channel using a snapshot.

## Limitations

While creating a snapshot and using it to join a peer to a channel from a snapshot (also known as a "checkpoint") will take less time and saves on storage costs compared to processing and storing every block on the ledger, there are a few limitations to consider:

* It is not possible for a peer that joins from a snapshot to query blocks (or transactions within blocks) that were committed before the ledger height of the snapshot. Similarly, it is not possible to query the history of a key prior to the snapshot. If the snapshot the peer uses to join a channel from was taken at block 1000, for example, none of the blocks between 0-999 can be queried. Applications attempting to query for the data in these blocks will have to target a peer that contains the relevant block. Because of this, it is likely that organizations will want to keep at least one peer that has all of the historical data, and target this peer for historical queries.
* While endorsements will continue and queries can be submitted, peers taking a snapshot at a particular height will not commit blocks on the channel while the snapshot is being generated. Because taking a snapshot is a resource-intensive operation, the peer might also be slow to endorse transactions or commit blocks on other channels. For these reasons, it is anticipated that snapshots will only be taken when necessary (for example, when a new peer needs a snapshot to join a channel, or when organizations want to verify that no ledger forks have occurred).
* Because the private data between organizations in a channel is likely to be at least somewhat different, private data is not included in the snapshot (hashes of the private data are included, but not the data itself). Peers that join a channel using a snapshot will discover the collections it is a member of and pull the relevant private data from peers that are members of those collections directly. This private data reconciliation will begin after the peer joins the channel, and may take some time.
* The snapshot process does not archive and prune the ledgers of peers that are already joined to the channel. Similarly, it is not meant as a method to take a full backup of a peer, as private data, and peer configuration information, such as MSPs, are not included in a snapshot).
* It is not possible to use the `reset`, `rollback`, or `rebuild-dbs` commands on peers that have joined a channel using a snapshot since the peer would not have all the block files required for the operations. Instead of these administrative commands, it is expected that peers that have joined a channel using a snapshot can be entirely rebuilt from the same or newer snapshots.

## Considerations

* When deciding whether to join peers from a genesis block or from a snapshot, consider the time it may take to join a peer to a channel based on the number of blocks since the genesis block, whether the peer will be able to pull blocks from the ordering service based on the original channel configuration in the genesis block, and whether you will need to query the entire history of a channel (historical blocks, transactions, or state).
* If your endorsement requests or queries don't require the latest block commits, you can target a peer that generates snapshots.
* If your endorsement requests or queries require the latest block commits, you can utilize service discovery to identify and target peers with the highest block heights on a channel, thereby avoiding any peer that is generating a snapshot. Alternatively, you could utilize dedicated peers for snapshot, and not make these peers available for endorsements and queries, for example by not setting `peer.gossip.externalEndpoint` so that the peer does not participate in service discovery.
* You may not want to make a peer available for endorsements and queries until it has joined all the expected channels, and has reconciled all the private data that it is authorized to receive.

## Overview

Snapshots can be used by organizations that already have peers on a channel or by organizations new to a channel. Whatever the use case, the process is largely the same.

1. **Schedule a snapshot**. These snapshots must be taken at **exactly the same ledger height** on each peer. This will allow an organization to evaluate the snapshots to make sure they contain the same data. This ledger height must be equal or higher than the current block height (snapshots scheduled for a higher block height will be taken when the block height is reached). They cannot be taken from a lower block height. If you attempt to schedule a snapshot at a height lower than the current height you will get an error. Note that a peer that already used a snapshot to join a channel can also be used to take a snapshot. Snapshots can be scheduled as needed or there can be an agreed among organizations to take them at a regular cadence, for example every 10,000 blocks. This ensures that consistent and recent snapshots are always available. Note that is is not possible to schedule recurring snapshots. Each snapshot has to be scheduled independently. However, there is no limit to the number of future snapshots that can be scheduled. When joining a peer from a snapshot, it is a good practice to use a snapshot more recent than the latest channel config block height. This ensures that the peer will have the most recent channel configuration including the latest ordering service endpoints and CA certificates.
2. **When the ledger height is reached, the snapshot is taken by the peer**. The snapshot is comprised of a directory that includes files that contain the public state, hashes of private state, transaction IDs, and the collection config history. A file containing metadata relating to these files is also included. For more information, check out [contents of a snapshot](#contents-of-a-snapshot).
3. **If the snapshot will be used by a new organization, the snapshot is sent to them**. This must be completed out of band. Because snapshot files are not compressed, it is likely that peer administrators will want to compress these files before sending them. In a typical scenario, the administrator will receive the snapshot from one of the existing organizations but will want to receive the snapshot metadata from more than one organizations in order to verify the snapshot received.

The organization that will use the snapshot to join the channel will then:

1. **Evaluate the snapshot or snapshots**. An administrator of the peer organization attempting to use the snapshot to join the peer to the channel should independently compute the hashes of the snapshot files and match these with the hashes present in the metadata file. In addition, the administrator may want to match the metadata files from more than one organization, depending on the trust model established by the network. In some scenarios, the administrator may want the administrators of other organizations to sign the metadata file for its records.
2. **Join the peer to the channel using the snapshot**. When the peer has finished joining the channel using the snapshot, it will begin pulling private data according to the collections it is a member of. It will also start committing blocks as normal, starting with any blocks greater than the snapshot height that are available from the ordering service.
3. **Verify the peer has joined the channel successfully**. For more information, check out [Joining a channel using a snapshot](#joining-a-channel-using-a-snapshot).

If an organization that is already joined to the channel wants to join a new peer using a snapshot, it might decide to skip the process of having other organizations take snapshots and evaluate them, though it is a best practice for an organization to periodically take snapshots of all its peers and compare them to ensure the no ledger forks have occurred. In that case, the organization can take a snapshot immediately and then use the snapshot to join the new peer to the channel.

## Using snapshots to verify peer integrity

Snapshots can be used to verify that the state between peers is identical (in other words, that no ledger fork has occurred), even if no new peer will use the snapshot to join the channel.
This can be done by ensuring that the `snapshot_hash` in the file `_snapshot_additional_metadata.json` in the snapshots generated across peers is the same.
If the hashes are not identical, you can use the [`ledgerutil compare` utility](./commands/ledgerutil.html) to troubleshoot which keys are different across any two snapshots and to understand when a divergence may have occurred.

## Taking a snapshot

For the full list of snapshot-related commands, check out [`peer snapshot` commands](./commands/peersnapshot.html#peer-snapshot).

Before taking a snapshot, it is a best practice to confirm the current ledger height by issuing a command similar to:

```
peer channel getinfo -c <name of channel>
```

You will see a response similar to:

```
Blockchain info: {"height":970,"currentBlockHash":"JgK9lcaPUNmFb5Mp1qe1SVMsx3o/22Ct4+n5tejcXCw=","previousBlockHash":"f8lZXoAn3gF86zrFq7L1DzW2aKuabH9Ow6SIE5Y04a4="}
```

In this example, the ledger height is `970`.

A snapshot request can be submitted by issuing a command similar to:

```
peer snapshot submitrequest -c <name of channel> -b <ledger height where snapshot will be taken> --peerAddress <address of peer> --tlsRootCertFile <path to root certificate of the TLS CA>
```

For example:

```
peer snapshot submitrequest -c testchannel -b 1000 --peerAddress 127.0.0.1:22509 --tlsRootCertFile tls/cert.pem
```

**If you give a ledger height of `0`, the snapshot is taken immediately. This is useful for cases when an organization is interested in generating a snapshot that will be used by one of its own peers and does not intend to share the data with another organization. Do not take these "immediate" snapshots in cases when snapshots will be evaluated from multiple peers, as it increases the likelihood that the snapshots will be taken at different ledger heights.**

If the request is successful, you will see a `Snapshot request submitted successfully` message.

You can list the pending snapshots by issuing a command similar to:

```
peer snapshot listpending -c testchannel --peerAddress 127.0.0.1:22509 --tlsRootCertFile tls/cert.pem
```

You will see a response similar to:

```
Successfully got pending snapshot requests [1000]
```

When a snapshot has been generated for a particular block height, the pending request for that block height will no longer appear in the list. You can also verify that a snapshot has been created successfully by looking at the peer logs.

Snapshots will be written to a directory based on the `core.yaml` `ledger.snapshots.rootDir` property. Completed snapshots are written to a subdirectory based on the channel name and block number of the snapshot: `{ledger.snapshots.rootDir}/completed/{channelName}/{lastBlockNumberInSnapshot}`. If the `ledger.snapshots.rootDir` property is not specified in the core.yaml, then the default value is `{peer.fileSystemPath}/snapshots`. If you expect a snapshot will be large, or you expect to share snapshots in the location that they are generated, consider setting the snapshot directory to a different volume than the peer's `fileSystemPath`.

To delete a snapshot request, simply exchange `submitrequest` with `cancelrequest`. For example:

```
peer snapshot cancelrequest -c testchannel -b 1000 --peerAddress 127.0.0.1:22509 --tlsRootCertFile tls/cert.pem
```

If you submit the `listpending` command again, the snapshot should no longer appear.

### Contents of a snapshot

Once the peer generates a snapshot to the `{ledger.snapshots.rootDir}/completed/{channelName}/{lastBlockNumberInSnapshot}` directory, the peer does not use that directory for any purpose and it is safe to compress and transfer the snapshot using external tools, and to delete it when no longer needed.

As mentioned above, the completed snapshot directory contains files for the different data items listed below:

* **Public state**
  * This includes the latest value of all of the keys on the channel. For example, the public state would show an asset (a key) and its current owner (the value), but not any of its historical owners.
* **Private data hashes**
  * This includes hashes of private data transactions on the channel. Recall from our documentation on private data that while the actual transaction data of a private data transaction is not stored on the public ledger of the channel, hashes of the data are stored. This allows the private data to be verified against the hash, for example by an organization that is added to a private data collection. These hashes are included in the snapshot so that new organizations can verify them against the private data they receive for the collections they are a member of.
* **Transactions IDs**
  * This consists of the transaction IDs that have been used in the channel until the last block in the snapshot. The transaction IDs are included so that peers can verify that a transaction ID is not later re-used for another transaction.
* **Collection config history**
  * This contains the history of the collection configurations for all chaincodes. Recall from our documentation on private data that the collection configuration derives the endorsement and dissemination policies for private data collections.

In addition, the snapshot contains two metadata files that help the data in the snapshot to be verified. This snapshot metadata file is expected to be same across snapshots of a channel for a particular height.

One of these files contains a JSON record called `_snapshot_signable_metadata.json` with the following fields:

* `channel_name`: the name of the channel.
* `last_block_number`: the block height when the snapshot was taken.
* `last_block_hash`: a hash of the block at the height the snapshot was taken.
* `previous_block_hash`: a hash of the block prior to the `last_block`.
* `state_db_type` (the value of this field will be either CouchDB or SimpleKeyValueDB (also known as LevelDB).
* `snapshot_files_raw_hashes`, is a JSON record that contains the hashes of the files above.

This metadata file is also a JSON record with the following two fields:

* `snapshot_hash`, the hash of the file `_snapshot_signable_metadata.json` and can be treated as a hash of the snapshot.
* `last_block_commit_hash`, which is included if the snapshot generating peer is equipped to compute the block commit hashes.

Note that the file types explained here is a superset of all of the files that might be included in a snapshot. If admins find some of these file types missing in their snapshots (for example, the collection config history) this is not mean the snapshot is incomplete. The channel might not have any collections.

## Joining a channel using a snapshot

When joining a channel using the genesis block, a command similar to `peer channel join --blockpath mychannel.block` is issued. When joining the peer to the channel using a snapshot, issue a command similar to:

```
peer channel joinbysnapshot --snapshotpath <path to snapshot>
```

To verify that the peer has joined the channel successfully, issue a command similar to:

```
peer channel getinfo -c <name of channel joined by snapshot>
```

Additionally, if the peer has not already installed a chaincode being used on the channel, do so, and then issue a query. A successful return of data indicates that the peer has successfully joined using the snapshot. You can then install all of the chaincodes being used on the channel. If the snapshot is used by a new organization, and the channel contains the definitions for the chaincodes that are defined using the [new chaincode lifecycle](./chaincode_lifecycle.html), the new organization will need to approve the definition of these chaincodes before they can be invoked on the peer.

## Try it out

If you want to try out the ledger snapshotting process, you'll first need a network with a running channel. If you don't have a network, deploy the [test network](./test_network.html). This will create a network with two orgs, which both have a single peer, and an application channel.

Next, follow the [Adding an Org to a Channel](./channel_update_tutorial.html) to add a new org to your network and application channel. When you reach the section where you are asked to [Join Org3 to the Channel](./channel_update_tutorial.html#join-org3-to-the-channel), select the peer you want to use to take the snapshot and follow the instructions above to take the snapshot. Then locate the snapshot on the peer and copy it somewhere else on your filesystem. Taking the snapshot at this step ensures that the new peer joins the channel using a snapshot taken after a point when its organization has already been joined to the channel.

After you have taken the snapshot and copied it, instead of issuing the `peer channel join -b mychannel.block` command, substitute `peer channel joinbysnapshot --snapshotpath <path to snapshot>` using the path to the snapshot on your filesystem.

## Troubleshooting

There are a few reasons why a peer might fail to join a channel using a snapshot:

* The snapshot is not at the location that was specified. Check to make sure the snapshot is in the location you have specified in the `joinbysnapshot` command.
* The hash of the data does not match the data. This can indicate that there was an undetected error during the creation of the snapshot or that the data in the snapshot has been corrupted somehow.
