Deploying a production network
==============================

This deployment guide is a high level overview of the proper sequence for setting up production Fabric network components, in addition to best practices and a few of the many considerations to keep in mind when deploying. Note that this topic will discuss "setting up the network" as a holistic process from the perspective of a single individual. More likely than not, real world production networks will not be set up by a single individual but as a collaborative effort directed by several individuals (a collection of banks each setting up their own components, for example) instead.

The process for deploying a Fabric network is complex and presumes an understanding of Public Key Infrastructure and managing distributed systems. If you are a smart contract or application developer, you should not need this level of expertise in deploying a production level Fabric network. However, you might need to be aware of how networks are deployed in order to develop effective smart contracts and applications.

If all you need is a development environment to test chaincode, smart contracts, and applications against, check out :doc:`test_network`. The network you'll deploy will include two organizations, each owning one peer, and a single ordering service organization that owns a single ordering node. **This test network is not meant to provide a blueprint for deploying production components, and should not be used as such, as it makes assumptions and decisions that production deployments will not make.**

The guide will give you an overview of the steps of setting up production components and a production network:

- :ref:`dg-step-one-decide-on-your-network-configuration`
- :ref:`dg-step-two-set-up-a-cluster-for-your-resources`
- :ref:`dg-step-three-set-up-your-cas`
- :ref:`dg-step-four-use-the-ca-to-create-identities-and-msps`
- :ref:`dg-step-five-deploy-nodes`
   - :ref:`dg-create-a-peer`
   - :ref:`dg-create-an-ordering-node`

.. _dg-step-one-decide-on-your-network-configuration:

Step one: Decide on your network configuration
----------------------------------------------

The structure of a blockchain network will be dictated by the use case it's serving. There are too many options to give definitive guidance on every point, but let's consider a few scenarios.

In contrast to development environments or proofs of concept, security, resource management, and high availability become a priority when operating in production. How many nodes do you need to satisfy high availability, and in what data centers do you wish to deploy them in to satisfy both the needs of disaster recovery and data residency? How will you ensure that your private keys and roots of trust remain secure?

In addition to the above, here is a sampling of the decisions you will need to make before deploying components:

* **Certificate Authority configuration**.
  As part of the overall decisions you have to make about your peers (how many, how many on each channel, and so on) and about your ordering service (how many nodes, who will own them), you also have to decide on how the CAs for your organization will be deployed. Production networks should be using Transport Layer Security (TLS), which will require setting up a TLS CA and using it to generate TLS certificates. This TLS CA will need to be deployed before your enrollment CA. We'll discuss this more in :ref:`dg-step-three-set-up-your-cas`.

* **Use Organizational Units or not?**
  Some organizations might find it necessary to establish Organizational Units to create a separation between certain identities and MSPs created by a single CA (for example, a manufacturer might want one organizational unit for its shipping department and another for its quality control department). Note that this is separate from the concept of the "Node OU", in which identities can have roles coded into them (for example, "admin" or "peer").

* **Database type.**
  Some channels in a network might require all data to be modeled in a way :doc:`couchdb_as_state_database` can understand, while other networks, prioritizing speed, might decide that all peers will use LevelDB. Note that channels should not have peers that use both CouchDB and LevelDB on them, as CouchDB imposes some data restrictions on keys and values. Keys and values that are valid in LevelDB may not be valid in CouchDB.

* **Create a system channel or not.**
  Ordering nodes can be bootstrapped with a configuration block for an administrative channel known as the “system channel” (from which application channels can be created), or simply started and joined to application channels as needed. The recommended method is to bootstrap without a configuration block, which is the approach this deployment guide assumes you will take. For more information about creating a system channel genesis block and bootstrapping an ordering node with it, check out `Deploying a production network <https://hyperledger-fabric.readthedocs.io/en/release-2.2/deployment_guide_overview.html#creating-an-ordering-node>`_ from the Fabric v2.2 documentation.

* **Channels and private data.**
  Some networks might decide that :doc:`channels` are the best way to ensure privacy and isolation for certain transactions. Others might decide that fewer channels, supplemented where necessary with :doc:`private-data/private-data` collections, better serves their privacy needs.

* **Container orchestration.**
  Different users might also make different decisions about their container orchestration, creating separate containers for their peer process, logging, CouchDB, gRPC communications, and chaincode, while other users might decide to combine some of these processes.

* **Chaincode deployment method.**
  Users have the option to deploy their chaincode using either the built in build and run support, a customized build and run using the :doc:`cc_launcher`, or using an :doc:`cc_service`.

* **Using firewalls.**
  In a production deployment, components belonging to one organization might need access to components from other organizations, necessitating the use of firewalls and advanced networking configuration. For example, applications using the Fabric SDK require access to all endorsing peers from all organizations and the ordering services for all channels. Similarly, peers need access to the ordering service on the channels that they are receiving new blocks from.

However and wherever your components are deployed, you will need a high degree of expertise in your management system of choice (such as Kubernetes) in order to efficiently operate your network. Similarly, the structure of the network must be designed to fit the business use case and any relevant laws and regulations government of the industry in which the network will be designed to function.

This deployment guide will not go through every iteration and potential network configuration, but does give common guidelines and rules to consider.

.. _dg-step-two-set-up-a-cluster-for-your-resources:

Step two: Set up a cluster for your resources
---------------------------------------------

Generally speaking, Fabric is agnostic to the methods used to deploy and manage it. It is possible, for example, to deploy and manage a peer on a laptop. For a number of reasons, this is likely to be unadvisable, but there is nothing in Fabric that prohibits it.

As long as you have the ability to deploy containers, whether locally (or behind a firewall), or in a cloud, it should be possible to stand up components and connect them to each other. However, Kubernetes features a number of helpful tools that have made it a popular container management platform for deploying and managing Fabric networks. For more information about Kubernetes, check out `the Kubernetes documentation <https://kubernetes.io/docs>`_. This topic will mostly limit its scope to the binaries and provide instructions that can be applied when using a Docker deployment or Kubernetes.

However and wherever you choose to deploy your components, you will need to make sure you have enough resources for the components to run effectively. The sizes you need will largely depend on your use case. If you plan to join a single peer to several high volume channels, it will need much more CPU and memory than if you only plan to join to a single channel. As a rough estimate, plan to dedicate approximately three times the resources to a peer as you plan to allocate to a single ordering node (as you will see below, it is recommended to deploy at least three and optimally five nodes in an ordering service). Similarly, you should need approximately a tenth of the resources for a CA as you will for a peer. You will also need to add storage to your cluster (some cloud providers may provide storage) as you cannot configure Persistent Volumes and Persistent Volume Claims without storage being set up with your cloud provider first. The use of persistent storage ensures that data such as MSPs, ledgers, and installed chaincodes are not stored on the container filesystem, preventing them from being destroyed if the containers are destroyed.

By deploying a proof of concept network and testing it under load, you will have a better sense of the resources you will require.

Managing your infrastructure
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The exact methods and tools you use to manage your backend will depend on the backend you choose. However, here are some considerations worth noting.

* Using secret objects to securely store important configuration files in your cluster. For information about Kubernetes secrets, check out `Kubernetes secrets <https://kubernetes.io/docs/concepts/configuration/secret/>`_. You also have the option to use Hardware Security Modules (HSMs) or encrypted Persistent Volumes (PVs). Along similar lines, after deploying Fabric components, you will likely want to connect to a container on your own backend, for example using a private repo in a service like Docker Hub. In that case, you will need to code the login information in the form of a Kubernetes secret and include it in the YAML file when deploying components.
* Cluster considerations and node sizing. In step 2 above, we discussed a general outline for how to think about the sizings of nodes. Your use case, as well as a robust period of development, is the only way you will truly know how large your peers, ordering nodes, and CAs will need to be.
* How you choose to mount your volumes. It is a best practice to mount the volumes relevant to your nodes external to the place where your nodes are deployed. This will allow you to reference these volumes later on (for example, restarting a node or a container that has crashed) without having to redeploy or regenerate your crypto material.
* How you will monitor your resources. It is critical that you establish a strategy and method for monitoring the resources used by your individual nodes and the resources deployed to your cluster generally. As you join your peers to more channels, you will need likely need to increase its CPU and memory allocation. Similarly, you will need to make sure you have enough storage space for your state database and blockchain.

.. _dg-step-three-set-up-your-cas:

Step three: Set up your CAs
---------------------------

The first component that must be deployed in a Fabric network is a CA. This is because the certificates associated with a node (not just for the node itself but also the certificates identifying who can administer the node) must be created before the node itself can be deployed. While it is not necessary to use the Fabric CA to create these certificates, the Fabric CA also creates MSP structures that are needed for components and organizations to be properly defined. If a user chooses to use a CA other than the Fabric CA, they will have to create the MSP folders themselves.

* One CA (or more, if you are using intermediate CAs --- more on intermediate CAs below) is used to generate (through a process called "enrollment") the certificates of the admin of an organization, the MSP of that organization, and any nodes owned by that organization. This CA will also generate the certificates for any additional users. Because of its role in "enrolling" identities, this CA is sometimes called the "enrollment CA" or the "ecert CA".
* The other CA generates the certificates used to secure communications on Transport Layer Security (TLS). For this reason, this CA is often referred to as a "TLS CA". These TLS certificates are attached to actions as a way of preventing "man in the middle" attacks. Note that the TLS CA is only used for issuing certificates for nodes and can be shut down when that activity is completed. Users have the option to use one way (client only) TLS as well as two way (server and client) TLS, with the latter also known as "mutual TLS". Because specifying that your network will be using TLS (which is recommended) should be decided before deploying the "enrollment" CA (the YAML file specifying the configuration of this CA has a field for enabling TLS), you should deploy your TLS CA first and use its root certificate when bootstrapping your enrollment CA. This TLS certificate will also be used by the ``fabric-ca client`` when connecting to the enrollment CA to enroll identities for users and nodes.

While all of the non-TLS certificates associated with an organization can be created by a single "root" CA (that is, a CA that is its own root of trust), for added security organizations can decide to use "intermediate" CAs whose certificates are created by a root CA (or another intermediate CA that eventually leads back to a root CA). Because a compromise in the root CA leads to a collapse for its entire trust domain (the certs for the admins, nodes, and any CAs it has generated certificates for), intermediate CAs are a useful way to limit the exposure of the root CA. Whether you choose to use intermediate CAs will depend on the needs of your use case. They are not mandatory. Note that it is also possible to configure a Lightweight Directory Access Protocol (LDAP) to manage identities on a Fabric network for those enterprises that already have this implementation and do not want to add a layer of identity management to their existing infrastructure. The LDAP effectively pre registers all of the members of the directory and allows them to enroll based on the criteria given.

**In a production network, it is recommended to deploy at least one CA per organization for enrollment purposes and another for TLS.** For example, if you deploy three peers that are associated with one organization and an ordering node that is associated with an ordering organization, you will need at least four CAs. Two of the CAs will be for the peer organization (generating the enrollment and TLS certificates for the peer, admins, communications, and the folder structure of the MSP representing the organization) and the other two will be for the orderer organization. Note that users will generally only register and enroll with the enrollment CA, while nodes will register and enroll with both the enrollment CA (where the node will get its signing certificates that identify it when it attempts to sign its actions) and with the TLS CA (where it will get the TLS certificates it uses to authenticate its communications).

For an example of how to setup an organization CA and a TLS CA and enroll their admin identity, check out the `Fabric CA Deployment Guide <https://hyperledger-fabric-ca.readthedocs.io/en/latest/deployguide/ca-deploy.html>`__.  The deploy guide uses the Fabric CA client to register and enroll the identities that are required when setting up CAs.

.. toctree::
   :maxdepth: 1
   :caption: Deploy a Production CA

   Planning for a CA <https://hyperledger-fabric-ca.readthedocs.io/en/latest/deployguide/ca-deploy-topology.html>
   Checklist for a production CA server <https://hyperledger-fabric-ca.readthedocs.io/en/latest/deployguide/ca-config.html>
   CA deployment steps <https://hyperledger-fabric-ca.readthedocs.io/en/latest/deployguide/cadeploy.html>

.. _dg-step-four-use-the-ca-to-create-identities-and-msps:

Step four: Use the CA to create identities and MSPs
---------------------------------------------------

After you have created your CAs, you can use them to create the certificates for the identities and components related to your organization (which is represented by an MSP). For each organization, you will need to, at a minimum:

* **Register and enroll an admin identity and create an MSP**. After the CA that will be associated with an organization has been created, it can be used to first register a user and then enroll an identity (producing the certificate pair used by all entities on the network). In the first step, a username and password for the identity is assigned by the admin of the CA. Attributes and affiliations can also be given to the identity (for example, a ``role`` of ``admin``, which is necessary for organization admins). After the identity has been registered, it can be enrolled by using the username and password. The CA will generate two certificates for this identity --- a public certificate (also known as a "signcert" or "public cert") known to the other members of the network, and the private key (stored in the ``keystore`` folder) used to sign actions taken by the identity. The CA will also generate a set of folders called an "MSP" containing the public certificate of the CA issuing the certificate and the root of trust for the CA (this may or may not be the same CA). This MSP can be thought of as defining the organization associated with the identity of the admin. In cases where the admin of the org will also be an admin of a node (which will be typical), **you must create the org admin identity before creating the local MSP of a node, since the certificate of the node admin must be used when creating the local MSP**.
* **Register and enroll node identities**. Just as an org admin identity is registered and enrolled, the identity of a node must be registered and enrolled with both an enrollment CA and a TLS CA (the latter generates certificates that are used to secure communications). Instead of giving a node a role of ``admin`` or ``user`` when registering it with the enrollment CA, give it a role of ``peer`` or ``orderer``. As with the admin, attributes and affiliations for this identity can also be assigned. The MSP structure for a node is known as a "local MSP", since the permissions assigned to the identities are only relevant at the local (node) level. This MSP is created when the node identity is created, and is used when bootstrapping the node.

For more conceptual information about identities and permissions in a Fabric-based blockchain network, see :doc:`identity/identity` and :doc:`membership/membership`.

For more information about how to use a CA to register and enroll identities, including sample commands, check out `Registering and enrolling identities with a CA <https://hyperledger-fabric-ca.readthedocs.io/en/latest/deployguide/use_CA.html>`_.

.. _dg-step-five-deploy-nodes:

Step five: Deploy peers and ordering nodes
------------------------------------------

Once you have gathered all of the certificates and MSPs you need, you're almost ready to create a node. As discussed above, there are a number of valid ways to deploy nodes.

Before any node can be deployed, its configuration file must be customized. For the peer, this file is called ``core.yaml``, while the configuration file for ordering nodes is called ``orderer.yaml``.

You have three main options for tuning your configuration.

1. Edit the YAML file bundled with the binaries.
2. Use environment variable overrides when deploying.
3. Specify flags on CLI commands.

Option 1 has the advantage of persisting your changes whenever you bring down and bring back up the node. The downside is that you will have to port the options you customized to the new YAML when upgrading to a new binary version (you should use the latest YAML when upgrading to a new version).

.. note:: You can extrapolate environment variables from the parameters in the relevant YAML file by using all capital letters, underscores between the relevant phrases, and a prefix. For example, the peer configuration variable called ``peer.localMSPid`` (which is the ``localMSPid`` variable inside the ``peer`` configuration section) in ``core.yaml`` would be rendered as an environment variable called ``CORE_PEER_LOCALMSPID``, while the ordering service environment variable ``General.LocalMSPID`` in the ``General`` section of the ``orderer.yaml`` configuration file would be rendered as an environment variable called ``ORDERER_GENERAL_LOCALMSPID``.

.. _dg-create-a-peer:

Creating a peer
~~~~~~~~~~~~~~~

If you’ve read through the key concept topic on :doc:`peers/peers`, you should have a good idea of the role peers play in a network and the nature of their interactions with other network components. Peers are owned by organizations that are members of a channel (for this reason, these organizations are sometimes called "peer organizations"). They connect to the ordering service and to other peers, have smart contracts installed on them, and are where ledgers are stored.

These roles are important to understand before you create a peer, as they will influence your customization and deployment decisions. For a look at the various decisions you will need to make, check out :doc:`deploypeer/peerplan`.

The configuration values in a peer's ``core.yaml`` file must be customized or overridden with environment variables. You can find the default ``core.yaml`` configuration file `in the sampleconfig directory of Hyperledger Fabric <https://github.com/hyperledger/fabric/blob/main/sampleconfig/core.yaml>`_. This configuration file is bundled with the peer image and is also included with the downloadable binaries. For information about how to download the production ``core.yaml`` along with the peer image, check out :doc:`deploypeer/peerdeploy`.

While there are many parameters in the default ``core.yaml``, you will only need to customize a small percentage of them. In general, if you do not have the need to change a tuning value, keep the default value.

Among the parameters in ``core.yaml``, there are:

* **Identifiers**: these include not just the paths to the relevant local MSP and Transport Layer Security (TLS) certificates, but also the name (known as the "peer ID") of the peer and the MSP ID of the organization that owns the peer.

* **Addresses and paths**: because peers are not entities unto themselves but interact with other peers and components, you must specify a series of addresses in the configuration. These include addresses where the peer itself can be found by other components as well as the addresses where, for example, chaincodes can be found (if you are employing external chaincodes). Similarly, you will need to specify the location of your ledger (as well as your state database type) and the path to your external builders (again, if you intend to employ external chaincodes). These include **Operations and metrics**, which allow you to set up methods for monitoring the health and performance of your peer through the configuration of endpoints.

* **Gossip**: components in Fabric networks communicate with each other using the "gossip" protocol. Through this protocol, they can be discovered by the discovery service and disseminate blocks and private data to each other. Note that gossip communications are secured using TLS.

For more information about ``core.yaml`` and its specific parameters, check out :doc:`deploypeer/peerchecklist`.

When you're comfortable with how your peer has been configured, how your volumes are mounted, and your backend configuration, you can run the command to launch the peer (this command will depend on your backend configuration).

.. toctree::
   :maxdepth: 1
   :caption: Deploying a production peer

   deploypeer/peerplan
   deploypeer/peerchecklist
   deploypeer/peerdeploy

.. _dg-create-an-ordering-node:

Creating an ordering node
~~~~~~~~~~~~~~~~~~~~~~~~~

Note: while it is possible to add additional nodes to an ordering service, only the process for creating an ordering service is covered in these tutorials.

If you’ve read through the key concept topic on :doc:`orderer/ordering_service`, you should have a good idea of the role the ordering service plays in a network and the nature of its interactions with other network components. The ordering service is responsible for literally “ordering” endorsed transactions into blocks, which peers then validate and commit to their ledgers.

These roles are important to understand before you create an ordering service, as it will influence your customization and deployment decisions. Among the chief differences between a peer and ordering service is that in a production network, multiple ordering nodes work together to form the “ordering service” of a channel (these nodes are also known as the “consenter set”). This creates a series of important decisions that need to be made at both the node level and at the cluster level. Some of these cluster decisions are not made in individual ordering node ``orderer.yaml`` files but instead in the ``configtx.yaml`` file that is used to generate the genesis block for an application channel. For a look at the various decisions you will need to make, check out :doc:`deployorderer/ordererplan`.

The configuration values in an ordering node’s ``orderer.yaml`` file must be customized or overridden with environment variables. You can find the default ``orderer.yaml`` configuration file `in the sampleconfig directory of Hyperledger Fabric <https://github.com/hyperledger/fabric/blob/main/sampleconfig/orderer.yaml>`_.

This configuration file is bundled with the orderer image and is also included with the downloadable binaries. For information about how to download the production ``orderer.yaml`` along with the orderer image, check out :doc:`deployorderer/ordererdeploy`.

While there are many parameters in the default ``orderer.yaml``, you will only need to customize a small percentage of them. In general, if you do not have the need to change a tuning value, keep the default value.

Among the parameters in ``orderer.yaml``, there are:

* **Identifiers**: these include not just the paths to the relevant local MSP and Transport Layer Security (TLS) certificates, but also the MSP ID of the organization that owns the ordering node.

* **Addresses and paths**: because ordering nodes interact with other components, you must specify a series of addresses in the configuration. These include addresses where the ordering node itself can be found by other components as well as **Operations and metrics**, which allow you to set up methods for monitoring the health and performance of your ordering node through the configuration of endpoints.

For more information about ``orderer.yaml`` and its specific parameters, check out :doc:`deployorderer/ordererchecklist`.

Note: This tutorial assumes that a system channel genesis block will not be used when bootstrapping the orderer. Instead, these nodes (or a subset of them), will be joined to a channel using the process to :doc:`create_channel/create_channel_participation`. For information on how to create an orderer that will be bootstrapped with a system channel genesis block, check out `Deploy the ordering service <https://hyperledger-fabric.readthedocs.io/en/release-2.2/deployment_guide_overview.html>`_ from the Fabric v2.2 documentation.

.. toctree::
   :maxdepth: 1
   :caption: Deploying a production ordering node

   deployorderer/ordererplan
   deployorderer/ordererchecklist
   deployorderer/ordererdeploy

Next steps
----------

Blockchain networks are all about connection, so once you've deployed nodes, you'll obviously want to connect them to other nodes! If you have a peer organization and a peer, you'll want to join your organization to a consortium and join or :doc:`create_channel/create_channel_participation`. If you have an ordering node, you will want to add peer organizations to your consortium. You'll also want to learn how to develop chaincode, which you can learn about in the topics :doc:`developapps/scenario` and :doc:`chaincode4ade`.

Part of the process of connecting nodes and creating channels will involve modifying policies to fit the use cases of business networks. For more information about policies, check out :doc:`policies/policies`.

One of the common tasks in a Fabric will be the editing of existing channels. For a tutorial about that process, check out :doc:`config_update`. One popular channel update is to add an org to an existing channel. For a tutorial about that specific process, check out :doc:`channel_update_tutorial`. For information about upgrading nodes after they have been deployed, check out :doc:`upgrading_your_components`.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
