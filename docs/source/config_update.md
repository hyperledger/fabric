# Updating a channel configuration

*Audience: network administrators, node administrators*

## What is a channel configuration?

Like many complex systems, Hyperledger Fabric networks are comprised of both **structure** and a number related of **processes**.

* **Structure**: encompassing users (like admins), organizations, peers, ordering nodes, CAs, smart contracts, and applications.
* **Process**: the way these structures interact. Most important of these are [Policies](./policies/policies.html), the rules that govern which users can do what, and under what conditions.

Information identifying the structure of blockchain networks and the processes governing how structures interact are contained in **channel configurations**. These configurations are collectively decided upon by the members of application channels, where transactions between peer organizations happen, and the "orderer system channel" managed by ordering organizations, and are contained in blocks that are committed to the ledger of a channel.

Because configurations are contained in blocks (the first of these is known as the genesis block with the latest representing the current configuration of the channel), the process for updating a channel configuration (changing the structure by adding members, for example, or processes by modifying channel policies) is known as a **configuration update transaction**.

In production networks, these configuration update transactions will normally be proposed by a single channel admin after an out of band discussion, just as the initial configuration of the channel will be decided on out of band by the initial members of the channel.

In this topic, we'll:

* Show a full sample configuration of an application channel.
* Discuss many of the channel parameters that can be edited.
* Show the process for updating a channel configuration, including the commands necessary to pull, translate, and scope a configuration into something that humans can read.
* Discuss the methods that can be used to edit a channel configuration.
* Show the process used to reformat a configuration and get the signatures necessary for it to be approved.

## Channel parameters that can be updated

Channels are highly configurable, but not infinitely so. Once certain things about a channel (for example, the name of the channel) have been specified, they cannot be changed. And changing one of the parameters we'll talk about in this topic requires satisfying the relevant policy as specified in the channel configuration.

In this section, we'll look a sample channel configuration and show the configuration parameters that can be updated.

### Sample channel configuration

To see what the configuration file of an application channel looks like after it has been pulled and scoped, click **Click here to see the config** below. For ease of readability, it might be helpful to put this config into a viewer that supports JSON folding, like atom or Visual Studio.

Note: for simplicity, we are only showing an application channel configuration here. The configuration of the orderer system channel is very similar, but not identical, to the configuration of an application channel. However, the same basic rules and structure apply, as do the commands to pull and edit a configuration, as you can see in our topic on [Updating the capability level of a channel](./updating_capabilities.html).

<details>
  <summary>
    **Click here to see the config**. Note that this is the configuration of an application channel, not the orderer system channel.
  </summary>
  ```
  {
  "channel_group": {
    "groups": {
      "Application": {
        "groups": {
          "Org1MSP": {
            "groups": {},
            "mod_policy": "Admins",
            "policies": {
              "Admins": {
                "mod_policy": "Admins",
                "policy": {
                  "type": 1,
                  "value": {
                    "identities": [
                      {
                        "principal": {
                          "msp_identifier": "Org1MSP",
                          "role": "ADMIN"
                        },
                        "principal_classification": "ROLE"
                      }
                    ],
                    "rule": {
                      "n_out_of": {
                        "n": 1,
                        "rules": [
                          {
                            "signed_by": 0
                          }
                        ]
                      }
                    },
                    "version": 0
                  }
                },
                "version": "0"
              },
              "Readers": {
                "mod_policy": "Admins",
                "policy": {
                  "type": 1,
                  "value": {
                    "identities": [
                      {
                        "principal": {
                          "msp_identifier": "Org1MSP",
                          "role": "ADMIN"
                        },
                        "principal_classification": "ROLE"
                      },
                      {
                        "principal": {
                          "msp_identifier": "Org1MSP",
                          "role": "PEER"
                        },
                        "principal_classification": "ROLE"
                      },
                      {
                        "principal": {
                          "msp_identifier": "Org1MSP",
                          "role": "CLIENT"
                        },
                        "principal_classification": "ROLE"
                      }
                    ],
                    "rule": {
                      "n_out_of": {
                        "n": 1,
                        "rules": [
                          {
                            "signed_by": 0
                          },
                          {
                            "signed_by": 1
                          },
                          {
                            "signed_by": 2
                          }
                        ]
                      }
                    },
                    "version": 0
                  }
                },
                "version": "0"
              },
              "Writers": {
                "mod_policy": "Admins",
                "policy": {
                  "type": 1,
                  "value": {
                    "identities": [
                      {
                        "principal": {
                          "msp_identifier": "Org1MSP",
                          "role": "ADMIN"
                        },
                        "principal_classification": "ROLE"
                      },
                      {
                        "principal": {
                          "msp_identifier": "Org1MSP",
                          "role": "CLIENT"
                        },
                        "principal_classification": "ROLE"
                      }
                    ],
                    "rule": {
                      "n_out_of": {
                        "n": 1,
                        "rules": [
                          {
                            "signed_by": 0
                          },
                          {
                            "signed_by": 1
                          }
                        ]
                      }
                    },
                    "version": 0
                  }
                },
                "version": "0"
              }
            },
            "values": {
              "AnchorPeers": {
                "mod_policy": "Admins",
                "value": {
                  "anchor_peers": [
                    {
                      "host": "peer0.org1.example.com",
                      "port": 7051
                    }
                  ]
                },
                "version": "0"
              },
              "MSP": {
                "mod_policy": "Admins",
                "value": {
                  "config": {
                    "admins": [],
                    "crypto_config": {
                      "identity_identifier_hash_function": "SHA256",
                      "signature_hash_family": "SHA2"
                    },
                    "fabric_node_ous": {
                      "admin_ou_identifier": {
                        "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNVakNDQWZpZ0F3SUJBZ0lSQVBYKzVxL21nckhNR280NFB5bjhYRnd3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpFdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekV1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGt4TVRFME1UTTBPREF3V2hjTk1qa3hNVEV4TVRNME9EQXcKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NUzVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1TNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQkV5TWtxZndLUkJCWDNKY2xGM3c0UkJZdGFGZytPeFgzSW9RanRrZzJodGxsV3dMV3YrWExXdVl2dkpUMTdxZAp1ei9uWGlWRWhhYnQ2VmVkRnBzanJuR2piVEJyTUE0R0ExVWREd0VCL3dRRUF3SUJwakFkQmdOVkhTVUVGakFVCkJnZ3JCZ0VGQlFjREFnWUlLd1lCQlFVSEF3RXdEd1lEVlIwVEFRSC9CQVV3QXdFQi96QXBCZ05WSFE0RUlnUWcKVytUN0RIb2puYXh0TkpJTHAvREd4UmVONkpPZENSUlBIdVhPNzZXY3k5OHdDZ1lJS29aSXpqMEVBd0lEU0FBdwpSUUloQUtUaHZNaFlBR3p6aVpNR1B1TjFpTTBEcHVpaFNydHJXRHJ6NkQ4QTBXOXBBaUI0MTFrTnJzVjN4ZU05ClNnaHFNc2lzK2QxWThEWE9DOXVrZGhBcU5GZ25FUT09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
                        "organizational_unit_identifier": "admin"
                      },
                      "client_ou_identifier": {
                        "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNVakNDQWZpZ0F3SUJBZ0lSQVBYKzVxL21nckhNR280NFB5bjhYRnd3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpFdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekV1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGt4TVRFME1UTTBPREF3V2hjTk1qa3hNVEV4TVRNME9EQXcKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NUzVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1TNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQkV5TWtxZndLUkJCWDNKY2xGM3c0UkJZdGFGZytPeFgzSW9RanRrZzJodGxsV3dMV3YrWExXdVl2dkpUMTdxZAp1ei9uWGlWRWhhYnQ2VmVkRnBzanJuR2piVEJyTUE0R0ExVWREd0VCL3dRRUF3SUJwakFkQmdOVkhTVUVGakFVCkJnZ3JCZ0VGQlFjREFnWUlLd1lCQlFVSEF3RXdEd1lEVlIwVEFRSC9CQVV3QXdFQi96QXBCZ05WSFE0RUlnUWcKVytUN0RIb2puYXh0TkpJTHAvREd4UmVONkpPZENSUlBIdVhPNzZXY3k5OHdDZ1lJS29aSXpqMEVBd0lEU0FBdwpSUUloQUtUaHZNaFlBR3p6aVpNR1B1TjFpTTBEcHVpaFNydHJXRHJ6NkQ4QTBXOXBBaUI0MTFrTnJzVjN4ZU05ClNnaHFNc2lzK2QxWThEWE9DOXVrZGhBcU5GZ25FUT09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
                        "organizational_unit_identifier": "client"
                      },
                      "enable": true,
                      "orderer_ou_identifier": {
                        "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNVakNDQWZpZ0F3SUJBZ0lSQVBYKzVxL21nckhNR280NFB5bjhYRnd3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpFdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekV1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGt4TVRFME1UTTBPREF3V2hjTk1qa3hNVEV4TVRNME9EQXcKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NUzVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1TNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQkV5TWtxZndLUkJCWDNKY2xGM3c0UkJZdGFGZytPeFgzSW9RanRrZzJodGxsV3dMV3YrWExXdVl2dkpUMTdxZAp1ei9uWGlWRWhhYnQ2VmVkRnBzanJuR2piVEJyTUE0R0ExVWREd0VCL3dRRUF3SUJwakFkQmdOVkhTVUVGakFVCkJnZ3JCZ0VGQlFjREFnWUlLd1lCQlFVSEF3RXdEd1lEVlIwVEFRSC9CQVV3QXdFQi96QXBCZ05WSFE0RUlnUWcKVytUN0RIb2puYXh0TkpJTHAvREd4UmVONkpPZENSUlBIdVhPNzZXY3k5OHdDZ1lJS29aSXpqMEVBd0lEU0FBdwpSUUloQUtUaHZNaFlBR3p6aVpNR1B1TjFpTTBEcHVpaFNydHJXRHJ6NkQ4QTBXOXBBaUI0MTFrTnJzVjN4ZU05ClNnaHFNc2lzK2QxWThEWE9DOXVrZGhBcU5GZ25FUT09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
                        "organizational_unit_identifier": "orderer"
                      },
                      "peer_ou_identifier": {
                        "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNVakNDQWZpZ0F3SUJBZ0lSQVBYKzVxL21nckhNR280NFB5bjhYRnd3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpFdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekV1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGt4TVRFME1UTTBPREF3V2hjTk1qa3hNVEV4TVRNME9EQXcKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NUzVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1TNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQkV5TWtxZndLUkJCWDNKY2xGM3c0UkJZdGFGZytPeFgzSW9RanRrZzJodGxsV3dMV3YrWExXdVl2dkpUMTdxZAp1ei9uWGlWRWhhYnQ2VmVkRnBzanJuR2piVEJyTUE0R0ExVWREd0VCL3dRRUF3SUJwakFkQmdOVkhTVUVGakFVCkJnZ3JCZ0VGQlFjREFnWUlLd1lCQlFVSEF3RXdEd1lEVlIwVEFRSC9CQVV3QXdFQi96QXBCZ05WSFE0RUlnUWcKVytUN0RIb2puYXh0TkpJTHAvREd4UmVONkpPZENSUlBIdVhPNzZXY3k5OHdDZ1lJS29aSXpqMEVBd0lEU0FBdwpSUUloQUtUaHZNaFlBR3p6aVpNR1B1TjFpTTBEcHVpaFNydHJXRHJ6NkQ4QTBXOXBBaUI0MTFrTnJzVjN4ZU05ClNnaHFNc2lzK2QxWThEWE9DOXVrZGhBcU5GZ25FUT09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
                        "organizational_unit_identifier": "peer"
                      }
                    },
                    "intermediate_certs": [],
                    "name": "Org1MSP",
                    "organizational_unit_identifiers": [],
                    "revocation_list": [],
                    "root_certs": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNVakNDQWZpZ0F3SUJBZ0lSQVBYKzVxL21nckhNR280NFB5bjhYRnd3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpFdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekV1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGt4TVRFME1UTTBPREF3V2hjTk1qa3hNVEV4TVRNME9EQXcKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NUzVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1TNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQkV5TWtxZndLUkJCWDNKY2xGM3c0UkJZdGFGZytPeFgzSW9RanRrZzJodGxsV3dMV3YrWExXdVl2dkpUMTdxZAp1ei9uWGlWRWhhYnQ2VmVkRnBzanJuR2piVEJyTUE0R0ExVWREd0VCL3dRRUF3SUJwakFkQmdOVkhTVUVGakFVCkJnZ3JCZ0VGQlFjREFnWUlLd1lCQlFVSEF3RXdEd1lEVlIwVEFRSC9CQVV3QXdFQi96QXBCZ05WSFE0RUlnUWcKVytUN0RIb2puYXh0TkpJTHAvREd4UmVONkpPZENSUlBIdVhPNzZXY3k5OHdDZ1lJS29aSXpqMEVBd0lEU0FBdwpSUUloQUtUaHZNaFlBR3p6aVpNR1B1TjFpTTBEcHVpaFNydHJXRHJ6NkQ4QTBXOXBBaUI0MTFrTnJzVjN4ZU05ClNnaHFNc2lzK2QxWThEWE9DOXVrZGhBcU5GZ25FUT09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
                    ],
                    "signing_identity": null,
                    "tls_intermediate_certs": [],
                    "tls_root_certs": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNWekNDQWY2Z0F3SUJBZ0lSQUtvOGhJS0JjeldFOFQrSUZSVWVmZm93Q2dZSUtvWkl6ajBFQXdJd2RqRUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpFdVpYaGhiWEJzWlM1amIyMHhIekFkQmdOVkJBTVRGblJzCmMyTmhMbTl5WnpFdVpYaGhiWEJzWlM1amIyMHdIaGNOTVRreE1URTBNVE0wT0RBd1doY05Namt4TVRFeE1UTTAKT0RBd1dqQjJNUXN3Q1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRQpCeE1OVTJGdUlFWnlZVzVqYVhOamJ6RVpNQmNHQTFVRUNoTVFiM0puTVM1bGVHRnRjR3hsTG1OdmJURWZNQjBHCkExVUVBeE1XZEd4elkyRXViM0puTVM1bGVHRnRjR3hsTG1OdmJUQlpNQk1HQnlxR1NNNDlBZ0VHQ0NxR1NNNDkKQXdFSEEwSUFCRUdGY3N6UmNJWXJnUVltK1JwL0owR3NrRXk4OFlDS2xqY2lSY1BoS3FKVWI3L29aSExRSWRtNQpSV0pLcVNZeSs3R2FqWHlROFQxNG1mY2IrM0ZodkpTamJUQnJNQTRHQTFVZER3RUIvd1FFQXdJQnBqQWRCZ05WCkhTVUVGakFVQmdnckJnRUZCUWNEQWdZSUt3WUJCUVVIQXdFd0R3WURWUjBUQVFIL0JBVXdBd0VCL3pBcEJnTlYKSFE0RUlnUWdOZzd3RHhRSkMwREYvQWo5YUwvbXJQVlJNMkozb2VRelEwdnV2cWgzdm5Fd0NnWUlLb1pJemowRQpBd0lEUndBd1JBSWdkYWZXMjJ0WXZuZVd6bEp2Mlg5WC9qM2VCbTdxSG9xejZ2QmV2cENZd3Q4Q0lHYXJJTm5oCnArRnFyaUVoaDI5SDgrcEVTV1NvZXQ1UzFKQVRrd0srNTJMawotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
                    ]
                  },
                  "type": 0
                },
                "version": "0"
              }
            },
            "version": "1"
          },
          "Org2MSP": {
            "groups": {},
            "mod_policy": "Admins",
            "policies": {
              "Admins": {
                "mod_policy": "Admins",
                "policy": {
                  "type": 1,
                  "value": {
                    "identities": [
                      {
                        "principal": {
                          "msp_identifier": "Org2MSP",
                          "role": "ADMIN"
                        },
                        "principal_classification": "ROLE"
                      }
                    ],
                    "rule": {
                      "n_out_of": {
                        "n": 1,
                        "rules": [
                          {
                            "signed_by": 0
                          }
                        ]
                      }
                    },
                    "version": 0
                  }
                },
                "version": "0"
              },
              "Readers": {
                "mod_policy": "Admins",
                "policy": {
                  "type": 1,
                  "value": {
                    "identities": [
                      {
                        "principal": {
                          "msp_identifier": "Org2MSP",
                          "role": "ADMIN"
                        },
                        "principal_classification": "ROLE"
                      },
                      {
                        "principal": {
                          "msp_identifier": "Org2MSP",
                          "role": "PEER"
                        },
                        "principal_classification": "ROLE"
                      },
                      {
                        "principal": {
                          "msp_identifier": "Org2MSP",
                          "role": "CLIENT"
                        },
                        "principal_classification": "ROLE"
                      }
                    ],
                    "rule": {
                      "n_out_of": {
                        "n": 1,
                        "rules": [
                          {
                            "signed_by": 0
                          },
                          {
                            "signed_by": 1
                          },
                          {
                            "signed_by": 2
                          }
                        ]
                      }
                    },
                    "version": 0
                  }
                },
                "version": "0"
              },
              "Writers": {
                "mod_policy": "Admins",
                "policy": {
                  "type": 1,
                  "value": {
                    "identities": [
                      {
                        "principal": {
                          "msp_identifier": "Org2MSP",
                          "role": "ADMIN"
                        },
                        "principal_classification": "ROLE"
                      },
                      {
                        "principal": {
                          "msp_identifier": "Org2MSP",
                          "role": "CLIENT"
                        },
                        "principal_classification": "ROLE"
                      }
                    ],
                    "rule": {
                      "n_out_of": {
                        "n": 1,
                        "rules": [
                          {
                            "signed_by": 0
                          },
                          {
                            "signed_by": 1
                          }
                        ]
                      }
                    },
                    "version": 0
                  }
                },
                "version": "0"
              }
            },
            "values": {
              "AnchorPeers": {
                "mod_policy": "Admins",
                "value": {
                  "anchor_peers": [
                    {
                      "host": "peer0.org2.example.com",
                      "port": 9051
                    }
                  ]
                },
                "version": "0"
              },
              "MSP": {
                "mod_policy": "Admins",
                "value": {
                  "config": {
                    "admins": [],
                    "crypto_config": {
                      "identity_identifier_hash_function": "SHA256",
                      "signature_hash_family": "SHA2"
                    },
                    "fabric_node_ous": {
                      "admin_ou_identifier": {
                        "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNVakNDQWZpZ0F3SUJBZ0lSQU1JaCt2V1lHTGs5bUFTT1JNb05iVkl3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpJdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekl1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGt4TVRFME1UTTBPREF3V2hjTk1qa3hNVEV4TVRNME9EQXcKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NaTVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1pNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQlBBc2FrbklPUmx2M0pPdnBsQld5VCt0SUlFZWo2U2ZQaTlKTS8yQmYzOWkxdFVpRCt2aVV1Tk43MGlKcXRwRQpUbm50V2htWjA5elAzaVUwcklXWGJLcWpiVEJyTUE0R0ExVWREd0VCL3dRRUF3SUJwakFkQmdOVkhTVUVGakFVCkJnZ3JCZ0VGQlFjREFnWUlLd1lCQlFVSEF3RXdEd1lEVlIwVEFRSC9CQVV3QXdFQi96QXBCZ05WSFE0RUlnUWcKQzl3Y0RKb3FKL2dyUGF3S1d4RnVjNVB3MitTdTBsQVpFcFRGaEdDQlJSTXdDZ1lJS29aSXpqMEVBd0lEU0FBdwpSUUloQU1JaFJOVDVJMHpwTlRaa1dCSkdyblJqSUhkWXYwS2lWL1JKbkd5Yi9XVFFBaUJ6eUJqYnR3L1JyWEV3Clpzb0N1MmtoOUUwOUZIdXl5dGgydUtWc3Y0ZTlnUT09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
                        "organizational_unit_identifier": "admin"
                      },
                      "client_ou_identifier": {
                        "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNVakNDQWZpZ0F3SUJBZ0lSQU1JaCt2V1lHTGs5bUFTT1JNb05iVkl3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpJdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekl1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGt4TVRFME1UTTBPREF3V2hjTk1qa3hNVEV4TVRNME9EQXcKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NaTVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1pNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQlBBc2FrbklPUmx2M0pPdnBsQld5VCt0SUlFZWo2U2ZQaTlKTS8yQmYzOWkxdFVpRCt2aVV1Tk43MGlKcXRwRQpUbm50V2htWjA5elAzaVUwcklXWGJLcWpiVEJyTUE0R0ExVWREd0VCL3dRRUF3SUJwakFkQmdOVkhTVUVGakFVCkJnZ3JCZ0VGQlFjREFnWUlLd1lCQlFVSEF3RXdEd1lEVlIwVEFRSC9CQVV3QXdFQi96QXBCZ05WSFE0RUlnUWcKQzl3Y0RKb3FKL2dyUGF3S1d4RnVjNVB3MitTdTBsQVpFcFRGaEdDQlJSTXdDZ1lJS29aSXpqMEVBd0lEU0FBdwpSUUloQU1JaFJOVDVJMHpwTlRaa1dCSkdyblJqSUhkWXYwS2lWL1JKbkd5Yi9XVFFBaUJ6eUJqYnR3L1JyWEV3Clpzb0N1MmtoOUUwOUZIdXl5dGgydUtWc3Y0ZTlnUT09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
                        "organizational_unit_identifier": "client"
                      },
                      "enable": true,
                      "orderer_ou_identifier": {
                        "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNVakNDQWZpZ0F3SUJBZ0lSQU1JaCt2V1lHTGs5bUFTT1JNb05iVkl3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpJdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekl1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGt4TVRFME1UTTBPREF3V2hjTk1qa3hNVEV4TVRNME9EQXcKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NaTVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1pNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQlBBc2FrbklPUmx2M0pPdnBsQld5VCt0SUlFZWo2U2ZQaTlKTS8yQmYzOWkxdFVpRCt2aVV1Tk43MGlKcXRwRQpUbm50V2htWjA5elAzaVUwcklXWGJLcWpiVEJyTUE0R0ExVWREd0VCL3dRRUF3SUJwakFkQmdOVkhTVUVGakFVCkJnZ3JCZ0VGQlFjREFnWUlLd1lCQlFVSEF3RXdEd1lEVlIwVEFRSC9CQVV3QXdFQi96QXBCZ05WSFE0RUlnUWcKQzl3Y0RKb3FKL2dyUGF3S1d4RnVjNVB3MitTdTBsQVpFcFRGaEdDQlJSTXdDZ1lJS29aSXpqMEVBd0lEU0FBdwpSUUloQU1JaFJOVDVJMHpwTlRaa1dCSkdyblJqSUhkWXYwS2lWL1JKbkd5Yi9XVFFBaUJ6eUJqYnR3L1JyWEV3Clpzb0N1MmtoOUUwOUZIdXl5dGgydUtWc3Y0ZTlnUT09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
                        "organizational_unit_identifier": "orderer"
                      },
                      "peer_ou_identifier": {
                        "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNVakNDQWZpZ0F3SUJBZ0lSQU1JaCt2V1lHTGs5bUFTT1JNb05iVkl3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpJdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekl1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGt4TVRFME1UTTBPREF3V2hjTk1qa3hNVEV4TVRNME9EQXcKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NaTVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1pNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQlBBc2FrbklPUmx2M0pPdnBsQld5VCt0SUlFZWo2U2ZQaTlKTS8yQmYzOWkxdFVpRCt2aVV1Tk43MGlKcXRwRQpUbm50V2htWjA5elAzaVUwcklXWGJLcWpiVEJyTUE0R0ExVWREd0VCL3dRRUF3SUJwakFkQmdOVkhTVUVGakFVCkJnZ3JCZ0VGQlFjREFnWUlLd1lCQlFVSEF3RXdEd1lEVlIwVEFRSC9CQVV3QXdFQi96QXBCZ05WSFE0RUlnUWcKQzl3Y0RKb3FKL2dyUGF3S1d4RnVjNVB3MitTdTBsQVpFcFRGaEdDQlJSTXdDZ1lJS29aSXpqMEVBd0lEU0FBdwpSUUloQU1JaFJOVDVJMHpwTlRaa1dCSkdyblJqSUhkWXYwS2lWL1JKbkd5Yi9XVFFBaUJ6eUJqYnR3L1JyWEV3Clpzb0N1MmtoOUUwOUZIdXl5dGgydUtWc3Y0ZTlnUT09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
                        "organizational_unit_identifier": "peer"
                      }
                    },
                    "intermediate_certs": [],
                    "name": "Org2MSP",
                    "organizational_unit_identifiers": [],
                    "revocation_list": [],
                    "root_certs": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNVakNDQWZpZ0F3SUJBZ0lSQU1JaCt2V1lHTGs5bUFTT1JNb05iVkl3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpJdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekl1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGt4TVRFME1UTTBPREF3V2hjTk1qa3hNVEV4TVRNME9EQXcKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NaTVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1pNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQlBBc2FrbklPUmx2M0pPdnBsQld5VCt0SUlFZWo2U2ZQaTlKTS8yQmYzOWkxdFVpRCt2aVV1Tk43MGlKcXRwRQpUbm50V2htWjA5elAzaVUwcklXWGJLcWpiVEJyTUE0R0ExVWREd0VCL3dRRUF3SUJwakFkQmdOVkhTVUVGakFVCkJnZ3JCZ0VGQlFjREFnWUlLd1lCQlFVSEF3RXdEd1lEVlIwVEFRSC9CQVV3QXdFQi96QXBCZ05WSFE0RUlnUWcKQzl3Y0RKb3FKL2dyUGF3S1d4RnVjNVB3MitTdTBsQVpFcFRGaEdDQlJSTXdDZ1lJS29aSXpqMEVBd0lEU0FBdwpSUUloQU1JaFJOVDVJMHpwTlRaa1dCSkdyblJqSUhkWXYwS2lWL1JKbkd5Yi9XVFFBaUJ6eUJqYnR3L1JyWEV3Clpzb0N1MmtoOUUwOUZIdXl5dGgydUtWc3Y0ZTlnUT09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
                    ],
                    "signing_identity": null,
                    "tls_intermediate_certs": [],
                    "tls_root_certs": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNXRENDQWY2Z0F3SUJBZ0lSQVBoOGJPUzV1aVZLK2xwdjBKZDN5ZUl3Q2dZSUtvWkl6ajBFQXdJd2RqRUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpJdVpYaGhiWEJzWlM1amIyMHhIekFkQmdOVkJBTVRGblJzCmMyTmhMbTl5WnpJdVpYaGhiWEJzWlM1amIyMHdIaGNOTVRreE1URTBNVE0wT0RBd1doY05Namt4TVRFeE1UTTAKT0RBd1dqQjJNUXN3Q1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRQpCeE1OVTJGdUlFWnlZVzVqYVhOamJ6RVpNQmNHQTFVRUNoTVFiM0puTWk1bGVHRnRjR3hsTG1OdmJURWZNQjBHCkExVUVBeE1XZEd4elkyRXViM0puTWk1bGVHRnRjR3hsTG1OdmJUQlpNQk1HQnlxR1NNNDlBZ0VHQ0NxR1NNNDkKQXdFSEEwSUFCRHlhdEd3R2ZLSmFLazZ5ZmVJMGpObHVyWU5rbjdOaG5uNGVYbnVTd0hCazBRMDY4bnZ1Ujg4awprQTBRWm5vR2ZRWkEwU3RRM3JqVCt4b3BnMHFMcVBhamJUQnJNQTRHQTFVZER3RUIvd1FFQXdJQnBqQWRCZ05WCkhTVUVGakFVQmdnckJnRUZCUWNEQWdZSUt3WUJCUVVIQXdFd0R3WURWUjBUQVFIL0JBVXdBd0VCL3pBcEJnTlYKSFE0RUlnUWdGVDh3UThUcWlIVUxqMU1sY21JRWoreUFPSDF1R1NDNGxFRUVadUlTVkZBd0NnWUlLb1pJemowRQpBd0lEU0FBd1JRSWhBTExXb3Z2Z2JWTnJwcC9mbzZwTStoVXNMTTFkT3NncGRKNlRNd2FFdzQrTkFpQnJ0WFArCno5MFd6ZEQvRWpFWlcyM0xmeVhNMWRscXcyVHJEL3FWUVlGYXJRPT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
                    ]
                  },
                  "type": 0
                },
                "version": "0"
              }
            },
            "version": "1"
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
          "Capabilities": {
            "mod_policy": "Admins",
            "value": {
              "capabilities": {
                "V1_4_2": {}
              }
            },
            "version": "0"
          }
        },
        "version": "1"
      },
      "Orderer": {
        "groups": {
          "OrdererOrg": {
            "groups": {},
            "mod_policy": "Admins",
            "policies": {
              "Admins": {
                "mod_policy": "Admins",
                "policy": {
                  "type": 1,
                  "value": {
                    "identities": [
                      {
                        "principal": {
                          "msp_identifier": "OrdererMSP",
                          "role": "ADMIN"
                        },
                        "principal_classification": "ROLE"
                      }
                    ],
                    "rule": {
                      "n_out_of": {
                        "n": 1,
                        "rules": [
                          {
                            "signed_by": 0
                          }
                        ]
                      }
                    },
                    "version": 0
                  }
                },
                "version": "0"
              },
              "Readers": {
                "mod_policy": "Admins",
                "policy": {
                  "type": 1,
                  "value": {
                    "identities": [
                      {
                        "principal": {
                          "msp_identifier": "OrdererMSP",
                          "role": "MEMBER"
                        },
                        "principal_classification": "ROLE"
                      }
                    ],
                    "rule": {
                      "n_out_of": {
                        "n": 1,
                        "rules": [
                          {
                            "signed_by": 0
                          }
                        ]
                      }
                    },
                    "version": 0
                  }
                },
                "version": "0"
              },
              "Writers": {
                "mod_policy": "Admins",
                "policy": {
                  "type": 1,
                  "value": {
                    "identities": [
                      {
                        "principal": {
                          "msp_identifier": "OrdererMSP",
                          "role": "MEMBER"
                        },
                        "principal_classification": "ROLE"
                      }
                    ],
                    "rule": {
                      "n_out_of": {
                        "n": 1,
                        "rules": [
                          {
                            "signed_by": 0
                          }
                        ]
                      }
                    },
                    "version": 0
                  }
                },
                "version": "0"
              }
            },
            "values": {
              "MSP": {
                "mod_policy": "Admins",
                "value": {
                  "config": {
                    "admins": [],
                    "crypto_config": {
                      "identity_identifier_hash_function": "SHA256",
                      "signature_hash_family": "SHA2"
                    },
                    "fabric_node_ous": {
                      "admin_ou_identifier": {
                        "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNQVENDQWVTZ0F3SUJBZ0lSQUxCOWVtTVhObkxaWW56TitkMHNJQ2N3Q2dZSUtvWkl6ajBFQXdJd2FURUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhGREFTQmdOVkJBb1RDMlY0WVcxd2JHVXVZMjl0TVJjd0ZRWURWUVFERXc1allTNWxlR0Z0CmNHeGxMbU52YlRBZUZ3MHhPVEV4TVRReE16UTRNREJhRncweU9URXhNVEV4TXpRNE1EQmFNR2t4Q3pBSkJnTlYKQkFZVEFsVlRNUk13RVFZRFZRUUlFd3BEWVd4cFptOXlibWxoTVJZd0ZBWURWUVFIRXcxVFlXNGdSbkpoYm1OcApjMk52TVJRd0VnWURWUVFLRXd0bGVHRnRjR3hsTG1OdmJURVhNQlVHQTFVRUF4TU9ZMkV1WlhoaGJYQnNaUzVqCmIyMHdXVEFUQmdjcWhrak9QUUlCQmdncWhrak9QUU1CQndOQ0FBVGVXYjlZWUdMNmgwMVlFckdzVS9xZmFzUmEKbUpHcFlWZGxsVTNwNldNUTMwZjcyazBFQitPRVQ3WWM3K0w2TWxxTTdNUDJBYnQ2RWUwV2w2OGpjZGxXbzIwdwphekFPQmdOVkhROEJBZjhFQkFNQ0FhWXdIUVlEVlIwbEJCWXdGQVlJS3dZQkJRVUhBd0lHQ0NzR0FRVUZCd01CCk1BOEdBMVVkRXdFQi93UUZNQU1CQWY4d0tRWURWUjBPQkNJRUlMQ0ZrREpzNkJNNzQ1YlVMUUhPY3RscFRtbFIKTTR6ZGpDaHlicW11QVNqb01Bb0dDQ3FHU000OUJBTUNBMGNBTUVRQ0lIQllZTVplZ2V3Wk5BUU1iU3hoQUhZMApqZnNCdDJOOHVTZ3prNHRIUWMvQ0FpQklCSjVXK3AyVTRzYi9zWjhPeS9mZjBIdFBBekZCSWV4VERkUGNnNUxBCk1RPT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
                        "organizational_unit_identifier": "admin"
                      },
                      "client_ou_identifier": {
                        "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNQVENDQWVTZ0F3SUJBZ0lSQUxCOWVtTVhObkxaWW56TitkMHNJQ2N3Q2dZSUtvWkl6ajBFQXdJd2FURUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhGREFTQmdOVkJBb1RDMlY0WVcxd2JHVXVZMjl0TVJjd0ZRWURWUVFERXc1allTNWxlR0Z0CmNHeGxMbU52YlRBZUZ3MHhPVEV4TVRReE16UTRNREJhRncweU9URXhNVEV4TXpRNE1EQmFNR2t4Q3pBSkJnTlYKQkFZVEFsVlRNUk13RVFZRFZRUUlFd3BEWVd4cFptOXlibWxoTVJZd0ZBWURWUVFIRXcxVFlXNGdSbkpoYm1OcApjMk52TVJRd0VnWURWUVFLRXd0bGVHRnRjR3hsTG1OdmJURVhNQlVHQTFVRUF4TU9ZMkV1WlhoaGJYQnNaUzVqCmIyMHdXVEFUQmdjcWhrak9QUUlCQmdncWhrak9QUU1CQndOQ0FBVGVXYjlZWUdMNmgwMVlFckdzVS9xZmFzUmEKbUpHcFlWZGxsVTNwNldNUTMwZjcyazBFQitPRVQ3WWM3K0w2TWxxTTdNUDJBYnQ2RWUwV2w2OGpjZGxXbzIwdwphekFPQmdOVkhROEJBZjhFQkFNQ0FhWXdIUVlEVlIwbEJCWXdGQVlJS3dZQkJRVUhBd0lHQ0NzR0FRVUZCd01CCk1BOEdBMVVkRXdFQi93UUZNQU1CQWY4d0tRWURWUjBPQkNJRUlMQ0ZrREpzNkJNNzQ1YlVMUUhPY3RscFRtbFIKTTR6ZGpDaHlicW11QVNqb01Bb0dDQ3FHU000OUJBTUNBMGNBTUVRQ0lIQllZTVplZ2V3Wk5BUU1iU3hoQUhZMApqZnNCdDJOOHVTZ3prNHRIUWMvQ0FpQklCSjVXK3AyVTRzYi9zWjhPeS9mZjBIdFBBekZCSWV4VERkUGNnNUxBCk1RPT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
                        "organizational_unit_identifier": "client"
                      },
                      "enable": true,
                      "orderer_ou_identifier": {
                        "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNQVENDQWVTZ0F3SUJBZ0lSQUxCOWVtTVhObkxaWW56TitkMHNJQ2N3Q2dZSUtvWkl6ajBFQXdJd2FURUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhGREFTQmdOVkJBb1RDMlY0WVcxd2JHVXVZMjl0TVJjd0ZRWURWUVFERXc1allTNWxlR0Z0CmNHeGxMbU52YlRBZUZ3MHhPVEV4TVRReE16UTRNREJhRncweU9URXhNVEV4TXpRNE1EQmFNR2t4Q3pBSkJnTlYKQkFZVEFsVlRNUk13RVFZRFZRUUlFd3BEWVd4cFptOXlibWxoTVJZd0ZBWURWUVFIRXcxVFlXNGdSbkpoYm1OcApjMk52TVJRd0VnWURWUVFLRXd0bGVHRnRjR3hsTG1OdmJURVhNQlVHQTFVRUF4TU9ZMkV1WlhoaGJYQnNaUzVqCmIyMHdXVEFUQmdjcWhrak9QUUlCQmdncWhrak9QUU1CQndOQ0FBVGVXYjlZWUdMNmgwMVlFckdzVS9xZmFzUmEKbUpHcFlWZGxsVTNwNldNUTMwZjcyazBFQitPRVQ3WWM3K0w2TWxxTTdNUDJBYnQ2RWUwV2w2OGpjZGxXbzIwdwphekFPQmdOVkhROEJBZjhFQkFNQ0FhWXdIUVlEVlIwbEJCWXdGQVlJS3dZQkJRVUhBd0lHQ0NzR0FRVUZCd01CCk1BOEdBMVVkRXdFQi93UUZNQU1CQWY4d0tRWURWUjBPQkNJRUlMQ0ZrREpzNkJNNzQ1YlVMUUhPY3RscFRtbFIKTTR6ZGpDaHlicW11QVNqb01Bb0dDQ3FHU000OUJBTUNBMGNBTUVRQ0lIQllZTVplZ2V3Wk5BUU1iU3hoQUhZMApqZnNCdDJOOHVTZ3prNHRIUWMvQ0FpQklCSjVXK3AyVTRzYi9zWjhPeS9mZjBIdFBBekZCSWV4VERkUGNnNUxBCk1RPT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
                        "organizational_unit_identifier": "orderer"
                      },
                      "peer_ou_identifier": {
                        "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNQVENDQWVTZ0F3SUJBZ0lSQUxCOWVtTVhObkxaWW56TitkMHNJQ2N3Q2dZSUtvWkl6ajBFQXdJd2FURUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhGREFTQmdOVkJBb1RDMlY0WVcxd2JHVXVZMjl0TVJjd0ZRWURWUVFERXc1allTNWxlR0Z0CmNHeGxMbU52YlRBZUZ3MHhPVEV4TVRReE16UTRNREJhRncweU9URXhNVEV4TXpRNE1EQmFNR2t4Q3pBSkJnTlYKQkFZVEFsVlRNUk13RVFZRFZRUUlFd3BEWVd4cFptOXlibWxoTVJZd0ZBWURWUVFIRXcxVFlXNGdSbkpoYm1OcApjMk52TVJRd0VnWURWUVFLRXd0bGVHRnRjR3hsTG1OdmJURVhNQlVHQTFVRUF4TU9ZMkV1WlhoaGJYQnNaUzVqCmIyMHdXVEFUQmdjcWhrak9QUUlCQmdncWhrak9QUU1CQndOQ0FBVGVXYjlZWUdMNmgwMVlFckdzVS9xZmFzUmEKbUpHcFlWZGxsVTNwNldNUTMwZjcyazBFQitPRVQ3WWM3K0w2TWxxTTdNUDJBYnQ2RWUwV2w2OGpjZGxXbzIwdwphekFPQmdOVkhROEJBZjhFQkFNQ0FhWXdIUVlEVlIwbEJCWXdGQVlJS3dZQkJRVUhBd0lHQ0NzR0FRVUZCd01CCk1BOEdBMVVkRXdFQi93UUZNQU1CQWY4d0tRWURWUjBPQkNJRUlMQ0ZrREpzNkJNNzQ1YlVMUUhPY3RscFRtbFIKTTR6ZGpDaHlicW11QVNqb01Bb0dDQ3FHU000OUJBTUNBMGNBTUVRQ0lIQllZTVplZ2V3Wk5BUU1iU3hoQUhZMApqZnNCdDJOOHVTZ3prNHRIUWMvQ0FpQklCSjVXK3AyVTRzYi9zWjhPeS9mZjBIdFBBekZCSWV4VERkUGNnNUxBCk1RPT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
                        "organizational_unit_identifier": "peer"
                      }
                    },
                    "intermediate_certs": [],
                    "name": "OrdererMSP",
                    "organizational_unit_identifiers": [],
                    "revocation_list": [],
                    "root_certs": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNQVENDQWVTZ0F3SUJBZ0lSQUxCOWVtTVhObkxaWW56TitkMHNJQ2N3Q2dZSUtvWkl6ajBFQXdJd2FURUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhGREFTQmdOVkJBb1RDMlY0WVcxd2JHVXVZMjl0TVJjd0ZRWURWUVFERXc1allTNWxlR0Z0CmNHeGxMbU52YlRBZUZ3MHhPVEV4TVRReE16UTRNREJhRncweU9URXhNVEV4TXpRNE1EQmFNR2t4Q3pBSkJnTlYKQkFZVEFsVlRNUk13RVFZRFZRUUlFd3BEWVd4cFptOXlibWxoTVJZd0ZBWURWUVFIRXcxVFlXNGdSbkpoYm1OcApjMk52TVJRd0VnWURWUVFLRXd0bGVHRnRjR3hsTG1OdmJURVhNQlVHQTFVRUF4TU9ZMkV1WlhoaGJYQnNaUzVqCmIyMHdXVEFUQmdjcWhrak9QUUlCQmdncWhrak9QUU1CQndOQ0FBVGVXYjlZWUdMNmgwMVlFckdzVS9xZmFzUmEKbUpHcFlWZGxsVTNwNldNUTMwZjcyazBFQitPRVQ3WWM3K0w2TWxxTTdNUDJBYnQ2RWUwV2w2OGpjZGxXbzIwdwphekFPQmdOVkhROEJBZjhFQkFNQ0FhWXdIUVlEVlIwbEJCWXdGQVlJS3dZQkJRVUhBd0lHQ0NzR0FRVUZCd01CCk1BOEdBMVVkRXdFQi93UUZNQU1CQWY4d0tRWURWUjBPQkNJRUlMQ0ZrREpzNkJNNzQ1YlVMUUhPY3RscFRtbFIKTTR6ZGpDaHlicW11QVNqb01Bb0dDQ3FHU000OUJBTUNBMGNBTUVRQ0lIQllZTVplZ2V3Wk5BUU1iU3hoQUhZMApqZnNCdDJOOHVTZ3prNHRIUWMvQ0FpQklCSjVXK3AyVTRzYi9zWjhPeS9mZjBIdFBBekZCSWV4VERkUGNnNUxBCk1RPT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
                    ],
                    "signing_identity": null,
                    "tls_intermediate_certs": [],
                    "tls_root_certs": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNRekNDQWVtZ0F3SUJBZ0lRYmVteDhRa200VTNMT1EwWWJibzNmVEFLQmdncWhrak9QUVFEQWpCc01Rc3cKQ1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRUJ4TU5VMkZ1SUVaeQpZVzVqYVhOamJ6RVVNQklHQTFVRUNoTUxaWGhoYlhCc1pTNWpiMjB4R2pBWUJnTlZCQU1URVhSc2MyTmhMbVY0CllXMXdiR1V1WTI5dE1CNFhEVEU1TVRFeE5ERXpORGd3TUZvWERUSTVNVEV4TVRFek5EZ3dNRm93YkRFTE1Ba0cKQTFVRUJoTUNWVk14RXpBUkJnTlZCQWdUQ2tOaGJHbG1iM0p1YVdFeEZqQVVCZ05WQkFjVERWTmhiaUJHY21GdQpZMmx6WTI4eEZEQVNCZ05WQkFvVEMyVjRZVzF3YkdVdVkyOXRNUm93R0FZRFZRUURFeEYwYkhOallTNWxlR0Z0CmNHeGxMbU52YlRCWk1CTUdCeXFHU000OUFnRUdDQ3FHU000OUF3RUhBMElBQkR4aWpEY3FDMjlja0JOb21lam4KKzliaHVhc05yNlRMZjlIOStibTFPNTZJcko4ZmZ6WnEwaUJ4MWlpdmJSVGRDb2gwQ1d6ZUxRRHQyR3VBTkMrTgpBMDJqYlRCck1BNEdBMVVkRHdFQi93UUVBd0lCcGpBZEJnTlZIU1VFRmpBVUJnZ3JCZ0VGQlFjREFnWUlLd1lCCkJRVUhBd0V3RHdZRFZSMFRBUUgvQkFVd0F3RUIvekFwQmdOVkhRNEVJZ1Fnbm56SFBNRlFsSk50d1NqRXNBUUkKdnBFb3BvK2R4RElxbVB3bW0xczErT1V3Q2dZSUtvWkl6ajBFQXdJRFNBQXdSUUloQUtCTGhsSTlTcVkwTTZzTQozdC9DVG9sSXVEcXNmUVF0eWxhTkZpOTFYZjVZQWlCb1JCVWNCdy95SExTU01BSEwwcXBqVStUWHdDVzBzZ29FCmZ3NUNnU2s4MWc9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
                    ]
                  },
                  "type": 0
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
              "absolute_max_bytes": 103809024,
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
          "Capabilities": {
            "mod_policy": "Admins",
            "value": {
              "capabilities": {
                "V1_4_2": {}
              }
            },
            "version": "0"
          },
          "ChannelRestrictions": {
            "mod_policy": "Admins",
            "value": null,
            "version": "0"
          },
          "ConsensusType": {
            "mod_policy": "Admins",
            "value": {
              "metadata": null,
              "state": "STATE_NORMAL",
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
      "Capabilities": {
        "mod_policy": "Admins",
        "value": {
          "capabilities": {
            "V1_4_3": {}
          }
        },
        "version": "0"
      },
      "Consortium": {
        "mod_policy": "Admins",
        "value": {
          "name": "SampleConsortium"
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
            "orderer.example.com:7050"
          ]
        },
        "version": "0"
      }
    },
    "version": "0"
  },
  "sequence": "3"
}
```
</details>

A config might look intimidating in this form, but once you study it you’ll see that it has a logical structure.

For example, let's take a look at the config with a few of the tabs closed.

Note that this is the configuration of an application channel, not the orderer system channel.

![Sample config simplified](./images/sample_config.png)

The structure of the config should now be more obvious. You can see the config groupings: `Channel`, `Application`, and `Orderer`, and the configuration parameters related to each config grouping (we'll talk more about these in the next section), but also where the MSPs representing organizations are. Note that the `Channel` config grouping is below the `Orderer` group config values.

### More about these parameters

In this section, we'll take a deeper look at the configurable values in the context of where they sit in the configuration.

First, there are config parameters that occur in multiple parts of the configuration:

* **Policies**. Policies are not just a configuration value (which can be updated as defined in a `mod_policy`), they define the circumstances under which all parameters can be changed. For more information, check out [Policies](./policies/policies.html).

* **Capabilities**. Ensures that networks and channels process things in the same way, creating deterministic results for things like channel configuration updates and chaincode invocations. Without deterministic results, one peer on a channel might invalidate a transaction while another peer may validate it. For more information, check out [Capabilities](./capabilities_concept.html).

#### `Channel/Application`

Governs the configuration parameters unique to application channels (for example, adding or removing channel members). By default, changing these parameters requires the signature of a majority of the application organization admins.

* **Add orgs to a channel**. To add an organization to a channel, their MSP and other organization parameters must be generated and added here (under `Channel/Application/groups`).

* **Organization-related parameters**. Any parameters specific to an organization, (identifying an anchor peer, for example, or the certificates of org admins), can be changed. Note that changing these values will by default not require the majority of application organization admins but only an admin of the organization itself.

#### `Channel/Orderer`

Governs configuration parameters unique to the ordering service or the orderer system channel, requires a majority of the ordering organizations’ admins (by default there is only one ordering organization, though more can be added, for example when multiple organizations contribute nodes to the ordering service).

* **Batch size**. These parameters dictate the number and size of transactions in a block. No block will appear larger than `absolute_max_bytes` large or with more than `max_message_count` transactions inside the block. If it is possible to construct a block under `preferred_max_bytes`, then a block will be cut prematurely, and transactions larger than this size will appear in their own block.

* **Batch timeout**. The amount of time to wait after the first transaction arrives for additional transactions before cutting a block. Decreasing this value will improve latency, but decreasing it too much may decrease throughput by not allowing the block to fill to its maximum capacity.

* **Block validation**. This policy specifies the signature requirements for a block to be considered valid. By default, it requires a signature from some member of the ordering org.

* **Consensus type**. To enable the migration of Kafka based ordering services to Raft based ordering services, it is possible to change the consensus type of a channel. For more information, check out [Migrating from Kafka to Raft](./kafka_raft_migration.html).

* **Raft ordering service parameters**. For a look at the parameters unique to a Raft ordering service, check out [Raft configuration](./raft_configuration.html).

* **Kafka brokers** (where applicable). When `ConsensusType` is set to `kafka`, the `brokers` list enumerates some subset (or preferably all) of the Kafka brokers for the orderer to initially connect to at startup.

#### `Channel`

Governs configuration parameters that both the peer orgs and the ordering service orgs need to consent to, requires both the agreement of a majority of application organization admins and orderer organization admins.

* **Orderer addresses**. A list of addresses where clients may invoke the orderer `Broadcast` and `Deliver` functions. The peer randomly chooses among these addresses and fails over between them for retrieving blocks.

* **Hashing structure**. The block data is an array of byte arrays. The hash of the block data is computed as a Merkle tree. This value specifies the width of that Merkle tree. For the time being, this value is fixed to `4294967295` which corresponds to a simple flat hash of the concatenation of the block data bytes.

* **Hashing algorithm**. The algorithm used for computing the hash values encoded into the blocks of the blockchain. In particular, this affects the data hash, and the previous block hash fields of the block. Note, this field currently only has one valid value (`SHA256`) and should not be changed.

#### System channel configuration parameters

Certain configuration values are unique to the orderer system channel.

* **Channel creation policy.** Defines the policy value which will be set as the mod_policy for the Application group of new channels for the consortium it is defined in. The signature set attached to the channel creation request will be checked against the instantiation of this policy in the new channel to ensure that the channel creation is authorized. Note that this config value is only set in the orderer system channel.

* **Channel restrictions.** Only editable in the orderer system channel. The total number of channels the orderer is willing to allocate may be specified as `max_count`. This is primarily useful in pre-production environments with weak consortium `ChannelCreation` policies.

## Editing a config

Updating a channel configuration is a three step operation that's conceptually simple:

1. Get the latest channel config
2. Create a modified channel config
3. Create a config update transaction

However, as you'll see, this conceptual simplicity is wrapped in a somewhat convoluted process. As a result, some users might choose to script the process of pulling, translating, and scoping a config update. Users also have the option of how to modify the channel configuration itself, either manually or by using a tool like `jq`.

We have two tutorials that deal specifically with editing a channel configuration to achieve a specific end:

* [Adding an Org to a Channel](./channel_update_tutorial.html): shows the process for adding an additional organization to an existing channel.
* [Updating channel capabilities](./updating_a_channel.html): shows how to update channel capabilities.

In this topic, we'll show the process of editing a channel configuration independent of the end goal of the configuration update.

### Set environment variables for your config update

Before you attempt to use the sample commands, make sure to export the following environment variables, which will depend on the way you have structured your deployment. Note that the channel name, `CH_NAME` will have to be set for every channel being updated, as channel configuration updates only apply to the configuration of the channel being updated (with the exception of the ordering system channel, whose configuration is copied into the configuration of application channels by default).

* `CH_NAME`: the name of the channel being updated.
* `TLS_ROOT_CA`: the path to the root CA cert of the TLS CA of the organization proposing the update.
* `CORE_PEER_LOCALMSPID`: the name of your MSP.
* `CORE_PEER_MSPCONFIGPATH`: the absolute path to the MSP of your organization.
* `ORDERER_CONTAINER`: the name of an ordering node container. Note that when targeting the ordering service, you can target any active node in the ordering service. Your requests will be forwarded to the leader automatically.

Note: this topic will provide default names for the various JSON and protobuf files being pulled and modified (`config_block.pb`, `config_block.json`, etc). You are free to use whatever names you want. However, be aware that unless you go back and erase these files at the end of each config update, you will have to select different when making an additional update.

### Step 1: Pull and translate the config

The first step in updating a channel configuration is getting the latest config block. This is a three step process. First, we'll pull the channel configuration in protobuf format, creating a file called `config_block.pb`.

Make sure you are in the peer container.

Now issue:

```
peer channel fetch config config_block.pb -o $ORDERER_CONTAINER -c $CH_NAME --tls --cafile $TLS_ROOT_CA
```

Next, we'll covert the protobuf version of the channel config into a JSON version called `config_block.json` (JSON files are easier for humans to read and understand):

```
configtxlator proto_decode --input config_block.pb --type common.Block --output config_block.json
```

Finally, we'll scope out all of the unnecessary metadata from the config, which makes it easier to read. You are free to call this file whatever you want, but in this example we'll call it `config.json`.

```
jq .data.data[0].payload.data.config config_block.json > config.json
```

Now let's make a copy of `config.json` called `modified_config.json`. **Do not edit ``config.json`` directly**, as we will be using it to compute the difference between ``config.json`` and ``modified_config.json`` in a later step.

```
cp config.json modified_config.json
```

### Step 2: Modify the config

At this point, you have two options of how you want to modify the config.

1. Open ``modified_config.json`` using the text editor of your choice and make edits. Online tutorials exist that describe how to copy a file from a container that does not have an editor, edit it, and add it back to the container.
2. Use ``jq`` to apply edits to the config.

Whether you choose to edit the config manually or using `jq` depends on your use case. Because `jq` is concise and scriptable (an advantage when the same configuration update will be made to multiple channels), it's the recommend method for performing a channel update. For an example on how `jq` can be used, check out [Updating channel capabilities](./updating_a_channel.html#Create-a-capabilities-config-file), which shows multiple `jq` commands leveraging a capabilities config file called `capabilities.json`. If you are updating something other than the capabilities in your channel, you will have to modify your `jq` command and JSON file accordingly.

For more information about the content and structure of a channel configuration, check out our [sample channel config](#Sample-channel-configuration) above.

### Step 3: Re-encode and submit the config

Whether you make your config updates manually or using a tool like `jq`, you now have to run the process you ran to pull and scope the config in reverse, along with a step to calculate the difference between the old config and the new one, before submitting the config update to the other administrators on the channel to be approved.

First, we'll turn our `config.json` file back to protobuf format, creating a file called `config.pb`. Then we'll do the same with our `modified_config.json` file. Afterwards, we'll compute the difference between the two files, creating a file called `config_update.pb`.

```
configtxlator proto_encode --input config.json --type common.Config --output config.pb

configtxlator proto_encode --input modified_config.json --type common.Config --output modified_config.pb

configtxlator compute_update --channel_id $CH_NAME --original config.pb --updated modified_config.pb --output config_update.pb
```

Now that we have calculated the difference between the old config and the new one, we can apply the changes to the config.

```
configtxlator proto_decode --input config_update.pb --type common.ConfigUpdate --output config_update.json

echo '{"payload":{"header":{"channel_header":{"channel_id":"'$CH_NAME'", "type":2}},"data":{"config_update":'$(cat config_update.json)'}}}' | jq . > config_update_in_envelope.json

configtxlator proto_encode --input config_update_in_envelope.json --type common.Envelope --output config_update_in_envelope.pb
```

Submit the config update transaction:

```
peer channel update -f config_update_in_envelope.pb -c $CH_NAME -o $ORDERER_CONTAINER --tls true --cafile $TLS_ROOT_CA
```

Our config update transaction represents the difference between the original config and the modified one, but the ordering service will translate this into a full channel config.

## Get the Necessary Signatures

Once you’ve successfully generated the new configuration protobuf file, it will need to satisfy the relevant policy for whatever it is you’re trying to change, typically (though not always) by requiring signatures from other organizations.

*Note: you may be able to script the signature collection, dependent on your application. In general, you may always collect more signatures than are required.*

The actual process of getting these signatures will depend on how you’ve set up your system, but there are two main implementations. Currently, the Fabric command line defaults to a “pass it along” system. That is, the Admin of the Org proposing a config update sends the update to someone else (another Admin, typically) who needs to sign it. This Admin signs it (or doesn’t) and passes it along to the next Admin, and so on, until there are enough signatures for the config to be submitted.

This has the virtue of simplicity --- when there are enough signatures, the last admin can simply submit the config transaction (in Fabric, the `peer channel update` command includes a signature by default). However, this process will only be practical in smaller channels, since the “pass it along” method can be time consuming.

The other option is to submit the update to every Admin on a channel and wait for enough signatures to come back. These signatures can then be stitched together and submitted. This makes life a bit more difficult for the Admin who created the config update (forcing them to deal with a file per signer) but is the recommended workflow for users which are developing Fabric management applications.

Once the config has been added to the ledger, it will be a best practice to pull it and convert it to JSON to check to make sure everything was added correctly. This will also serve as a useful copy of the latest config.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
