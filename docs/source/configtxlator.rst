Reconfiguring with configtxlator
================================

Overview
--------

The ``configtxlator`` tool was created to support reconfiguration independent
of SDKs. Channel configuration is stored as a transaction in configuration
blocks of a channel and may be manipulated directly, such as in the bdd behave
tests.  However, at the time of this writing, no SDK natively supports
manipulating the configuration directly, so the ``configtxlator`` tool is
designed to provide an API which consumers of any SDK may interact with to
assist with configuration updates.

The tool name is a portmanteau of *configtx* and *translator* and is intended to
convey that the tool simply converts between different equivalent data
representations. It does not generate configuration. It does not submit or
retrieve configuration. It does not modify configuration itself, it simply
provides some bijective operations between different views of the configtx
format.

The standard usage is expected to be:

  1. SDK retrieves latest config
  2. ``configtxlator`` produces human readable version of config
  3. User or application edits the config
  4. ``configtxlator`` is used to compute config update representation of
     changes to the config
  5. SDK submits signs and submits config

The ``configtxlator`` tool exposes a truly stateless REST API for interacting
with configuration elements.  These REST components support converting the
native configuration format to/from a human readable JSON representation, as
well as computing configuration updates based on the difference between two
configurations.

Because the ``configtxlator`` service deliberately does not contain any crypto
material, or otherwise secret information, it does not include any authorization
or access control. The anticipated typical deployment would be to operate as
a sandboxed container, locally with the application, so that there is a
dedicated ``configtxlator`` process for each consumer of it.

Running the configtxlator
-------------------------

The ``configtxlator`` tool can be downloaded with the other Hyperledger Fabric
platform-specific binaries. Please see :ref:`download-platform-specific-binaries`
for details.

The tool may be configured to listen on a different port and you may also
specify the hostname using the ``--port`` and ``--hostname`` flags. To explore
the complete set of commands and flags, run ``configtxlator --help``.

The binary will start an http server listening on the designated port and is now
ready to process request.

To start the ``configtxlator`` server:

.. code:: bash

  configtxlator start
  2017-06-21 18:16:58.248 HKT [configtxlator] startServer -> INFO 001 Serving HTTP requests on 0.0.0.0:7059

Proto translation
-----------------

For extensibility, and because certain fields must be signed over, many proto
fields are stored as bytes.  This makes the natural proto to JSON translation
using the ``jsonpb`` package ineffective for producing a human readable version
of the protobufs.  Instead, the ``configtxlator`` exposes a REST component to do
a more sophisticated translation.

To convert a proto to its human readable JSON equivalent, simply post the binary
proto to the rest target
``http://$SERVER:$PORT/protolator/decode/<message.Name>``,
where ``<message.Name>`` is the fully qualified proto name of the message.

For instance, to decode a configuration block saved as
``configuration_block.pb``, run the command:

.. code:: bash

  curl -X POST --data-binary @configuration_block.pb http://127.0.0.1:7059/protolator/decode/common.Block

To convert the human readable JSON version of the proto message, simply post the
JSON version to ``http://$SERVER:$PORT/protolator/encode/<message.Name``, where
``<message.Name>`` is again the fully qualified proto name of the message.

For instance, to re-encode the block saved as ``configuration_block.json``, run
the command:

.. code:: bash

  curl -X POST --data-binary @configuration_block.json http://127.0.0.1:7059/protolator/encode/common.Block

Any of the configuration related protos, including ``common.Block``,
``common.Envelope``, ``common.ConfigEnvelope``, ``common.ConfigUpdateEnvelope``,
``common.Config``, and ``common.ConfigUpdate`` are valid targets for
these URLs.  In the future, other proto decoding types may be added, such as
for endorser transactions.

Config update computation
-------------------------

Given two different configurations, it is possible to compute the config update
which transitions between them.  Simply POST the two ``common.Config`` proto
encoded configurations as ``multipart/formdata``, with the original as field
``original`` and the updated as field ``updated``, to
``http://$SERVER:$PORT/configtxlator/compute/update-from-configs``.

For example, given the original config as the file ``original_config.pb`` and
the updated config as the file ``updated_config.pb`` for the channel
``desiredchannel``:

.. code:: bash

  curl -X POST -F channel=desiredchannel -F original=@original_config.pb -F updated=@updated_config.pb http://127.0.0.1:7059/configtxlator/compute/update-from-configs

Bootstraping example
--------------------

First start the ``configtxlator``:

.. code:: bash

  $ configtxlator start
  2017-05-31 12:57:22.499 EDT [configtxlator] main -> INFO 001 Serving HTTP requests on port: 7059

First, produce a genesis block for the ordering system channel:

.. code:: bash

  $ configtxgen -outputBlock genesis_block.pb
  2017-05-31 14:15:16.634 EDT [common/configtx/tool] main -> INFO 001 Loading configuration
  2017-05-31 14:15:16.646 EDT [common/configtx/tool] doOutputBlock -> INFO 002 Generating genesis block
  2017-05-31 14:15:16.646 EDT [common/configtx/tool] doOutputBlock -> INFO 003 Writing genesis block

Decode the genesis block into a human editable form:

.. code:: bash

  curl -X POST --data-binary @genesis_block.pb http://127.0.0.1:7059/protolator/decode/common.Block > genesis_block.json

Edit the ``genesis_block.json`` file in your favorite JSON editor, or manipulate
it programatically.  Here we use the JSON CLI tool ``jq``.  For simplicity, we
are editing the batch size for the channel, because it is a single numeric
field. However, any edits, including policy and MSP edits may be made here.

First, let's establish an environment variable to hold the string that defines
the path to a property in the json:

.. code:: bash

  export MAXBATCHSIZEPATH=".data.data[0].payload.data.config.channel_group.groups.Orderer.values.BatchSize.value.max_message_count"

Next, let's display the value of that property:

.. code:: bash

  jq "$MAXBATCHSIZEPATH" genesis_block.json
  10

Now, let's set the new batch size, and display the new value:

  jq "$MAXBATCHSIZEPATH = 20" genesis_block.json  > updated_genesis_block.json
  jq "$MAXBATCHSIZEPATH" updated_genesis_block.json
  20

The genesis block is now ready to be re-encoded into the native proto form to be
used for bootstrapping:

.. code:: bash

  curl -X POST --data-binary @updated_genesis_block.json http://127.0.0.1:7059/protolator/encode/common.Block > updated_genesis_block.pb

The ``updated_genesis_block.pb`` file may now be used as the genesis block for
bootstrapping an ordering system channel.

Reconfiguration example
-----------------------

In another terminal window, start the orderer using the default options,
including the provisional bootstrapper which will create a ``testchainid``
ordering system channel.

.. code:: bash

  ORDERER_GENERAL_LOGLEVEL=debug orderer

Reconfiguring a channel can be performed in a very similar way to modifying a
genesis config.

First, fetch the config_block proto:

.. code:: bash

  $ peer channel fetch config config_block.pb -o 127.0.0.1:7050 -c testchainid
  2017-05-31 15:11:37.617 EDT [msp] getMspConfig -> INFO 001 intermediate certs folder not found at [/home/yellickj/go/src/github.com/hyperledger/fabric/sampleconfig/msp/intermediatecerts]. Skipping.: [stat /home/yellickj/go/src/github.com/hyperledger/fabric/sampleconfig/msp/intermediatecerts: no such file or directory]
  2017-05-31 15:11:37.617 EDT [msp] getMspConfig -> INFO 002 crls folder not found at [/home/yellickj/go/src/github.com/hyperledger/fabric/sampleconfig/msp/intermediatecerts]. Skipping.: [stat /home/yellickj/go/src/github.com/hyperledger/fabric/sampleconfig/msp/crls: no such file or directory]
  Received block:  1
  Received block:  1
  2017-05-31 15:11:37.635 EDT [main] main -> INFO 003 Exiting.....

Next, send the config block to the ``configtxlator`` service for decoding:

.. code:: bash

  curl -X POST --data-binary @config_block.pb http://127.0.0.1:7059/protolator/decode/common.Block > config_block.json

Extract the config section from the block:

.. code:: bash

  jq .data.data[0].payload.data.config config_block.json > config.json

Edit the config, saving it as a new ``updated_config.json``.  Here, we set the
batch size to 30.

.. code:: bash

  jq ".channel_group.groups.Orderer.values.BatchSize.value.max_message_count = 30" config.json  > updated_config.json

Re-encode both the original config, and the updated config into proto:

.. code:: bash

  curl -X POST --data-binary @config.json http://127.0.0.1:7059/protolator/encode/common.Config > config.pb

.. code:: bash

  curl -X POST --data-binary @updated_config.json http://127.0.0.1:7059/protolator/encode/common.Config > updated_config.pb

Now, with both configs properly encoded, send them to the `configtxlator`
service to compute the config update which transitions between the two.

.. code:: bash

  curl -X POST -F original=@config.pb -F updated=@updated_config.pb http://127.0.0.1:7059/configtxlator/compute/update-from-configs -F channel=testchainid > config_update.pb

At this point, the computed config update is now prepared. Traditionally,
an SDK would be used to sign and wrap this message. However, in the interest of
using only the peer cli, the `configtxlator` can also be used for this task.

First, we decode the ConfigUpdate so that we may work with it as text:

.. code:: bash

  $ curl -X POST --data-binary @config_update.pb http://127.0.0.1:7059/protolator/decode/common.ConfigUpdate > config_update.json

Then, we wrap it in an envelope message:

.. code:: bash

  echo '{"payload":{"header":{"channel_header":{"channel_id":"testchainid", "type":2}},"data":{"config_update":'$(cat config_update.json)'}}}' > config_update_as_envelope.json

Next, convert it back into the proto form of a full fledged config
transaction:

.. code:: bash

  curl -X POST --data-binary @config_update_as_envelope.json http://127.0.0.1:7059/protolator/encode/common.Envelope > config_update_as_envelope.pb

Finally, submit the config update transaction to ordering to perform a config
update.

.. code:: bash

  peer channel update -f config_update_as_envelope.pb -c testchainid -o 127.0.0.1:7050

Adding an organization
----------------------

First start the ``configtxlator``:

.. code:: bash

  $ configtxlator start
  2017-05-31 12:57:22.499 EDT [configtxlator] main -> INFO 001 Serving HTTP requests on port: 7059

Start the orderer using the ``SampleDevModeSolo`` profile option.

.. code:: bash

  ORDERER_GENERAL_LOGLEVEL=debug ORDERER_GENERAL_GENESISPROFILE=SampleDevModeSolo orderer

The process to add an organization then follows exactly like the batch size
example. However, instead of setting the batch size, a new org is defined at
the application level. Adding an organization is slightly more involved because
we must first create a channel, then modify its membership set.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
