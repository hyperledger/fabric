Peer and Ordering nodes
=======================

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


.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
