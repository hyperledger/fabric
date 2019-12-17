# Migrating from Kafka to Raft

**Note: this document presumes a high degree of expertise with channel
configuration update transactions. As the process for migration involves
several channel configuration update transactions, do not attempt to migrate
from Kafka to Raft without first familiarizing yourself with the [Add an
Organization to a Channel](channel_update_tutorial.html) tutorial, which
describes the channel update process in detail.**

For users who want to transition channels from using Kafka-based ordering
services to [Raft-based](./orderer/ordering_service.html#Raft) ordering services,
v1.4.2 allows this to be accomplished through a series of configuration update
transactions on each channel in the network.

This tutorial will describe this process at a high level, calling out specific
details where necessary, rather than show each command in detail.

## Assumptions and considerations

Before attempting migration, take the following into account:

1. This process is solely for migration from Kafka to Raft. Migrating between
any other orderer consensus types is not currently supported.

2. Migration is one way. Once the ordering service is migrated to Raft, and
starts committing transactions, it is not possible to go back to Kafka.

3. Because the ordering nodes must go down and be brought back up, downtime must
be allowed during the migration.

4. Recovering from a botched migration is possible only if a backup is taken at
the point in migration prescribed later in this document. If you do not take a
backup, and migration fails, you will not be able to recover your previous state.

5. All channels must be migrated during the same maintenance window. It is not
possible to migrate only some channels before resuming operations.

6. At the end of the migration process, every channel will have the same
consenter set of Raft nodes. This is the same consenter set that will exist in
the ordering system channel. This makes it possible to diagnose a successful
migration.

7. Migration is done in place, utilizing the existing ledgers for the deployed
ordering nodes. Addition or removal of orderers should be performed after the
migration.

## High level migration flow

Migration is carried out in five phases.

1. The system is placed into a maintenance mode where application transactions
   are rejected and only ordering service admins can make changes to the channel
   configuration.
2. The system is stopped, and a backup is taken in case an error occurs during
   migration.
3. The system is started, and each channel has its consensus type and metadata
   modified.
4. The system is restarted and is now operating on Raft consensus; each channel
   is checked to confirm that it has successfully achieved a quorum.
5. The system is moved out of maintenance mode and normal function resumes.

## Preparing to migrate

There are several steps you should take before attempting to migrate.

* Design the Raft deployment, deciding which ordering service nodes are going to
  remain as Raft consenters. You should deploy at least three ordering nodes in
  your cluster, but note that deploying a consenter set of at least five nodes
  will maintain high availability should a node goes down, whereas a three node
  configuration will lose high availability once a single node goes down for any
  reason (for example, as during a maintenance cycle).
* Prepare the material for
  building the Raft `Metadata` configuration. **Note: all the channels should receive
  the same Raft `Metadata` configuration**. Refer to the [Raft configuration guide](raft_configuration.html)
  for more information on these fields. Note: you may find it easiest to bootstrap
  a new ordering network with the Raft consensus protocol, then copy and modify
  the consensus metadata section from its config. In any case, you will need
  (for each ordering node):
  - `hostname`
  - `port`
  - `server certificate`
  - `client certificate`
* Compile a list of all channels (system and application) in the system. Make
  sure you have the correct credentials to sign the configuration updates. For
  example, the relevant ordering service admin identities.
* Ensure all ordering service nodes are running the same version of Fabric, and
  that this version is v1.4.2 or greater.
* Ensure all peers are running at least v1.4.2 of Fabric. Make sure all channels
  are configured with the channel capability that enables migration.
  - Orderer capability `V1_4_2` (or above).
  - Channel capability `V1_4_2` (or above).

### Entry to maintenance mode

Prior to setting the ordering service into maintenance mode, it is recommended
that the peers and clients of the network be stopped. Leaving peers or clients
up and running is safe, however, because the ordering service will reject all of
their requests, their logs will fill with benign but misleading failures.

Follow the process in the [Add an Organization to a Channel](channel_update_tutorial.html)
tutorial to pull, translate, and scope the configuration of **each channel,
starting with the system channel**. The only field you should change during
this step is in the channel configuration at `/Channel/Orderer/ConsensusType`.
In a JSON representation of the channel configuration, this would be
`.channel_group.groups.Orderer.values.ConsensusType​`.

The `ConsensusType` is represented by three values: `Type`, `Metadata`, and
`State`, where:

  * `Type` is either `kafka` or `etcdraft` (Raft). This value can only be
     changed while in maintenance mode.
  * `Metadata` will be empty if the `Type` is kafka, but must carry valid Raft
     metadata if the `ConsensusType` is `etcdraft`. More on this below.
  * `State` is either `STATE_NORMAL`, when the channel is processing transactions, or
    `STATE_MAINTENANCE`, during the migration process.

In the first step of the channel configuration update, only change the `State`
from `STATE_NORMAL` to `STATE_MAINTENANCE`. Do not change the `Type` or the `Metadata` field
yet. Note that the `Type` should currently be `kafka`.

While in maintenance mode, normal transactions, config updates unrelated to
migration, and `Deliver` requests from the peers used to retrieve new blocks are
rejected. This is done in order to prevent the need to both backup, and if
necessary restore, peers during migration, as they only receive updates once
migration has successfully completed. In other words, we want to keep the
ordering service backup point, which is the next step, ahead of the peer’s ledger,
in order to be able to perform rollback if needed. However, ordering node admins
can issue `Deliver` requests (which they need to be able to do in order to
continue the migration process).

**Verify** that each ordering service node has entered maintenance mode on each
of the channels. This can be done by fetching the last config block and making
sure that the `Type`, `Metadata`, `State` on each channel is `kafka`, empty
(recall that there is no metadata for Kafka), and `STATE_MAINTENANCE`, respectively.

If the channels have been updated successfully, the ordering service is now
ready for backup.

#### Backup files and shut down servers

Shut down all ordering nodes, Kafka servers, and Zookeeper servers. It is
important to **shutdown the ordering service nodes first**. Then, after allowing
the Kafka service to flush its logs to disk (this typically takes about 30
seconds, but might take longer depending on your system), the Kafka servers
should be shut down. Shutting down the Kafka brokers at the same time as the
orderers can result in the filesystem state of the orderers being more recent
than the Kafka brokers which could prevent your network from starting.

Create a backup of the file system of these servers. Then restart the Kafka
service and then the ordering service nodes.

### Switch to Raft in maintenance mode

The next step in the migration process is another channel configuration update
for each channel. In this configuration update, switch the `Type` to `etcdraft`
(for Raft) while keeping the `State` in `STATE_MAINTENANCE`, and fill in the
`Metadata` configuration​. It is highly recommended that the `Metadata` configuration​ be
identical​ on all channels. If you want to establish different consenter sets
with different nodes, you will be able to reconfigure the `Metadata` configuration​
after the system is restarted into `etcdraft` mode. Supplying an identical metadata
object, and hence, an identical consenter set, means that when the nodes are
restarted, if the system channel forms a quorum and can exit maintenance mode,
other channels will likely be able do the same. Supplying different consenter
sets to each channel can cause one channel to succeed in forming a cluster while
another channel will fail.

Then, validate that each ordering service node has committed the `ConsensusType`
change configuration update by pulling and inspecting the configuration of each
channel.

Note: For each channel, the transaction that changes the `ConsensusType` must be the last
configuration transaction before restarting the nodes (in the next step). If
some other configuration transaction happens after this step, the nodes will
most likely crash on restart, or result in undefined behavior.

#### Restart and validate leader

Note: exit of maintenance mode **must** be done **after** restart.

After the `ConsensusType` update has been completed on each channel, stop all
ordering service nodes, stop all Kafka brokers and Zookeepers, and then restart
only the ordering service nodes. They should restart as Raft nodes, form a cluster per
channel, and elect a leader on each channel.

**Note**: Since Raft-based ordering service requires mutual TLS between orderer nodes,
**additional configurations** are required before you start them again, see
[Section: Local Configuration](./raft_configuration.md#local-configuration) for more details.

After restart process finished, make sure to **validate** that a
leader has been elected on each channel by inspecting the node logs (you can see
what to look for below). This will confirm that the process has been completed
successfully.

When a leader is elected, the log will show, for each channel:

``` ​
"Raft leader changed: 0 -> ​node-number​ ​channel=​channel-name​
node=​node-number​ ​"
```

For example:

```
2019-05-26 10:07:44.075 UTC [orderer.consensus.etcdraft] serveRequest ->
INFO 047 Raft leader changed: 0 -> 1 channel=testchannel1 node=2
```

In this example `​node 2​` reports that a leader was elected (the leader is
​`node 1`​) by the cluster of channel `​testchannel1​`.

### Switch out of maintenance mode

Perform another channel configuration update on each channel (sending the config
update to the same ordering node you have been sending configuration updates to
until now), switching the `State` from `STATE_MAINTENANCE` to `STATE_NORMAL`. Start with the
system channel, as usual. If it succeeds on the ordering system channel,
migration is likely to succeed on all channels. To verify, fetch the last config
block of the system channel from the ordering node, verifying that the `State`
is now `STATE_NORMAL`. For completeness, verify this on each ordering node.

When this process is completed, the ordering service is now ready to accept all
transactions on all channels. If you stopped your peers and application as
recommended, you may now restart them.

## Abort and rollback

If a problem emerges during the migration process **before exiting maintenance
mode**, simply perform the rollback procedure below.

1. Shut down the ordering nodes and the Kafka service (servers and Zookeeper
   ensemble).
2. Rollback the file system of these servers to the backup taken at maintenance
   mode before changing the `ConsensusType`.
3. Restart said servers, the ordering nodes will bootstrap to Kafka in
   maintenance mode.
4. Send a configuration update exiting maintenance mode to continue using Kafka
   as your consensus mechanism, or resume the instructions after the point of
   backup and fix the error which prevented a Raft quorum from forming and retry
   migration with corrected Raft configuration `Metadata`.

There are a few states which might indicate migration has failed:

1. Some nodes crash or shutdown.
2. There is no record of a successful leader election per channel in the logs.
3. The attempt to flip to `STATE_NORMAL` mode on the system channel fails.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/) -->