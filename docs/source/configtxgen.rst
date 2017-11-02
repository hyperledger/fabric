Channel Configuration (configtxgen)
===================================

This document describe the usage for the ``configtxgen`` utility for
manipulating Hyperledger Fabric channel configuration.

For now, the tool is primarily focused on generating the genesis block
for bootstrapping the orderer, but it is intended to be enhanced in the
future for generating new channel configurations as well as
reconfiguring existing channels.

Configuration Profiles
----------------------

The configuration parameters supplied to the ``configtxgen`` tool are
primarily provided by the ``configtx.yaml`` file. This file is located
at ``fabric/sampleconfig/configtx.yaml`` in the fabric.git
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

    configtxgen -profile <profile_name> -outputBlock orderer_genesisblock.pb

This will produce an ``orderer_genesisblock.pb`` file in the current directory.
This genesis block is used to bootstrap the ordering system channel, which the
orderers use to authorize and orchestrate creation of other channels.  By
default, the channel ID encoded into the genesis block by ``configtxgen`` will be
``testchainid``.  It is recommended that you modify this identifier to something
which will be globally unique.

Then, to utilize this genesis block, before starting the orderer, simply
specify ``ORDERER_GENERAL_GENESISMETHOD=file`` and
``ORDERER_GENERAL_GENESISFILE=$PWD/orderer_genesisblock.pb`` or modify the
``orderer.yaml`` file to encode these values.

Creating a channel
------------------

The tool can also output a channel creation tx by executing

::

    configtxgen -profile <profile_name> -channelID <channel_name> -outputCreateChannelTx <tx_filename>

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

    $ build/bin/configtxgen -channelID foo -outputBlock foo_genesisblock.pb -inspectBlock foo_genesisblock.pb
    2017-11-02 17:56:04.489 EDT [common/tools/configtxgen] main -> INFO 001 Loading configuration
    2017-11-02 17:56:04.564 EDT [common/tools/configtxgen] doOutputBlock -> INFO 002 Generating genesis block
    2017-11-02 17:56:04.564 EDT [common/tools/configtxgen] doOutputBlock -> INFO 003 Writing genesis block
    2017-11-02 17:56:04.564 EDT [common/tools/configtxgen] doInspectBlock -> INFO 004 Inspecting block
    2017-11-02 17:56:04.564 EDT [common/tools/configtxgen] doInspectBlock -> INFO 005 Parsing genesis block
    {
      "data": {
        "data": [
          {
            "payload": {
              "data": {
                "config": {
                  "channel_group": {
                    "groups": {
                      "Consortiums": {
                        "groups": {
                          "SampleConsortium": {
                            "mod_policy": "/Channel/Orderer/Admins",
                            "values": {
                              "ChannelCreationPolicy": {
                                "mod_policy": "/Channel/Orderer/Admins",
                                "value": {
                                  "type": 3,
                                  "value": {
                                    "rule": "ANY",
                                    "sub_policy": "Admins"
                                  }
                                },
                                "version": "0"
                              }
                            },
                            "version": "0"
                          }
                        },
                        "mod_policy": "/Channel/Orderer/Admins",
                        "policies": {
                          "Admins": {
                            "mod_policy": "/Channel/Orderer/Admins",
                            "policy": {
                              "type": 1,
                              "value": {
                                "rule": {
                                  "n_out_of": {
                                    "n": 0
                                  }
                                },
                                "version": 0
                              }
                            },
                            "version": "0"
                          }
                        },
                        "version": "0"
                      },
                      "Orderer": {
                        "mod_policy": "Admins",
                        "policies": {
                          "Admins": {
                            "mod_policy": "Admins",
                            "policy": {
                              "type": 3,
                              "value": {
                                "rule": "MAJORITY",
                                "sub_policy": "Admins"
                              }
                            },
                            "version": "0"
                          },
                          "BlockValidation": {
                            "mod_policy": "Admins",
                            "policy": {
                              "type": 3,
                              "value": {
                                "rule": "ANY",
                                "sub_policy": "Writers"
                              }
                            },
                            "version": "0"
                          },
                          "Readers": {
                            "mod_policy": "Admins",
                            "policy": {
                              "type": 3,
                              "value": {
                                "rule": "ANY",
                                "sub_policy": "Readers"
                              }
                            },
                            "version": "0"
                          },
                          "Writers": {
                            "mod_policy": "Admins",
                            "policy": {
                              "type": 3,
                              "value": {
                                "rule": "ANY",
                                "sub_policy": "Writers"
                              }
                            },
                            "version": "0"
                          }
                        },
                        "values": {
                          "BatchSize": {
                            "mod_policy": "Admins",
                            "value": {
                              "absolute_max_bytes": 10485760,
                              "max_message_count": 10,
                              "preferred_max_bytes": 524288
                            },
                            "version": "0"
                          },
                          "BatchTimeout": {
                            "mod_policy": "Admins",
                            "value": {
                              "timeout": "2s"
                            },
                            "version": "0"
                          },
                          "ChannelRestrictions": {
                            "mod_policy": "Admins",
                            "version": "0"
                          },
                          "ConsensusType": {
                            "mod_policy": "Admins",
                            "value": {
                              "type": "solo"
                            },
                            "version": "0"
                          }
                        },
                        "version": "0"
                      }
                    },
                    "mod_policy": "Admins",
                    "policies": {
                      "Admins": {
                        "mod_policy": "Admins",
                        "policy": {
                          "type": 3,
                          "value": {
                            "rule": "MAJORITY",
                            "sub_policy": "Admins"
                          }
                        },
                        "version": "0"
                      },
                      "Readers": {
                        "mod_policy": "Admins",
                        "policy": {
                          "type": 3,
                          "value": {
                            "rule": "ANY",
                            "sub_policy": "Readers"
                          }
                        },
                        "version": "0"
                      },
                      "Writers": {
                        "mod_policy": "Admins",
                        "policy": {
                          "type": 3,
                          "value": {
                            "rule": "ANY",
                            "sub_policy": "Writers"
                          }
                        },
                        "version": "0"
                      }
                    },
                    "values": {
                      "BlockDataHashingStructure": {
                        "mod_policy": "Admins",
                        "value": {
                          "width": 4294967295
                        },
                        "version": "0"
                      },
                      "HashingAlgorithm": {
                        "mod_policy": "Admins",
                        "value": {
                          "name": "SHA256"
                        },
                        "version": "0"
                      },
                      "OrdererAddresses": {
                        "mod_policy": "/Channel/Orderer/Admins",
                        "value": {
                          "addresses": [
                            "127.0.0.1:7050"
                          ]
                        },
                        "version": "0"
                      }
                    },
                    "version": "0"
                  },
                  "sequence": "0",
                  "type": 0
                }
              },
              "header": {
                "channel_header": {
                  "channel_id": "foo",
                  "epoch": "0",
                  "timestamp": "2017-11-02T21:56:04.000Z",
                  "tx_id": "6acfe1257c23a4f844cc299cbf53acc7bf8fa8bcf8aae8d049193098fe982eab",
                  "type": 1,
                  "version": 1
                },
                "signature_header": {
                  "nonce": "eZOKru6jmeiWykBtSDwnkGjyQt69GwuS"
                }
              }
            }
          }
        ]
      },
      "header": {
        "data_hash": "/86I/7NScbH/bHcDcYG0/9qTmVPWVoVVfSN8NKMARKI=",
        "number": "0"
      },
      "metadata": {
        "metadata": [
          "",
          "",
          "",
          ""
        ]
      }
    }

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
