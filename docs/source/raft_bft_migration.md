# Migrating from Raft to BFT

Nodes at v3.0.0 or higher allow for users who want to transition channels from using Raft-based ordering services to BFT-based ordering services.
This can be accomplished through a series of configuration update transactions on each channel in the network.
To migrate, upgrade from version 2.x to version 3.0.0.

This tutorial will describe the migration process at a high level, calling out specific details where necessary.

## Pre-migration Checklist
Before beginning the migration process, ensure the following:
1. All ordering service nodes are running Fabric v3.0.0 or greater
2. All peers are running at least Fabric v3.0.0
3. All channels are configured with the V3_0 or later channel capability
4. The number of nodes is configured as 3f + 1, where f is the number of tolerated failures
5. BFT metadata and ConsenterMapping are prepared for each channel

## Security Considerations
1. Ensure all communication channels are encrypted
2. Verify node identities and certificates before and after migration
3. Update access controls if necessary after migration

## Assumptions and considerations
Before attempting migration, take the following into account:

1. This process is solely for migration from Raft to BFT. Migrating between any other orderer consensus types is not supported.
2. Migration is one way. Once the ordering service is migrated to BFT, and starts committing transactions, it is not possible to go back to Raft.
3. Because the ordering nodes must go down and be brought back up, downtime must be allowed during the migration.
4. Recovering from a botched migration is possible only if a backup is taken at the point in migration prescribed later in this document. If you do not take a backup, and migration fails, you will be possibly unable to recover your previous state.
5. All channels managed by a targeted ordering service node must be migrated during the same maintenance window. It is not supported to migrate only some channels before resuming operations. If you want to migrate one channel to BFT but keep another channel on Raft, make sure these two channels have no ordering service nodes in common.
6. At the end of the migration process, the set of consenters in a specific channel, prior to migration, should be the same as the set of consenters on that channel after migration.
   That is, addition or removal of consenters from a channel, or changing certificates, is not permitted during migration.
7. Migration is done in place, utilizing the existing ledgers for the deployed ordering nodes. Addition or removal of orderers should be performed after the migration. 


## Migration flow
Migration is carried out in five phases.

1. The system is placed into a maintenance mode where application transactions are rejected and only ordering service admins can make changes to the channel configuration.
2. The system is stopped, and a backup is taken in case an error occurs during migration.
   Comment: backup should be taken at this point since in maintenance mode data tx's are blocked, that's how we ensure that we take a backup of a state which is coherent.
3. The system is started, and each channel has its consensus type and metadata modified.
4. The system is restarted and is now operating on BFT consensus; each channel is checked to confirm that it has successfully achieved a quorum.
5. The system is moved out of maintenance mode back to the normal state and normal function resumes.


## Preparing to migrate

The migration process requires changes in the channel configuration at /Channel/Orderer/ConsensusType and /Channel/Orderer/Orderers.
In a JSON representation of the channel configuration, this would be .channel_group.groups.Orderer.values.ConsensusType and .channel_group.groups.Orderer.values.Orderers respectively.

The `ConsensusType` is represented by three values: `Type`, `Metadata`, and `State`, where:
- `Type` is either Raft (etcdraft) or BFT. This value can only be changed while in maintenance mode.
- `Metadata` must carry valid Raft or BFT metadata.
- `State` is the state of the system and can be one of the following: `STATE_NORMAL`, when the channel is processing transactions, or `STATE_MAINTENANCE` during the migration process.

The `Orderers` is represented by one value, which is `ConsenterMapping`.
Each consenter is represented by the following properties:
* id
* host
* port
* msp_id
* identity
* client_tls_cert
* server_tls_cert

\
Before proceeding with the migration, the following preparatory steps are required:

1.  **Config the number of nodes to be 3f + 1, so the cluster will be functional. (Where f is the number of tolerated failures).**
2. Prepare the BFT `Metadata` channel configuration.
Refer to the [BFT configuration guide](./bft_configuration.html) for more information on the channel configuration fields.

3. Prepare the ConsenterMapping. Ensure all nodes of the current Raft cluster are part of the consenters. 

Note: different channels may receive different BFT `Metadata` and `ConsenterMapping` configuration.

4. Ensure all ordering service nodes are running the same version of Fabric, and that this version is v3.0.0 or greater.
5. Ensure all peers are running at least v3.0.0 of Fabric. Make sure all channels are configured with the channel capability that enables migration to BFT (V3_0 or later).


## Phase 1 - entry to maintenance mode
Prior to setting the ordering service into maintenance mode, it is recommended that the peers and clients of the network be stopped.
Leaving peers or clients up and running is safe, however, because the ordering service will reject all of their requests, their logs will fill with benign but misleading failures.

The only field that has to be changed during this step is in the channel configuration at `/Channel/Orderer/ConsensusType`.
In this step of the channel configuration update, only change the `State` from `STATE_NORMAL` to `STATE_MAINTENANCE`. 
Do not change the `Type` or the `Metadata` field yet. Note that the Type should currently be Raft ("etcdraft").

After updating the `State`, verify that each ordering service node has entered maintenance mode on each of the channels. 
This can be done by fetching the last config block and making sure that on each channel the `Type` is `etcdraft`, the `Metadata` matched the Raft metadata and the `State` is `STATE_MAINTENANCE`.

If the channels have been updated successfully, the ordering service is now ready for backup.

Note:
While in maintenance mode, normal transactions, config updates unrelated to migration, and `Deliver` requests from the peers used to retrieve new blocks are rejected. 
This is done in order to prevent the need to both backup, and if necessary restore, peers during migration, as they only receive updates once migration has successfully completed. 
In other words, we want to keep the ordering service backup point, which is the next step, ahead of the peer’s ledger, in order to be able to perform rollback if needed. 
However, ordering node admins can issue `Deliver` requests (which they need to be able to do in order to continue the migration process).


## Phase 2 - backup 
Shut down all Raft ordering nodes, create a backup of the file system of these servers. Then restart the ordering service nodes.


## Phase 3 - switch to BFT in maintenance mode
This step in the migration process is another channel configuration update for each channel. 
In this configuration update, switch the `Type` to `BFT` while keeping the State in `STATE_MAINTENANCE`, 
and fill in the `Metadata` configuration and the `ConsenterMapping`. 
Keep to set the `Metadata` configuration identically on all channels and keep to set the `ConsenterMapping` with all orderers.

Then, validate that each ordering service node has committed the `ConsensusType` and `Orderers` changes by pulling and inspecting the last config block on each channel.

Note: For each channel, the transaction that changes the `ConsensusType` and `Orderers` must be the last configuration transaction before restarting the nodes in the next step. 
If some other configuration transaction happens after this step, the nodes will most likely crash on restart, or result in undefined behavior.

The log output that shows a migration is detected, for example in a cluster of 4 nodes and channel `testchannel`, is:
```
Detected migration to BFT channel=testchannel node=1
Detected migration to BFT channel=testchannel node=2
Detected migration to BFT channel=testchannel node=3
Detected migration to BFT channel=testchannel node=4
```

## Phase 4 - restart and validate leader
Note: exit of maintenance mode must be done after restart.

After the configuration update from the previous step has been completed on each channel, restart the ordering service nodes. 
They should restart as `BFT` nodes, form a cluster per channel, and elect a leader on each channel.

The log output which proves that a BFT cluster is configured is: ```SmartBFT-v3 is now servicing chain channel=channel-name```

After the restart process had finished, make sure to validate that a leader has been elected on each channel by inspecting the node logs. 
This will confirm that the process has been completed successfully.

When the followers see the leader, the log will show, for each channel:
```
Message from leader channel=channel-name
```

## Phase 5 - switch out of maintenance mode
Perform another channel configuration update on each channel (sending the config update to the same ordering node you have been sending configuration updates to until now), switching the `State` from `STATE_MAINTENANCE` to `STATE_NORMAL`. 
To verify, fetch the last config block from the ordering node, verifying that the `State` is now `STATE_NORMAL`. 
For completeness, verify this on each ordering node.

When this process is completed, the ordering service is now ready to accept all transactions on all channels.
If you stopped your peers and application as recommended, you may now restart them.


## Abort and rollback

If a problem emerges during the migration process **before exiting maintenance
mode**, simply perform the rollback procedure below.

1. Shut down the ordering nodes.
2. Rollback the file system of these servers to the backup taken at maintenance
   mode before changing the `ConsensusType`.
3. Restart said servers, the ordering nodes will bootstrap to Raft in
   maintenance mode.
4. Send a configuration update exiting maintenance mode to continue using Raft
   as your consensus mechanism, or resume the instructions after the point of
   backup and fix the error which prevented to proceed with the process.

There are a few states which might indicate migration has failed:

1. Some nodes crash or shutdown.
2. There is no record of a successful leader election per channel in the logs.
3. The attempt to switch to `STATE_NORMAL` mode fails.
