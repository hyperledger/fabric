# Reconfiguring with configtxlator

## Overview

The `configtxlator` tool was created to support reconfiguration independent of SDKs.  Channel configuration is stored as a transaction in configuration blocks of a channel and may be manipulated directly, such as in the bdd behave tests.  However, at the time of this writing, no SDK natively supports manipulating the configuration directly, so the `configtxlator` tool is designed to provide an API which consumers of any SDK may interact with to assist with configuration updates.

The tool name is a portmanteau of *configtx* and *translator* and is intended to convey that the tool simply converts between different equivalent data representations.  It does not generate configuration, it does not submit or retrieve configuration, it does not modify configuration itself, it simply provides some bijective operations between different views of the configtx format.

The standard usage is expected to be:
1. SDK retrieves latest config
2. `configtxlator` produces human readable version of config
3. User or application edits the config
4. `configtxlator` used to compute config update representation of changes to the config
5. SDK submits signs and submits config

The `configtxlator` tool exposes a truly stateless REST API for interacting with configuration elements.  These REST components support converting the native configuration format to/from a human readable JSON representation, as well as computing configuration updates based on the difference between two configurations.

Because the `configtxlator` service deliberately does not contain any crypto material, or otherwise secret information, it does not include any authorization or access control.  The anticipated typical deployment would be to operate as a sandboxed container, locally with the application, so that there is a dedicated configtxlator process for each consumer of it.

## Building and running the configtxlator

The `configtxlator` binary may be produced by executing `make configtxlator`.

To execute the `configtxlator` simply execute the binary, optionally with the `-serverPort` option to specify a listen port, or accept the default listen port, `7059`.

The binary will start an http server listening on the designated port and is now ready to process requests.

## Proto translation

For extensibility, and because certain fields must be signed over, many proto fields are stored as bytes.  This makes the natural proto to JSON translation using the `jsonpb` package ineffective for producing a human readable version of the protobufs.  Instead, the `configtxlator` exposes a REST component to do a more sophisticated translation.

To convert a proto to its human readable JSON equivalent, simply post the binary proto to the rest target `http://$SERVER:$PORT/protolator/decode/<message.Name>` where `<message.Name>` is the fully qualified proto name of the message.

For instance, to decode a configuration block saved as `configuration_block.pb`, run the command:

```
curl -X POST --data-binary @configuration_block.pb http://127.0.0.1:7059/protolator/decode/common.Block
```

To convert the human readable JSON version of the proto message, simply post the JSON version to `http://$SERVER:$PORT/protolator/encode/<message.Name>` where `<message.Name>` is again the fully qualified proto name of the message.

For instance, re-encode the block saved as `configuration_block.json`, run the command:

```
curl -X POST --data-binary @configuration_block.json http://127.0.0.1:7059/protolator/encode/common.Block
```

Any of the configuration related protos, including `common.Block`, `common.Envelope`, `common.ConfigEnvelope`, `common.ConfigUpdateEnvelope`, `common.Config`, and `common.ConfigUpdate` are valid targets for these URLs.  In the future, other proto decoding types may be added, such as for endorser transactions.

## Config update computation

Given two different configurations, it is possible to compute the config update which transitions between them.  Simply POST the two `common.Config` proto encoded configurations as `multipart/formdata`, with the original as field `original` and the updated as field `updated`, to `http://$SERVER:$PORT/configtxlator/compute/update-from-configs`.

For example, given the original config as the file `original_config.pb` and the updated config as the file `updated_config.pb` for the channel `desiredchannel`:

```
curl -X POST -F channel=desiredchannel -F original=@original_config.pb -F updated=@updated_config.pb http://127.0.0.1:7059/configtxlator/compute/update-from-configs
```

## Bootstrapping example

First build and start the `configtxlator`.

```
$ make configtxlator
build/bin/configtxlator
CGO_CFLAGS=" " GOBIN=/home/yellickj/go/src/github.com/hyperledger/fabric/build/bin go install -tags "" -ldflags "-X github.com/hyperledger/fabric/common/metadata.Version=1.0.0-alpha3-snapshot-42434e60f -X github.com/hyperledger/fabric/common/metadata.BaseVersion=0.3.1 -X github.com/hyperledger/fabric/common/metadata.BaseDockerLabel=org.hyperledger.fabric -X github.com/hyperledger/fabric/common/metadata.DockerNamespace=hyperledger -X github.com/hyperledger/fabric/common/metadata.BaseDockerNamespace=hyperledger" github.com/hyperledger/fabric/common/tools/configtxlator
Binary available as build/bin/configtxlator
```
```
$ configtxlator start
2017-05-31 12:57:22.499 EDT [configtxlator] main -> INFO 001 Serving HTTP requests on port: 7059
```

Then, in another window, build the `configtxgen` tool.

```
$ make configtxgen
build/bin/configtxgen
CGO_CFLAGS=" " GOBIN=/home/yellickj/go/src/github.com/hyperledger/fabric/build/bin go install -ldflags "-X github.com/hyperledger/fabric/common/metadata.Version=1.0.0-alpha3-snapshot-63e0dc80f -X github.com/hyperledger/fabric/common/metadata.BaseVersion=0.3.1 -X github.com/hyperledger/fabric/common/metadata.BaseDockerLabel=org.hyperledger.fabric -X github.com/hyperledger/fabric/common/metadata.DockerNamespace=hyperledger -X github.com/hyperledger/fabric/common/metadata.BaseDockerNamespace=hyperledger" github.com/hyperledger/fabric/common/configtx/tool/configtxgen
Binary available as build/bin/configtxgen
```

It is recommended to run the example by invoking the script `fabric/examples/configtxupdate/bootstrap_batchsize/script.sh` as follows:

```
INTERACTIVE=true ./script.sh
```

This will write out what is happening at each step, pause, and allow you to inspect the artifacts generated before proceeding. Alternatively, you may run the steps below manually.

----
First produce a genesis block for the ordering system channel.

```
$ configtxgen -outputBlock genesis_block.pb
2017-05-31 14:15:16.634 EDT [common/configtx/tool] main -> INFO 001 Loading configuration
2017-05-31 14:15:16.646 EDT [common/configtx/tool] doOutputBlock -> INFO 002 Generating genesis block
2017-05-31 14:15:16.646 EDT [common/configtx/tool] doOutputBlock -> INFO 003 Writing genesis block
```
Decode the genesis block into a human editable form.
```
curl -X POST --data-binary @genesis_block.pb http://127.0.0.1:7059/protolator/decode/common.Block > genesis_block.json
```
Edit the `genesis_block.json` file in your favorite JSON editor, or manipulate it programmatically, here, we use the JSON CLI tool `jq`.  For simplicity, we are editing the batch size for the channel, because it is a single numeric field, but any edits, including policy and MSP edits may be made here.
```
$ export MAXBATCHSIZEPATH=".data.data[0].payload.data.config.channel_group.groups.Orderer.values.BatchSize.value.max_message_count"
# Display the old batch size
$ jq "$MAXBATCHSIZEPATH" genesis_block.json
10

# Set the new batch size
$ jq "$MAXBATCHSIZEPATH = 20" genesis_block.json  > updated_genesis_block.json

# Display the new batch size
$ jq "$MAXBATCHSIZEPATH" updated_genesis_block.json
20
```
The genesis block is now ready to be re-encoded into the native proto form to be used for bootstrapping.
```
curl -X POST --data-binary @updated_genesis_block.json http://127.0.0.1:7059/protolator/encode/common.Block > updated_genesis_block.pb
```
The `updated_genesis_block.pb` file may now be used as the genesis block for bootstrapping an ordering system channel.

## Reconfiguration example

First build and start the `configtxlator`.

```
$ make configtxlator
build/bin/configtxlator
CGO_CFLAGS=" " GOBIN=/home/yellickj/go/src/github.com/hyperledger/fabric/build/bin go install -tags "" -ldflags "-X github.com/hyperledger/fabric/common/metadata.Version=1.0.0-alpha3-snapshot-42434e60f -X github.com/hyperledger/fabric/common/metadata.BaseVersion=0.3.1 -X github.com/hyperledger/fabric/common/metadata.BaseDockerLabel=org.hyperledger.fabric -X github.com/hyperledger/fabric/common/metadata.DockerNamespace=hyperledger -X github.com/hyperledger/fabric/common/metadata.BaseDockerNamespace=hyperledger" github.com/hyperledger/fabric/common/tools/configtxlator
Binary available as build/bin/configtxlator
```
```
$ configtxlator start
2017-05-31 12:57:22.499 EDT [configtxlator] main -> INFO 001 Serving HTTP requests on port: 7059
```

Then, in another window, build the orderer and peer.

```
$ make peer
Installing chaintool
curl -L https://github.com/hyperledger/fabric-chaintool/releases/download/v0.10.3/chaintool > build/bin/chaintool
...
CGO_CFLAGS=" " GOBIN=/home/yellickj/go/src/github.com/hyperledger/fabric/build/bin go install -tags "" -ldflags "-X github.com/hyperledger/fabric/common/metadata.Version=1.0.0-alpha3-snapshot-63e0dc80f -X github.com/hyperledger/fabric/common/metadata.BaseVersion=0.3.1 -X github.com/hyperledger/fabric/common/metadata.BaseDockerLabel=org.hyperledger.fabric -X github.com/hyperledger/fabric/common/metadata.DockerNamespace=hyperledger -X github.com/hyperledger/fabric/common/metadata.BaseDockerNamespace=hyperledger" github.com/hyperledger/fabric/peer
Binary available as build/bin/peer

$ make orderer
build/bin/orderer
CGO_CFLAGS=" " GOBIN=/home/yellickj/go/src/github.com/hyperledger/fabric/build/bin go install -tags "" -ldflags "-X github.com/hyperledger/fabric/common/metadata.Version=1.0.0-alpha3-snapshot-63e0dc80f -X github.com/hyperledger/fabric/common/metadata.BaseVersion=0.3.1 -X github.com/hyperledger/fabric/common/metadata.BaseDockerLabel=org.hyperledger.fabric -X github.com/hyperledger/fabric/common/metadata.DockerNamespace=hyperledger -X github.com/hyperledger/fabric/common/metadata.BaseDockerNamespace=hyperledger" github.com/hyperledger/fabric/orderer
Binary available as build/bin/orderer
```
Start the orderer using the default options, including the provisional bootstrapper which will create a `testchainid` ordering system channel.

```
ORDERER_GENERAL_LOGLEVEL=debug orderer
```

Reconfiguring a channel can be performed in a very similar way to modifying a genesis config.

The recommended path to proceed with this example is to run the script located at `fabric/examples/configtxupdate/reconfigure_batchsize/script.sh` by invoking

```
INTERACTIVE=true ./script.sh
```

This will write out what is happening at each step, pause, and allow you to inspect the artifacts generated before proceeding. Alternatively, you may run the steps below manually.

----

At this point, executing `examples/reconfig/script.sh` in another window will increase the batch size of the ordering system channel by 1, or, you may follow the steps below to do the process interactively.

```
$ peer channel fetch config config_block.pb -o 127.0.0.1:7050 -c testchainid
2017-05-31 15:11:37.617 EDT [msp] getMspConfig -> INFO 001 intermediate certs folder not found at [/home/yellickj/go/src/github.com/hyperledger/fabric/sampleconfig/msp/intermediatecerts]. Skipping.: [stat /home/yellickj/go/src/github.com/hyperledger/fabric/sampleconfig/msp/intermediatecerts: no such file or directory]
2017-05-31 15:11:37.617 EDT [msp] getMspConfig -> INFO 002 crls folder not found at [/home/yellickj/go/src/github.com/hyperledger/fabric/sampleconfig/msp/intermediatecerts]. Skipping.: [stat /home/yellickj/go/src/github.com/hyperledger/fabric/sampleconfig/msp/crls: no such file or directory]
Received block:  1
Received block:  1
2017-05-31 15:11:37.635 EDT [main] main -> INFO 003 Exiting.....
```

Send the config block to the `configtxlator` service for decoding:

```
curl -X POST --data-binary @config_block.pb http://127.0.0.1:7059/protolator/decode/common.Block > config_block.json
```

Extract the config section from the block:

```
jq .data.data[0].payload.data.config config_block.json > config.json
```

Edit the config, saving it as a new `updated_config.json`.  Here, we set the batch size to 30.

```
jq ".channel_group.groups.Orderer.values.BatchSize.value.max_message_count = 30" config.json  > updated_config.json
```

Re-encode both the original config, and the updated config into proto.

```
curl -X POST --data-binary @config.json http://127.0.0.1:7059/protolator/encode/common.Config > config.pb
```

```
curl -X POST --data-binary @updated_config.json http://127.0.0.1:7059/protolator/encode/common.Config > updated_config.pb
```

Now, with both configs properly encoded, send them to the `configtxlator` service to compute the config update which transitions between the two.

```
curl -X POST -F original=@config.pb -F updated=@updated_config.pb http://127.0.0.1:7059/configtxlator/compute/update-from-configs -F channel=testchainid > config_update.pb
```

At this point, the computed config update is now prepared, and traditionally, an SDK would be used to sign and wrap this message, but in the interest of using only the peer cli, the `configtxlator` can also be used for this task.

First, we decode the ConfigUpdate so that we may work with it as text.

```
$ curl -X POST --data-binary @config_update.pb http://127.0.0.1:7059/protolator/decode/common.ConfigUpdate > config_update.json
```

Then, we wrap it in an envelope message.

```
echo '{"payload":{"header":{"channel_header":{"channel_id":"testchainid", "type":2}},"data":{"config_update":'$(cat config_update.json)'}}}' > config_update_as_envelope.json
```

And finally, convert it back into the proto form of a full fledged config transaction.

```
curl -X POST --data-binary @config_update_as_envelope.json http://127.0.0.1:7059/protolator/encode/common.Envelope > config_update_as_envelope.pb
```

Finally, submit the config update transaction to ordering to perform a config update.

```
peer channel update -f config_update_as_envelope.pb -c testchainid -o 127.0.0.1:7050
```

## Adding an organization

First build and start the `configtxlator`.

```
$ make configtxlator
build/bin/configtxlator
CGO_CFLAGS=" " GOBIN=/home/yellickj/go/src/github.com/hyperledger/fabric/build/bin go install -tags "" -ldflags "-X github.com/hyperledger/fabric/common/metadata.Version=1.0.0-alpha3-snapshot-42434e60f -X github.com/hyperledger/fabric/common/metadata.BaseVersion=0.3.1 -X github.com/hyperledger/fabric/common/metadata.BaseDockerLabel=org.hyperledger.fabric -X github.com/hyperledger/fabric/common/metadata.DockerNamespace=hyperledger -X github.com/hyperledger/fabric/common/metadata.BaseDockerNamespace=hyperledger" github.com/hyperledger/fabric/common/tools/configtxlator
Binary available as build/bin/configtxlator
```
```
$ configtxlator start
2017-05-31 12:57:22.499 EDT [configtxlator] main -> INFO 001 Serving HTTP requests on port: 7059
```

Then, in another window, build the `configtxgen` tool.

```
$ make configtxgen
build/bin/configtxgen
CGO_CFLAGS=" " GOBIN=/home/yellickj/go/src/github.com/hyperledger/fabric/build/bin go install -ldflags "-X github.com/hyperledger/fabric/common/metadata.Version=1.0.0-alpha3-snapshot-63e0dc80f -X github.com/hyperledger/fabric/common/metadata.BaseVersion=0.3.1 -X github.com/hyperledger/fabric/common/metadata.BaseDockerLabel=org.hyperledger.fabric -X github.com/hyperledger/fabric/common/metadata.DockerNamespace=hyperledger -X github.com/hyperledger/fabric/common/metadata.BaseDockerNamespace=hyperledger" github.com/hyperledger/fabric/common/configtx/tool/configtxgen
Binary available as build/bin/configtxgen
```

Then, build the orderer and peer.

```
$ make peer
Installing chaintool
curl -L https://github.com/hyperledger/fabric-chaintool/releases/download/v0.10.3/chaintool > build/bin/chaintool
...
CGO_CFLAGS=" " GOBIN=/home/yellickj/go/src/github.com/hyperledger/fabric/build/bin go install -tags "" -ldflags "-X github.com/hyperledger/fabric/common/metadata.Version=1.0.0-alpha3-snapshot-63e0dc80f -X github.com/hyperledger/fabric/common/metadata.BaseVersion=0.3.1 -X github.com/hyperledger/fabric/common/metadata.BaseDockerLabel=org.hyperledger.fabric -X github.com/hyperledger/fabric/common/metadata.DockerNamespace=hyperledger -X github.com/hyperledger/fabric/common/metadata.BaseDockerNamespace=hyperledger" github.com/hyperledger/fabric/peer
Binary available as build/bin/peer

$ make orderer
build/bin/orderer
CGO_CFLAGS=" " GOBIN=/home/yellickj/go/src/github.com/hyperledger/fabric/build/bin go install -tags "" -ldflags "-X github.com/hyperledger/fabric/common/metadata.Version=1.0.0-alpha3-snapshot-63e0dc80f -X github.com/hyperledger/fabric/common/metadata.BaseVersion=0.3.1 -X github.com/hyperledger/fabric/common/metadata.BaseDockerLabel=org.hyperledger.fabric -X github.com/hyperledger/fabric/common/metadata.DockerNamespace=hyperledger -X github.com/hyperledger/fabric/common/metadata.BaseDockerNamespace=hyperledger" github.com/hyperledger/fabric/orderer
Binary available as build/bin/orderer
```

Start the orderer using the `SampleDevModeSolo` profile option.

```
ORDERER_GENERAL_LOGLEVEL=debug ORDERER_GENERAL_GENESISPROFILE=SampleDevModeSolo orderer
```

The process to add an organization then follows exactly like the batch size example, but, instead of setting the batch size, a new org is defined at the application level.  Adding an organization is slightly more involved, because we must first create a channel, then modify its membership set.

To see this example run the script `fabric/examples/configtxupdate/reconfig_membership/script.sh` by:

```
INTERACTIVE=true ./script.sh
```

Running the script interactively (as above), you may inspect the artifacts produced as they appear in the `example_output` directory.
