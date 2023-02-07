Fabric Network Design Considerations
====================================

The structure of a blockchain network will be dictated by the use case it's serving. Use the topics below to help explorer the issues that your use case has. 

Certificate Authorities and Identities
--------------------------------------

* **Certificate Authority configuration**.
  As part of the overall decisions you have to make about your peers (how many, how many on each channel, and so on) and about your ordering service (how many nodes, who will own them), you also have to decide on how the CAs for your organization will be deployed. Production networks should be using Transport Layer Security (TLS), which will require setting up a TLS CA and using it to generate TLS certificates. This TLS CA will need to be deployed before your enrollment CA. We'll discuss this more in :ref:`dg-step-three-set-up-your-cas`.

* **Use Organizational Units or not?**
  Some organizations might find it necessary to establish Organizational Units to create a separation between certain identities and MSPs created by a single CA (for example, a manufacturer might want one organizational unit for its shipping department and another for its quality control department). Note that this is separate from the concept of the "Node OU", in which identities can have roles coded into them (for example, "admin" or "peer").

Peer configuration
------------------

* **Database type.**
  Some channels in a network might require all data to be modeled in a way :doc:`couchdb_as_state_database` can understand, while other networks, prioritizing speed, might decide that all peers will use LevelDB. Note that channels should not have peers that use both CouchDB and LevelDB on them, as CouchDB imposes some data restrictions on keys and values. Keys and values that are valid in LevelDB may not be valid in CouchDB.

* **Create a system channel or not.**
  Ordering nodes can be bootstrapped with a configuration block for an administrative channel known as the “system channel” (from which application channels can be created), or simply started and joined to application channels as needed. The recommended method is to bootstrap without a configuration block, which is the approach this deployment guide assumes you will take. For more information about creating a system channel genesis block and bootstrapping an ordering node with it, check out `Deploying a production network <https://hyperledger-fabric.readthedocs.io/en/release-2.2/deployment_guide_overview.html#creating-an-ordering-node>`_ from the Fabric v2.2 documentation.

* **Channels and private data.**
  Some networks might decide that :doc:`channels` are the best way to ensure privacy and isolation for certain transactions. Others might decide that fewer channels, supplemented where necessary with :doc:`private-data/private-data` collections, better serves their privacy needs.

Smart contracts
---------------

* **Chaincode deployment method.**
  Users have the option to deploy their chaincode using either the built in build and run support, a customized build and run using the :doc:`cc_launcher`, or using an :doc:`cc_service`.

Client Applications
-------------------

* ...