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

.. note:: For a tutorial that shows how to update a channel configuration, check
          out :doc:`channel_update_tutorial`. For an overview of the different
          kinds of channel updates that are possible, check out :doc:`config_update`.

Because new channels copy the configuration of the ordering system channel by
default, new channels will automatically be configured to work with the orderer
and channel capabilities of the ordering system channel and the application
capabilities specified by the channel creation transaction. Channels that already
exist, however, must be reconfigured.

The schema for the Capabilities value is defined in the protobuf as:

.. code:: bash

  message Capabilities {
        map<string, Capability> capabilities = 1;
  }

  message Capability { }

As an example, rendered in JSON:

.. code:: bash

  {
      "capabilities": {
          "V1_1": {}
      }
  }

Capabilities in an Initial Configuration
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

In the ``configtx.yaml`` file distributed in the ``config`` directory of the release
artifacts, there is a ``Capabilities`` section which enumerates the possible capabilities
for each capability type (Channel, Orderer, and Application).

The simplest way to enable capabilities is to pick a v1.1 sample profile and customize
it for your network. For example:

.. code:: bash

    SampleSingleMSPSoloV1_1:
        Capabilities:
            <<: *GlobalCapabilities
        Orderer:
            <<: *OrdererDefaults
            Organizations:
                - *SampleOrg
            Capabilities:
                <<: *OrdererCapabilities
        Consortiums:
            SampleConsortium:
                Organizations:
                    - *SampleOrg

Note that there is a ``Capabilities`` section defined at the root level (for the channel
capabilities), and at the Orderer level (for orderer capabilities). The sample above uses
a YAML reference to include the capabilities as defined at the bottom of the YAML.

When defining the orderer system channel there is no Application section, as those
capabilities are defined during the creation of an application channel. To define a new
channel's application capabilities at channel creation time, the application admins should
model their channel creation transaction after the ``SampleSingleMSPChannelV1_1`` profile.

.. code:: bash

   SampleSingleMSPChannelV1_1:
        Consortium: SampleConsortium
        Application:
            Organizations:
                - *SampleOrg
            Capabilities:
                <<: *ApplicationCapabilities

Here, the Application section has a new element ``Capabilities`` which references the
``ApplicationCapabilities`` section defined at the end of the YAML.

.. note:: The capabilities for the Channel and Orderer sections are inherited from
          the definition in the ordering system channel and are automatically included
          by the orderer during the process of channel creation.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
