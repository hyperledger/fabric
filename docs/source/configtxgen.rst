Configuring using the configtxgen tool
======================================

This document describe the usage for the ``configtxgen`` utility for
manipulating fabric channel configuration.

For now, the tool is primarily focused on generating the genesis block
for bootstrapping the orderer, but it is intended to be enhanced in the
future for generating new channel configurations as well as
reconfiguring existing channels.

Building the tool
-----------------

Building the tool is as simple as ``make configtxgen``. This will create
a ``configtxgen`` binary at ``build/bin/configtxgen`` which is included
in the Vagrant development environment path by default.

Configuration Profiles
----------------------

The configuration parameters supplied to the ``configtxgen`` tool are
primarily provided by the ``configtx.yaml`` file. This file is located
at ``fabric/common/configtx/tool/configtx.yaml`` in the fabric.git
repository.

This configuration file is split primarily into three pieces.

1. The ``Profiles`` section. By default, this section includes some
   sample configurations which can be used for development or testing
   scenarios, and refer to crypto material present in the fabric.git
   tree. These profiles can make a good starting point for construction
   a real deployment profile. The ``configtxgen`` tool allows you to
   specify the profile it is operating under by passing the ``-profile``
   flag. Profiles may explicitly declare all configuration, but usually
   inherit configuration from the defaults in (3) below.
2. The ``Organizations`` section. By default, this section includes a
   single reference to the sampleconfig MSP definition. For production
   deployments, the sample organization should be removed, and the MSP
   definitions of the network members should be referenced and defined
   instead. Each element in the ``Organizations`` section should be
   tagged with an anchor label such as ``&orgName`` which will allow the
   definition to be referenced in the ``Profiles`` sections.
3. The default sections. There are default sections for ``Orderer`` and
   ``Application`` configuration, these include attributes like
   ``BatchTimeout`` and are generally used as the base inherited values
   for the profiles.

This configuration file may be edited, or, individual properties may be
overridden by setting environment variables, such as
``CONFIGTX_ORDERER_ORDERERTYPE=kafka``. Note that the ``Profiles``
element and profile name do not need to be specified.

Bootstrapping the orderer
-------------------------

After creating a configuration profile as desired, simply invoke

::

    configtxgen -profile &lt;profile_name&gt;

This will produce a ``genesis.block`` file in the current directory. You
may optionally specify another filename by passing in the ``-path``
parameter, or, you may skip the writing of the file by passing the
``dryRun`` parameter if you simply wish to test parsing of the file.

Then, to utilize this genesis block, before starting the orderer, simply
specify ``ORDERER_GENERAL_GENESISMETHOD=file`` and
``ORDERER_GENERAL_GENESISFILE=$PWD/genesis.block`` or modify the
``orderer.yaml`` file to encode these values.

Creating a channel
------------------

The tool can also output a channel creation tx by executing

::

    configtxgen -profile <profile_name> -outputCreateChannelTx <output.txname>

This will output a marshaled ``Envelope`` message which may be sent to
broadcast to create a channel.

Reviewing a configuration
-------------------------

In addition to creating configuration, the ``configtxgen`` tool is also
capable of inspecting configuration.

It supports inspecting both configuration blocks, and configuration
transactions. You may use the inspect flags ``-inspectBlock`` and
``-inspectChannelCreateTx`` respectively with the path to a file to
inspect to output a human readable (JSON) representation of the
configuration.

You may even wish to combine the inspection with generation. For
example:

::

    $ build/bin/configtxgen -channelID foo -outputBlock foo.block -inspectBlock foo.block
    2017/03/01 21:24:24 Loading configuration
    2017/03/01 21:24:24 Checking for configtx.yaml at:
    2017/03/01 21:24:24 Checking for configtx.yaml at:
    2017/03/01 21:24:24 Checking for configtx.yaml at: /home/yellickj/go/src/github.com/hyperledger/fabric/common/configtx/tool
    2017/03/01 21:24:24 map[orderer:map[BatchSize:map[MaxMessageCount:10 AbsoluteMaxBytes:99 MB PreferredMaxBytes:512 KB] Kafka:map[Brokers:[127.0.0.1:9092]] Organizations:<nil> OrdererType:solo Addresses:[127.0.0.1:7050] BatchTimeout:10s] application:map[Organizations:<nil>] profiles:map[SampleInsecureSolo:map[Orderer:map[BatchTimeout:10s BatchSize:map[MaxMessageCount:10 AbsoluteMaxBytes:99 MB PreferredMaxBytes:512 KB] Kafka:map[Brokers:[127.0.0.1:9092]] Organizations:<nil> OrdererType:solo Addresses:[127.0.0.1:7050]] Application:map[Organizations:<nil>]] SampleInsecureKafka:map[Orderer:map[Addresses:[127.0.0.1:7050] BatchTimeout:10s BatchSize:map[AbsoluteMaxBytes:99 MB PreferredMaxBytes:512 KB MaxMessageCount:10] Kafka:map[Brokers:[127.0.0.1:9092]] Organizations:<nil> OrdererType:kafka] Application:map[Organizations:<nil>]] SampleSingleMSPSolo:map[Orderer:map[OrdererType:solo Addresses:[127.0.0.1:7050] BatchTimeout:10s BatchSize:map[MaxMessageCount:10 AbsoluteMaxBytes:99 MB PreferredMaxBytes:512 KB] Kafka:map[Brokers:[127.0.0.1:9092]] Organizations:[map[Name:SampleOrg ID:DEFAULT MSPDir:msp/sampleconfig BCCSP:map[Default:SW SW:map[Hash:SHA3 Security:256 FileKeyStore:map[KeyStore:<nil>]]] AnchorPeers:[map[Host:127.0.0.1 Port:7051]]]]] Application:map[Organizations:[map[Name:SampleOrg ID:DEFAULT MSPDir:msp/sampleconfig BCCSP:map[Default:SW SW:map[Hash:SHA3 Security:256 FileKeyStore:map[KeyStore:<nil>]]] AnchorPeers:[map[Port:7051 Host:127.0.0.1]]]]]]] organizations:[map[Name:SampleOrg ID:DEFAULT MSPDir:msp/sampleconfig BCCSP:map[Default:SW SW:map[Hash:SHA3 Security:256 FileKeyStore:map[KeyStore:<nil>]]] AnchorPeers:[map[Host:127.0.0.1 Port:7051]]]]]
    2017/03/01 21:24:24 Generating genesis block
    2017/03/01 21:24:24 Writing genesis block
    2017/03/01 21:24:24 Inspecting block
    2017/03/01 21:24:24 Parsing genesis block
    Config for channel: foo
    {
        "": {
            "Values": {},
            "Groups": {
                "/Channel": {
                    "Values": {
                        "HashingAlgorithm": {
                            "Version": "0",
                            "ModPolicy": "",
                            "Value": {
                                "name": "SHA256"
                            }
                        },
                        "BlockDataHashingStructure": {
                            "Version": "0",
                            "ModPolicy": "",
                            "Value": {
                                "width": 4294967295
                            }
                        },
                        "OrdererAddresses": {
                            "Version": "0",
                            "ModPolicy": "",
                            "Value": {
                                "addresses": [
                                    "127.0.0.1:7050"
                                ]
                            }
                        }
                    },
                    "Groups": {
                        "/Channel/Orderer": {
                            "Values": {
                                "ChainCreationPolicyNames": {
                                    "Version": "0",
                                    "ModPolicy": "",
                                    "Value": {
                                        "names": [
                                            "AcceptAllPolicy"
                                        ]
                                    }
                                },
                                "ConsensusType": {
                                    "Version": "0",
                                    "ModPolicy": "",
                                    "Value": {
                                        "type": "solo"
                                    }
                                },
                                "BatchSize": {
                                    "Version": "0",
                                    "ModPolicy": "",
                                    "Value": {
                                        "maxMessageCount": 10,
                                        "absoluteMaxBytes": 103809024,
                                        "preferredMaxBytes": 524288
                                    }
                                },
                                "BatchTimeout": {
                                    "Version": "0",
                                    "ModPolicy": "",
                                    "Value": {
                                        "timeout": "10s"
                                    }
                                },
                                "IngressPolicyNames": {
                                    "Version": "0",
                                    "ModPolicy": "",
                                    "Value": {
                                        "names": [
                                            "AcceptAllPolicy"
                                        ]
                                    }
                                },
                                "EgressPolicyNames": {
                                    "Version": "0",
                                    "ModPolicy": "",
                                    "Value": {
                                        "names": [
                                            "AcceptAllPolicy"
                                        ]
                                    }
                                }
                            },
                            "Groups": {}
                        },
                        "/Channel/Application": {
                            "Values": {},
                            "Groups": {}
                        }
                    }
                }
            }
        }
    }
