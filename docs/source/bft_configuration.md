# Configuring and operating a BFT ordering service

**Audience**: *BFT ordering service admins*

## Conceptual overview

For a high level overview of the concept of ordering and how the supported
ordering service implementations (including BFT) work at a high level, check
out our conceptual documentation on the [Ordering Service](./orderer/ordering_service.md).

To learn about the process of setting up an ordering node, check out our
documentation on [Planning for an ordering service](./deployorderer/ordererplan.html).

## Configuration

A BFT cluster is configured in two places:

* **Local configuration**: Governs node specific aspects, such as TLS
  communication, replication behavior, and file storage.

* **Channel configuration**: Defines the membership of the  BFT cluster for the
  corresponding channel, as well as protocol specific parameters such as timeouts.

Unlike the [Raft ordering service](./raft_configuration.md), where nodes identify each other using TLS pinning, 
BFT nodes identify each other using their enrollment certificate.

Each channel has its own instance of a BFT protocol running. Thus, each
BFT node must be referenced in the configuration of each channel it participates in
by adding its enrollment certificate (in `PEM` format) and MSP ID to the channel
config. 

The following [section](https://github.com/hyperledger/fabric/blob/main/sampleconfig/configtx.yaml) from `configtx.yaml` shows four BFT nodes (also called
“consenters”) in the channel:

```
    ConsenterMapping:
    - ID: 1
      Host: bft0.example.com
      Port: 7050
      MSPID: OrdererOrg1
      Identity: /path/to/identity
      ClientTLSCert: path/to/ClientTLSCert0
      ServerTLSCert: path/to/ServerTLSCert0
    - ID: 2
      Host: bft1.example.com
      Port: 7050
      MSPID: OrdererOrg2
      Identity: /path/to/identity
      ClientTLSCert: path/to/ClientTLSCert1
      ServerTLSCert: path/to/ServerTLSCert1
    - ID: 3
      Host: bft2.example.com
      Port: 7050
      MSPID: OrdererOrg3
      Identity: /path/to/identity
      ClientTLSCert: path/to/ClientTLSCert2
      ServerTLSCert: path/to/ServerTLSCert2
    - ID: 4
      Host: bft3.example.com
      Port: 7050
      MSPID: OrdererOrg4
      Identity: /path/to/identity
      ClientTLSCert: path/to/ClientTLSCert3
      ServerTLSCert: path/to/ServerTLSCert3
```

When the channel config block is created, the `configtxgen` tool reads the paths
to the identities (enrollment certificates) and TLS certificates, and replaces the paths with the corresponding bytes of
the identities and certificates.
Note that the `Identity` refers to the path of the enrollment certificate, and not to the entire MSP directory.

Note: it is possible to remove or add an ordering node to a channel dynamically, a process described in the reconfiguration section below and in more detail in the [reconfiguration tutorial](./create_channel/add_orderer.md).

In addition, note that the [channel capabilities](./capabilities_concept.md) V3.0 flag must be set to true.

### Local configuration

The `orderer.yaml` has two configuration sections that are relevant for BFT
orderers:

**Cluster**, which determines the communication configuration, and
**Consensus**, which determines BFT protocol related configuration.

**Cluster parameters:**

By default, the BFT service is running in the same gRPC server as the client
facing gRPC API (which is used for transaction submission or block retrieval), but it can be
configured to have a separate gRPC server with a separate port.

This is useful for cases where you want TLS certificates issued by the
organizational CAs, but used only by the cluster nodes to communicate among each
other, and TLS certificates issued by a public TLS CA for the client facing API.

* `ClientCertificate`, `ClientPrivateKey`: The file path of the client TLS certificate
  and corresponding private key.
* `ListenPort`: The port the cluster listens on.
  It must be same as `consenters[i].Port` in Channel configuration.
  If blank, the port is the same port as the orderer general port (`general.listenPort`)
* `ListenAddress`: The address the cluster service is listening on.
* `ServerCertificate`, `ServerPrivateKey`: The TLS server certificate key pair
  which is used when the cluster service is running on a separate gRPC server
  (different port).

Note: `ListenPort`, `ListenAddress`, `ServerCertificate`, `ServerPrivateKey` must
be either set together or unset together.
If they are unset, they are inherited from the general TLS section,
in example `general.tls.{privateKey, certificate}`.
When general TLS is disabled:
- Use a different `ListenPort` than the orderer general port
- Properly configure TLS root CAs in the channel configuration.

Currently, if the cluster communication uses a separate listener, then mutual TLS authentication is implicitly enforced,
while if the cluster communication uses the same gRPC server as the client facing gRPC API, it is not implicitly enforced.

There are also hidden configuration parameters for `general.cluster` which can be
used to further fine tune the cluster communication or replication mechanisms:

* `SendBufferSize`: Regulates the number of messages in the egress buffer.
* `DialTimeout`, `RPCTimeout`: Specify the timeouts of creating connections and
  establishing streams.
* `ReplicationBufferSize`: the maximum number of bytes that can be allocated
  for each in-memory buffer used for block replication from other cluster nodes.
  Each channel has its own memory buffer. Defaults to `20971520` which is `20MB`.
* `PullTimeout`: the maximum duration the ordering node will wait for a block
  to be received before it aborts. Defaults to five seconds.
* `ReplicationRetryTimeout`: The maximum duration the ordering node will wait
  between two consecutive attempts. Defaults to five seconds.
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

* `WALDir`: the location at which Write Ahead Logs for BFT are stored.
  Each channel will have its own subdirectory named after the channel ID.

### Channel configuration

Apart from the (already discussed) consenters, the BFT channel configuration has a section which relates to protocol specific knobs.
It is possible to change these values dynamically at runtime, described in the Reconfiguration section below.

* `RequestBatchMaxCount`: The maximal number of requests in a batch. A request batch that reaches this count is proposed immediately.
* `RequestBatchMaxBytes`: The maximal total size of requests in a batch, in bytes. This is also the maximal size of a single request. A request batch that reaches this size is proposed immediately.
* `RequestBatchMaxInterval`: The maximal time interval a request batch can wait before it is proposed. A request batch is accumulating requests until `RequestBatchMaxInterval` had elapsed from the time the batch was first created (i.e. the time the first request was added to it), or until it is of count `RequestBatchMaxCount`, or it reaches `RequestBatchMaxBytes`, whichever occurs first.
* `IncomingMessageBufferSize`: The size of the buffer holding incoming messages before they are processed (maximal number of messages).
* `RequestPoolSize` : The number of pending requests retained by the node. The `RequestPoolSize` is recommended to be at least double (x2) the `RequestBatchMaxCount`. This cannot be changed dynamically and the node must be restarted to pick up the change.
* `RequestForwardTimeout`: Is started from the moment a request is submitted, and defines the interval after which a request is forwarded to the leader.
* `RequestComplainTimeout`: Is started when `RequestForwardTimeout` expires, and defines the interval after which the node complains about the view leader.
* `RequestAutoRemoveTimeout`: Is started when `RequestComplainTimeout` expires, and defines the interval after which a request is removed (dropped) from the request pool.
* `ViewChangeResendInterval`: Defines the interval after which the ViewChange message is resent.
* `ViewChangeTimeout`: Is started when a node first receives a quorum of `ViewChange` messages, and defines the interval after which the node will try to initiate a view change with a higher view number.
* `LeaderHeartbeatTimeout`: Is the interval after which, if nodes do not receive a "sign of life" from the leader, they complain about the current leader and try to initiate a view change. A sign of life is either a heartbeat or a message from the leader.
* `LeaderHeartbeatCount`: Is the number of heartbeats per `LeaderHeartbeatTimeout` that the leader should emit. The heartbeat-interval is equal to: `LeaderHeartbeatTimeout/LeaderHeartbeatCount`.
* `CollectTimeout`: Is the interval after which the node stops listening to `StateTransferResponse` messages, stops collecting information about view metadata from remote nodes.


## Block validation policy 
The block validation policy in a Fabric channel defaults to a policy that requires a signature from any orderer:
```
        # BlockValidation specifies what signatures must be included in the block
        # from the orderer for the peer to validate it.
        BlockValidation:
            Type: ImplicitMeta
            Rule: "ANY Writers"
```

In BFT, the [configtxgen](./commands/configtxgen.md) tool encodes a policy that is suitable for BFT.
It automatically derives the policy from the nodes configured in the `ConsenterMapping` section.
However, when adding or removing ordering service nodes from the channel,
the policy should be adjusted accordingly as described in the reconfiguration [tutorial](./create_channel/add_orderer.html#add-the-orderer-to-the-policy-rules).


## Reconfiguration

The  BFT orderer supports dynamic (meaning, while the channel is being serviced) addition and removal of nodes, and configuration changes.
The only configuration parameter which cannot be changed dynamically is the `RequestPoolSize`.

Note that your cluster must be operational and able to achieve consensus before you attempt to reconfigure it.
As a rule, you should never attempt any configuration changes to the  BFT consenters, such as adding or removing a consenter, or rotating a consenter's certificate, unless all consenters are online and healthy.
Unless it is a removal of a consenter that is known to be offline.

If you do decide to change these parameters, it is recommended to only attempt such a change during a maintenance cycle. 
Problems are most likely to occur when a reconfiguration is attempted in clusters with only a few nodes while a node is down. 
For example, if you have four nodes in your consenters set and one of them is down, it means you have three out of four nodes alive. 
Extending the cluster to five nodes means until the fifth node finishes replicating, the cluster is not functional.
To add a new node to the ordering service:

1. **Ensure the orderer organization that owns the new node is one of the orderer organizations on the channel**. If the orderer organization is not an administrator, the node will be unable to pull blocks as a follower or be joined to the consenter set.
2. **Start the new ordering node**. For information about how to deploy an ordering node, check out [Planning for an ordering service](./deployorderer/ordererdeploy.html). Note that when you use the `osnadmin` CLI to create and join a channel, you do not need to point to a configuration block when starting the node.
3. **Use the `osnadmin` CLI to add the first orderer to the channel**. For more information, check out the [Create a channel](./create_channel/create_channel_participation.html#step-two-use-the-osnadmin-cli-to-add-the-first-orderer-to-the-channel) tutorial.
4. **Wait for the ordering node to replicate the blocks** from existing nodes for all channels its certificates have been added to. When an ordering node is added to a channel, it is added as a "follower", a state in which it can replicate blocks but is not part of the "consenter set" actively servicing the channel. When the node finishes replicating the blocks, its status should change from "onboarding" to "active". Note that an "active" ordering node is still not part of the consenter set.
5. **Add the new ordering node to the consenter set** as described in the [reconfiguration tutorial](./create_channel/add_orderer.md)


To remove an ordering node from the consenter set of a channel, use the `osnadmin channel remove` command to remove its endpoint and certificates from the channel. For more information, check out the [reconfiguration tutorial](./create_channel/add_orderer.md)

Once an ordering node is removed from the channel, the other ordering nodes stop communicating with the removed orderer in the context of the removed channel. They might still be communicating on other channels.

If the intent is to delete the node entirely, remove it from all channels before shutting down the node.

## Metrics

For a description of the Operations Service and how to set it up, check out
[our documentation on the Operations Service](operations_service.html).

For a list at the metrics that are gathered by the Operations Service, check out
our [reference material on metrics](metrics_reference.html).

While the metrics you prioritize will have a lot to do with your particular use case and configuration, these are the metrics you should consider monitoring:

* `cluster_size`: the number of nodes in this channel.
* `committed_block_number`: the number of the latest committed block.
* `is_leader`: the leadership status of the current node according to the latest committed block: 1 if it is the leader else 0.
* `leader_id`: the id of the current leader according to the latest committed block.
