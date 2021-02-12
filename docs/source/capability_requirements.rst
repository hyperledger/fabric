Defining capability requirements
================================

.. note:: This topic describes a network that does not use a "system channel", a
          channel that the ordering service is bootstrapped with and the ordering
          service exclusively controls. Since the release of v2.3, using a system
          channel is now considered the legacy process as compared to the process
          to :doc:`create_channel/create_channel_participation`. For a version of this topic that
          includes information about the system channel, check out
          `Capability requirements <https://hyperledger-fabric.readthedocs.io/en/release-2.2/capability_requirements.html>`_ from the v2.2 documentation.

As discussed in :doc:`capabilities_concept`, capability requirements are defined
per channel in the channel configuration (found in the channel’s most recent
configuration block). The channel configuration contains three locations, each
of which defines a capability of a different type.

+------------------+-----------------------------------+----------------------------------------------------+
| Capability Type  | Canonical Path                    | JSON Path                                          |
+==================+===================================+====================================================+
| Channel          | /Channel/Capabilities             | .channel_group.values.Capabilities                 |
+------------------+-----------------------------------+----------------------------------------------------+
| Orderer          | /Channel/Orderer/Capabilities     | .channel_group.groups.Orderer.values.Capabilities  |
+------------------+-----------------------------------+----------------------------------------------------+
| Application      | /Channel/Application/Capabilities | .channel_group.groups.Application.values.          |
|                  |                                   | Capabilities                                       |
+------------------+-----------------------------------+----------------------------------------------------+

Setting Capabilities
--------------------

Capabilities are set as part of the channel configuration (either as part of the
initial configuration -- which we'll talk about in a moment -- or as part of a
reconfiguration).

.. note:: For more information about how to update a channel configuration, check
          out :doc:`config_update`.

Capabilities in an Initial Configuration
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

In the ``configtx.yaml`` file distributed in the ``config`` directory of the release
artifacts, there is a ``Capabilities`` section which enumerates the possible capabilities
for each capability type (Channel, Orderer, and Application).

Note that there is a ``Capabilities`` section defined at the root level (for the channel
capabilities), and at the Orderer level (for orderer capabilities).

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
