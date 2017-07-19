Bringing up a Kafka-based Ordering Service
===========================================

Caveat emptor
-------------

This document assumes that the reader generally knows how to set up a Kafka
cluster and a ZooKeeper ensemble. The purpose of this guide is to identify the
steps you need to take so as to have a set of Hyperledger Fabric ordering
service nodes (OSNs) use your Kafka cluster and provide an ordering service to
your blockchain network.

Big picture
-----------

Each channel maps to a separate single-partition topic in Kafka. When
an OSN receives transactions via the ``Broadcast`` RPC, it checks to make sure
that the broadcasting client has permissions to write on the channel, then
relays (i.e. produces) those transactions to the appropriate partition in Kafka.
This partition is also consumed by the OSN which groups the received
transactions into blocks locally, persists them in its local ledger, and serves
them to receiving clients via the ``Deliver`` RPC. For low-level details, refer
to `the document that describes how we came to this design
<https://docs.google.com/document/d/1vNMaM7XhOlu9tB_10dKnlrhy5d7b1u8lSY8a-kVjCO4/edit>`_
-- Figure 8 is a schematic representation of the process described above.

Steps
-----

Let ``K`` and ``Z`` be the number of nodes in the Kafka cluster and the
ZooKeeper ensemble respectively:

i. At a minimum, ``K`` should be set to 4. (As we will explain in Step 4 below,
this is the minimum number of nodes necessary in order to exhibit crash fault
tolerance, i.e. with 4 brokers, you can have 1 broker go down, all channels will
continue to be writeable and readable, and new channels can be created.)

ii. ``Z`` will either be 3, 5, or 7. It has to be an odd number to avoid
split-brain scenarios, and larger than 1 in order to avoid single point of
failures. Anything beyond 7 ZooKeeper servers is considered an overkill.

Proceed as follows:

1. Orderers: **Encode the Kafka-related information in the network's genesis
block.** If you are using ``configtxgen``, edit ``configtx.yaml`` -- or pick a
preset profile for the system channel's genesis block --  so that:

    a. ``Orderer.OrdererType`` is set to ``kafka``.

    b. ``Orderer.Kafka.Brokers`` contains the address of *at least two* of the
    Kafka brokers in your cluster in ``IP:port`` notation. The list does not
    need to be exhaustive. (These are your seed brokers.)

2. Orderers: **Set the maximum block size.** Each block will have at most
`Orderer.AbsoluteMaxBytes` bytes (not including headers), a value that you can
set in ``configtx.yaml``. Let the value you pick here be ``A`` and make note of
it -- it will affect how you configure your Kafka brokers in Step 4.

3. Orderers: **Create the genesis block.** Use ``configtxgen``. The settings you
picked in Steps 1 and 2 above are system-wide settings, i.e. they apply across
the network for all the OSNs. Make note of the genesis block's location.

4. Kafka cluster: **Configure your Kafka brokers appropriately.** Ensure that
every Kafka broker has these keys configured:

    a. ``unclean.leader.election.enable = false`` -- Data consistency is key in
    a blockchain environment. We cannot have a channel leader chosen outside of
    the in-sync replica set, or we run the risk of overwriting the offsets that
    the previous leader produced, and --as a result-- rewrite the blockchain
    that the orderers produce.

    b.  ``min.insync.replicas = M`` -- Where you pick a value ``M`` such that
    1 < M < N (see ``default.replication.factor`` below). Data is considered
    committed when it is written to at least ``M`` replicas (which are then
    considered in-sync and belong to the in-sync replica set, or ISR). In any
    other case, the write operation returns an error. Then:

        i. If up to N-M replicas -- out of the ``N`` that the channel data is
        written to -- become unavailable, operations proceed normally.
        ii. If more replicas become unavailable, Kafka cannot maintain an ISR
        set of ``M,`` so it stops accepting writes. Reads work without issues.
        The channel becomes writeable again when ``M`` replicas get in-sync.

    c. ``default.replication.factor = N`` -- Where you pick a value ``N`` such
    that N < K. A replication factor of ``N`` means that each channel will have
    its data replicated to ``N`` brokers. These are the candidates for the ISR
    set of a channel. As we noted in the ``min.insync.replicas section`` above,
    not all of these brokers have to be available all the time. ``N`` should be
    set *strictly smaller* to ``K`` because channel creations cannot go forward
    if less than ``N`` brokers are up. So if you set N = K, a single broker
    going down means that no new channels can be created on the blockchain
    network -- the crash fault tolerance of the ordering service is
    non-existent.

    d. ``message.max.bytes`` and ``replica.fetch.max.bytes`` should be set to a
    value larger than ``A``, the value you picked in
    ``Orderer.AbsoluteMaxBytes`` in Step 2 above. Add some buffer to account for
    headers -- 1 MiB is more than enough. The following condition applies:

    ::

        Orderer.AbsoluteMaxBytes < replica.fetch.max.bytes <= message.max.bytes

    (For completeness, we note that ``message.max.bytes`` should be strictly
    smaller to ``socket.request.max.bytes`` which is set by default to 100 MiB.
    If you wish to have blocks larger than 100 MiB you will need to edit the
    hard-coded value in ``brokerConfig.Producer.MaxMessageBytes`` in
    ``fabric/orderer/kafka/config.go`` and rebuild the binary from source.
    This is not advisable.)

    e. ``log.retention.ms = -1``. Until the ordering service adds
    support for pruning of the Kafka logs, you should disable time-based
    retention and prevent segments from expiring. (Size-based retention -- see
    ``log.retention.bytes`` -- is disabled by default in Kafka at the time of
    this writing, so there's no need to set it explicitly.)

    Based on what we've described above, the minimum allowed values for ``M``
    and ``N`` are 2 and 3 respectively. This configuration allows for the
    creation of new channels to go forward, and for all channels to continue to
    be writeable.

5. Orderers: **Point each OSN to the genesis block.** Edit
``General.GenesisFile`` in ``orderer.yaml`` so that it points to the genesis
block created in Step 3 above. (While at it, ensure all other keys in that YAML
file are set appropriately.)

6. Orderers: **Adjust polling intervals and timeouts.** (Optional step.)

    a. The ``Kafka.Retry`` section in the ``orderer.yaml`` file allows you to
    adjust the frequency of the metadata/producer/consumer requests, as well as
    the socket timeouts. (These are all settings you would expect to see in a
    Kafka producer or consumer.)

    b. Additionally, when a new channel is created, or when an existing channel
    is reloaded (in case of a just-restarted orderer), the orderer interacts
    with the Kafka cluster in the following ways:

        a. It creates a Kafka producer (writer) for the Kafka partition that
        corresponds to the channel.

        b. It uses that producer to post a no-op ``CONNECT`` message to that
        partition.

        c. It creates a Kafka consumer (reader) for that partition.

        If any of these steps fail, you can adjust the frequency with which they
        are repeated. Specifically they will be re-attempted every
        ``Kafka.Retry.ShortInterval`` for a total of ``Kafka.Retry.ShortTotal``,
        and then every ``Kafka.Retry.LongInterval`` for a total of
        ``Kafka.Retry.LongTotal`` until they succeed. Note that the orderer will
        be unable to write to or read from a channel until all of the steps
        above have been completed successfully.

7. **Set up the OSNs and Kafka cluster so that they communicate over SSL.**
(Optional step, but highly recommended.) Refer to `the Confluent guide
<http://docs.confluent.io/2.0.0/kafka/ssl.html>`_ for the Kafka cluster side of
the equation, and set the keys under ``Kafka.TLS`` in ``orderer.yaml`` on every
OSN accordingly.

8. **Bring up the nodes in the following order: ZooKeeper ensemble, Kafka
cluster, ordering service nodes.**

Additional considerations
-------------------------

1. **Preferred message size.** In Step 2 above (see `Steps`_ section) you can
also set the preferred size of blocks by setting the
``Orderer.Batchsize.PreferredMaxBytes`` key. Kafka offers higher throughput when
dealing with relatively small messages; aim for a value no bigger than 1 MiB.

2. **Using environment variables to override settings.** When using the sample
Kafka and Zookeeper Docker images provided with Hyperledger Fabric (see
``images/kafka`` and ``images/zookeeper`` respectively), you can override a
Kafka broker or a ZooKeeper server's settings by using environment variables.
Replace the dots of the configuration key with underscores --
e.g. ``KAFKA_UNCLEAN_LEADER_ELECTION_ENABLE=false`` will allow you to override
the default value of ``unclean.leader.election.enable``. The same applies to the
OSNs for their *local* configuration, i.e. what can be set in ``orderer.yaml``.
For example ``ORDERER_KAFKA_RETRY_SHORTINTERVAL=1s`` allows you to override the
default value for ``Orderer.Kafka.Retry.ShortInterval``.

Supported Kafka versions and upgrading
--------------------------------------

Supported Kafka versions for v1 are ``0.9`` and ``0.10``. (Hyperledger Fabric
uses the `sarama client library <https://github.com/Shopify/sarama>`_
and vendors a version of it that supports Kafka 0.9 and 0.10.)

Out of the box the Kafka version defaults to ``0.9.0.1``. The sample Kafka
image provided by Hyperledger Fabric matches this default version. If you are
not using the sample Kafka image provided by Hyperledger Fabric, ensure that
you specify your Kafka cluster's Kafka version using the ``Kafka.Version`` key
in ``orderer.yaml``.

The current supported Kafka versions are:

* ``Version: 0.9.0.1``
* ``Version: 0.10.0.0``
* ``Version: 0.10.0.1``
* ``Version: 0.10.1.0``

Debugging
---------

Set ``General.LogLevel`` to ``DEBUG`` and ``Kafka.Verbose`` in ``orderer.yaml``
to ``true``.

Example
-------

Sample Docker Compose configuration files inline with the recommended settings
above can be found under the ``fabric/bddtests`` directory. Look for
``dc-orderer-kafka-base.yml`` and ``dc-orderer-kafka.yml``.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
