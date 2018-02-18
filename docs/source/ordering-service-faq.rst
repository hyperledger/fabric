Ordering Service FAQ
====================

General
~~~~~~~

:Question:
  **I have an ordering service up and running and want to switch consensus
  algorithms. How do I do that?**

:Answer:
  This is explicitly not supported.

..

:Question:
  **What is the orderer system channel?**

:Answer:
  The orderer system channel (sometimes called ordering system channel) is the
  channel the orderer is initially bootstrapped with. It is used to orchestrate
  channel creation. The orderer system channel defines consortia and the initial
  configuration for new channels. At channel creation time, the organization
  definition in the consortium, the ``/Channel`` group's values and policies, as
  well as the ``/Channel/Orderer`` group's values and policies, are all combined
  to form the new initial channel definition.

..

:Question:
  **If I update my application channel, should I update my orderer system
  channel?**

:Answer:
  Once an application channel is created, it is managed independently of any
  other channel (including the orderer system channel). Depending on the
  modification, the change may or may not be desirable to port to other
  channels. In general, MSP changes should be synchronized across all channels,
  while policy changes are more likely to be specific to a particular channel.

..

:Question:
  **Can I have an organization act both in an ordering and application role?**

:Answer:
  Although this is possible, it is a highly discouraged configuration. By
  default the ``/Channel/Orderer/BlockValidation`` policy allows any valid
  certificate of the ordering organizations to sign blocks. If an organization
  is acting both in an ordering and application role, then this policy should be
  updated to restrict block signers to the subset of certificates authorized for
  ordering.

..

:Question:
  **I want to write a consensus implementation for Fabric. Where do I begin?**

:Answer:
  A consensus plugin needs to implement the ``Consenter`` and ``Chain``
  interfaces defined in the `consensus package`_. There are two plugins built
  against these interfaces already: solo_ and kafka_. You can study them to take
  cues for your own implementation. The ordering service code can be found under
  the `orderer package`_.

.. _consensus package: https://github.com/hyperledger/fabric/blob/master/orderer/consensus/consensus.go
.. _solo: https://github.com/hyperledger/fabric/tree/master/orderer/consensus/solo
.. _kafka: https://github.com/hyperledger/fabric/tree/master/orderer/consensus/kafka
.. _orderer package: https://github.com/hyperledger/fabric/tree/master/orderer

..

:Question:
  **I want to change my ordering service configurations, e.g. batch timeout,
  after I start the network, what should I do?**

:Answer:
  This falls under reconfiguring the network. Please consult the topic on
  :doc:`configtxlator`.

Solo
~~~~

:Question:
  **How can I deploy Solo in production?**

:Answer:
  Solo is not intended for production.  It is not, and will never be, fault
  tolerant.

Kafka
~~~~~

:Question:
  **How do I remove a node from the ordering service?**

:Answer:
  This is a two step-process:
  
  1. Add the node's certificate to the relevant orderer's MSP CRL to prevent peers/clients from connecting to it. 
  2. Prevent the node from connecting to the Kafka cluster by leveraging standard Kafka access control measures such as TLS CRLs, or firewalling.

..

:Question:
  **I have never deployed a Kafka/ZK cluster before, and I want to use the
  Kafka-based ordering service. How do I proceed?**

:Answer:
  The Hyperledger Fabric documentation assumes the reader generally has the
  operational expertise to setup, configure, and manage a Kafka cluster
  (see :ref:`kafka-caveat`). If you insist on proceeding without such expertise,
  you should complete, *at a minimum*, the first 6 steps of the
  `Kafka Quickstart guide`_ before experimenting with the Kafka-based ordering
  service.

.. _Kafka Quickstart guide: https://kafka.apache.org/quickstart

..

:Question:
  **Where can I find a Docker composition for a network that uses the
  Kafka-based ordering service?**

:Answer:
  Consult `the end-to-end CLI example`_.

.. _the end-to-end CLI example: https://github.com/hyperledger/fabric/blob/master/examples/e2e_cli/docker-compose-e2e.yaml

..

:Question:
  **Why is there a ZooKeeper dependency in the Kafka-based ordering service?**

:Answer:
  Kafka uses it internally for coordination between its brokers.

..

:Question:
  **I'm trying to follow the BYFN example and get a "service unavailable" error,
  what should I do?**

:Answer:
  Check the ordering service's logs. A "Rejecting deliver request because of
  consenter error" log message is usually indicative of a connection problem
  with the Kafka cluster. Ensure that the Kafka cluster is set up properly, and
  is reachable by the ordering service's nodes.

BFT
~~~

:Question:
  **When is a BFT version of the ordering service going to be available?**

:Answer:
  No date has been set. We are working towards a release during the 1.x cycle,
  i.e. it will come with a minor version upgrade in Fabric. Track FAB-33_ for
  updates.

.. _FAB-33: https://jira.hyperledger.org/browse/FAB-33

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
