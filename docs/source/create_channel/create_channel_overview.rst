Creating a channel
==================

In order to create and transfer assets on a Hyperledger Fabric network, an
organization needs to join a channel. Channels are a private layer of communication
between specific organizations. Each channel consists of a separate blockchain
ledger that can only be read and written to by channel members, who are allowed to
join their peers to the channel and receive new blocks of transactions from the
ordering service. Channels are invisible to other members of the network, making
them the most powerful tool for privacy in a blockchain consortium.

Because of the fundamental role that channels play in the operation and governance
of Fabric, we provide a series of tutorials that will cover different aspects
of how channels are created. The :doc:`create_channel` describes the
operational steps that need to be taken by a network administrator. The
:doc:`create_channel_genesis` tutorial discusses the conceptual aspects of creating
a channel, including the creation of channel policies that will govern how
organizations interact with the channel and update the channel after it is created.


.. toctree::
   :maxdepth: 1

   create_channel.md
   create_channel_genesis.md

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
