Creating a channel
==================

In order to create and transfer assets on a Hyperledger Fabric network, an
organization needs to join a channel. Channels are a private layer of communication
between specific organizations and are invisible to other members of the network.
Each channel consists of a separate ledger that can only be read and written to
by channel members, who are allowed to join their peers to the channel and receive
new blocks of transactions from the ordering service. While the peers, nodes, and
Certificate Authorities form the physical infrastructure of the network, channels
are the process by which organizations connect with each other and interact.

Because of the fundamental role that channels play in the operation and governance
of Fabric, we provide a series of tutorials that will cover different aspects
of how channels are created. The :doc:`create_channel` tutorial describes the
operational steps that need to be taken by a network administrator. The
:doc:`create_channel_config` tutorial introduces the conceptual aspects of creating
a channel, followed by a separate discussion of :doc:`channel_policies`.


.. toctree::
   :maxdepth: 1

   create_channel.md
   create_channel_config.md
   channel_policies.md

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
