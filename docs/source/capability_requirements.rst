Defining capability requirements
================================

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

Because new channels copy the configuration of the ordering system channel by
default, new channels will automatically be configured to work with the orderer
and channel capabilities of the ordering system channel and the application
capabilities specified by the channel creation transaction.

Capabilities in an Initial Configuration
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

In the ``configtx.yaml`` file distributed in the ``config`` directory of the release
artifacts, there is a ``Capabilities`` section which enumerates the possible capabilities
for each capability type (Channel, Orderer, and Application).

Note that there is a ``Capabilities`` section defined at the root level (for the channel
capabilities), and at the Orderer level (for orderer capabilities).

When defining the orderer system channel there is no Application section, as those
capabilities are defined during the creation of an application channel.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
