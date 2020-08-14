# Configuring and operating a Raft ordering service

**Audience**: *Raft ordering node admins*

## Conceptual overview

For a high level overview of the concept of ordering and how the supported
ordering service implementations (including Raft) work at a high level, check
out our conceptual documentation on the [Ordering Service](./orderer/ordering_service.html).

To learn about the process of setting up an ordering node --- including the
creation of a local MSP and the creation of a genesis block --- check out our
documentation on [Setting up an ordering node](orderer_deploy.html).

## Configuration

While every Raft node must be added to the system channel, a node does not need
to be added to every application channel. Additionally, you can remove and add a
node from a channel dynamically without affecting the other nodes, a process
described in the Reconfiguration section below.

Raft nodes identify each other using TLS pinning, so in order to impersonate a
Raft node, an attacker needs to obtain the **private key** of its TLS
certificate. As a result, it is not possible to run a Raft node without a valid
TLS configuration.

A Raft cluster is configured in two planes:

  * **Local configuration**: Governs node specific aspects, such as TLS
  communication, replication behavior, and file storage.

  * **Channel configuration**: Defines the membership of the Raft cluster for the
  corresponding channel, as well as protocol specific parameters such as heartbeat
  frequency, leader timeouts, and more.

Recall, each channel has its own instance of a Raft protocol running. Thus, a
Raft node must be referenced in the configuration of each channel it belongs to
by adding its server and client TLS certificates (in `PEM` format) to the channel
config. This ensures that when other nodes receive a message from it, they can
securely confirm the identity of the node that sent the message.

The following section from `configtx.yaml` shows three Raft nodes (also called
“consenters”) in the channel:

```
       Consenters:
            - Host: raft0.example.com
              Port: 7050
              ClientTLSCert: path/to/ClientTLSCert0
              ServerTLSCert: path/to/ServerTLSCert0
            - Host: raft1.example.com
              Port: 7050
              ClientTLSCert: path/to/ClientTLSCert1
              ServerTLSCert: path/to/ServerTLSCert1
            - Host: raft2.example.com
              Port: 7050
              ClientTLSCert: path/to/ClientTLSCert2
              ServerTLSCert: path/to/ServerTLSCert2
```

Note: an orderer will be listed as a consenter in the system channel as well as
any application channels they're joined to.

When the channel config block is created, the `configtxgen` tool reads the paths
to the TLS certificates, and replaces the paths with the corresponding bytes of
the certificates.

### Local configuration

The `orderer.yaml` has two configuration sections that are relevant for Raft
orderers:

**Cluster**, which determines the TLS communication configuration. And
**consensus**, which determines where Write Ahead Logs and Snapshots are
stored.

**Cluster parameters:**

By default, the Raft service is running on the same gRPC server as the client
facing server (which is used to send transactions or pull blocks), but it can be
configured to have a separate gRPC server with a separate port.

This is useful for cases where you want TLS certificates issued by the
organizational CAs, but used only by the cluster nodes to communicate among each
other, and TLS certificates issued by a public TLS CA for the client facing API.

  * `ClientCertificate`, `ClientPrivateKey`: The file path of the client TLS certificate
  and corresponding private key.
  * `ListenPort`: The port the cluster listens on. If blank, the port is the same
  port as the orderer general port (`general.listenPort`)
  * `ListenAddress`: The address the cluster service is listening on.
  * `ServerCertificate`, `ServerPrivateKey`: The TLS server certificate key pair
  which is used when the cluster service is running on a separate gRPC server
  (different port).
  * `SendBufferSize`: Regulates the number of messages in the egress buffer.

Note: `ListenPort`, `ListenAddress`, `ServerCertificate`, `ServerPrivateKey` must
be either set together or unset together.
If they are unset, they are inherited from the general TLS section,
in example `general.tls.{privateKey, certificate}`.

There are also hidden configuration parameters for `general.cluster` which can be
used to further fine tune the cluster communication or replication mechanisms:

  * `DialTimeout`, `RPCTimeout`: Specify the timeouts of creating connections and
  establishing streams.
  * `ReplicationBufferSize`: the maximum number of bytes that can be allocated
  for each in-memory buffer used for block replication from other cluster nodes.
  Each channel has its own memory buffer. Defaults to `20971520` which is `20MB`.
  * `PullTimeout`: the maximum duration the ordering node will wait for a block
  to be received before it aborts. Defaults to five seconds.
  * `ReplicationRetryTimeout`: The maximum duration the ordering node will wait
  between two consecutive attempts. Defaults to five seconds.
  * `ReplicationBackgroundRefreshInterval`: the time between two consecutive
  attempts to replicate existing channels that this node was added to, or
  channels that this node failed to replicate in the past. Defaults to five
  minutes.
  * `TLSHandshakeTimeShift`: If the TLS certificates of the ordering nodes
  expire and are not replaced in time (see TLS certificate rotation below),
   communication between them cannot be established, and it will be impossible
   to send new transactions to the ordering service.
   To recover from such a scenario, it is possible to make TLS handshakes
   between ordering nodes consider the time to be shifted backwards a given
   amount that is configured to `TLSHandshakeTimeShift`.
   This setting only applies when a separate cluster listener is in use.  If
   the cluster service is sharing the orderer's main gRPC server, then instead
   specify `TLSHandshakeTimeShift` in the `General.TLS` section.

**Consensus parameters:**

  * `WALDir`: the location at which Write Ahead Logs for `etcd/raft` are stored.
  Each channel will have its own subdirectory named after the channel ID.
  * `SnapDir`: specifies the location at which snapshots for `etcd/raft` are stored.
  Each channel will have its own subdirectory named after the channel ID.

There is also a hidden configuration parameter that can be set by adding it to
the consensus section in the `orderer.yaml`:

  * `EvictionSuspicion`: The cumulative period of time of channel eviction
  suspicion that triggers the node to pull blocks from other nodes and see if it
  has been evicted from the channel in order to confirm its suspicion. If the
  suspicion is confirmed (the inspected block doesn't contain the node's TLS
  certificate), the node halts its operation for that channel. A node suspects
  its channel eviction when it doesn't know about any elected leader nor can be
  elected as leader in the channel. Defaults to 10 minutes.

### Channel configuration

Apart from the (already discussed) consenters, the Raft channel configuration has
an `Options` section which relates to protocol specific knobs. It is currently
not possible to change these values dynamically while a node is running. The
node have to be reconfigured and restarted.

The only exceptions is `SnapshotIntervalSize`, which can be adjusted at runtime.

Note: It is recommended to avoid changing the following values, as a misconfiguration
might lead to a state where a leader cannot be elected at all (i.e, if the
`TickInterval` and `ElectionTick` are extremely low). Situations where a leader
cannot be elected are impossible to resolve, as leaders are required to make
changes. Because of such dangers, we suggest not tuning these parameters for most
use cases.

  * `TickInterval`: The time interval between two `Node.Tick` invocations.
  * `ElectionTick`: The number of `Node.Tick` invocations that must pass between
  elections. That is, if a follower does not receive any message from the leader
  of current term before `ElectionTick` has elapsed, it will become candidate
  and start an election.
  * `ElectionTick` must be greater than `HeartbeatTick`.
  * `HeartbeatTick`: The number of `Node.Tick` invocations that must pass between
  heartbeats. That is, a leader sends heartbeat messages to maintain its
  leadership every `HeartbeatTick` ticks.
  * `MaxInflightBlocks`: Limits the max number of in-flight append blocks during
  optimistic replication phase.
  * `SnapshotIntervalSize`: Defines number of bytes per which a snapshot is taken.

## Reconfiguration

The Raft orderer supports dynamic (meaning, while the channel is being serviced)
addition and removal of nodes as long as only one node is added or removed at a
time. Note that your cluster must be operational and able to achieve consensus
before you attempt to reconfigure it. For instance, if you have three nodes, and
two nodes fail, you will not be able to reconfigure your cluster to remove those
nodes. Similarly, if you have one failed node in a channel with three nodes, you
should not attempt to rotate a certificate, as this would induce a second fault.
As a rule, you should never attempt any configuration changes to the Raft
consenters, such as adding or removing a consenter, or rotating a consenter's
certificate unless all consenters are online and healthy.

If you do decide to change these parameters, it is recommended to only attempt
such a change during a maintenance cycle. Problems are most likely to occur when
a configuration is attempted in clusters with only a few nodes while a node is
down. For example, if you have three nodes in your consenter set and one of them
is down, it means you have two out of three nodes alive. If you extend the cluster
to four nodes while in this state, you will have only two out of four nodes alive,
which is not a quorum. The fourth node won't be able to onboard because nodes can
only onboard to functioning clusters (unless the total size of the cluster is
one or two).

So by extending a cluster of three nodes to four nodes (while only two are
alive) you are effectively stuck until the original offline node is resurrected.

Adding a new node to a Raft cluster is done by:

  1. **Adding the TLS certificates** of the new node to the channel through a
  channel configuration update transaction. Note: the new node must be added to
  the system channel before being added to one or more application channels.
  2. **Fetching the latest config block** of the system channel from an orderer node
  that's part of the system channel.
  3. **Ensuring that the node that will be added is part of the system channel**
  by checking that the config block that was fetched includes the certificate of
  (soon to be) added node.
  4. **Starting the new Raft node** with the path to the config block in the
  `General.BootstrapFile` configuration parameter.
  5. **Waiting for the Raft node to replicate the blocks** from existing nodes for
  all channels its certificates have been added to. After this step has been
  completed, the node begins servicing the channel.
  6. **Adding the endpoint** of the newly added Raft node to the channel
  configuration of all channels.

It is possible to add a node that is already running (and participates in some
channels already) to a channel while the node itself is running. To do this, simply
add the node’s certificate to the channel config of the channel. The node will
autonomously detect its addition to the new channel (the default value here is
five minutes, but if you want the node to detect the new channel more quickly,
reboot the node) and will pull the channel blocks from an orderer in the
channel, and then start the Raft instance for that chain.

After it has successfully done so, the channel configuration can be updated to
include the endpoint of the new Raft orderer.

Removing a node from a Raft cluster is done by:

  1. Removing its endpoint from the channel config for all channels, including
  the system channel controlled by the orderer admins.
  2. Removing its entry (identified by its certificates) from the channel
  configuration for all channels. Again, this includes the system channel.
  3. Shut down the node.

Removing a node from a specific channel, but keeping it servicing other channels
is done by:

  1. Removing its endpoint from the channel config for the channel.
  2. Removing its entry (identified by its certificates) from the channel
  configuration.
  3. The second phase causes:
     * The remaining orderer nodes in the channel to cease communicating with
     the removed orderer node in the context of the removed channel. They might
     still be communicating on other channels.
     * The node that is removed from the channel would autonomously detect its
     removal either immediately or after `EvictionSuspicion` time has passed
     (10 minutes by default) and will shut down its Raft instance.

### TLS certificate rotation for an orderer node

All TLS certificates have an expiration date that is determined by the issuer.
These expiration dates can range from 10 years from the date of issuance to as
little as a few months, so check with your issuer. Before the expiration date,
you will need to rotate these certificates on the node itself and every channel
the node is joined to, including the system channel.

For each channel the node participates in:

  1. Update the channel configuration with the new certificates.
  2. Replace its certificates in the file system of the node.
  3. Restart the node.

Because a node can only have a single TLS certificate key pair, the node will be
unable to service channels its new certificates have not been added to during
the update process, degrading the capacity of fault tolerance. Because of this,
**once the certificate rotation process has been started, it should be completed
as quickly as possible.**

If for some reason the rotation of the TLS certificates has started but cannot
complete in all channels, it is advised to rotate TLS certificates back to
what they were and attempt the rotation later.

### Certificate expiration related authentication
Whenever a client with an identity that has an expiration date (such as an identity based on an x509 certificate)
sends a transaction to the orderer, the orderer checks whether its identity has expired, and if
so, rejects the transaction submission.

However, it is possible to configure the orderer to ignore expiration of identities via enabling
the `General.Authentication.NoExpirationChecks` configuration option in the `orderer.yaml`.

This should be done only under extreme circumstances, where the certificates of the administrators
have expired, and due to this it is not possible to send configuration updates to replace the administrator
certificates with renewed ones, because the config transactions signed by the existing administrators
are now rejected because they have expired.
After updating the channel it is recommended to change back to the default configuration which enforces
expiration checks on identities.


## Metrics

For a description of the Operations Service and how to set it up, check out
[our documentation on the Operations Service](operations_service.html).

For a list at the metrics that are gathered by the Operations Service, check out
our [reference material on metrics](metrics_reference.html).

While the metrics you prioritize will have a lot to do with your particular use
case and configuration, there are two metrics in particular you might want to
monitor:

* `consensus_etcdraft_is_leader`: identifies which node in the cluster is
   currently leader. If no nodes have this set, you have lost quorum.
* `consensus_etcdraft_data_persist_duration`: indicates how long write operations
   to the Raft cluster's persistent write ahead log take. For protocol safety,
   messages must be persisted durably, calling `fsync` where appropriate, before
   they can be shared with the consenter set. If this value begins to climb, this
   node may not be able to participate in consensus (which could lead to a
   service interruption for this node and possibly the network).
* `consensus_etcdraft_cluster_size` and `consensus_etcdraft_active_nodes`: these
   channel metrics help track the "active" nodes (which, as it sounds, are the nodes that
   are currently contributing to the cluster, as compared to the total number of
   nodes in the cluster). If the number of active nodes falls below a majority of
   the nodes in the cluster, quorum will be lost and the ordering service will
   stop processing blocks on the channel.

## Troubleshooting

* The more stress you put on your nodes, the more you might have to change certain
parameters. As with any system, computer or mechanical, stress can lead to a drag
in performance. As we noted in the conceptual documentation, leader elections in
Raft are triggered when follower nodes do not receive either a "heartbeat"
messages or an "append" message that carries data from the leader for a certain
amount of time. Because Raft nodes share the same communication layer across
channels (this does not mean they share data --- they do not!), if a Raft node is
part of the consenter set in many channels, you might want to lengthen the amount
of time it takes to trigger an election to avoid inadvertent leader elections.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/) -->
