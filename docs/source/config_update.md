# Updating a Channel Configuration

## What is a Channel Configuration?

Channel configurations contain all of the information relevant to the
administration of a channel. Most importantly, the channel configuration
specifies which organizations are members of channel, but it also includes other
channel-wide configuration information such as channel access policies and block
batch sizes.

This configuration is stored on the ledger in a **block**, and is therefore
known as a configuration (config) block. Configuration blocks contain a single
configuration. The first of these blocks is known as the "genesis block" and
contains the initial configuration required to bootstrap a channel. Each time
the configuration of a channel changes it is done through a new configuration
block, with the latest configuration block representing the current channel
configuration. Orderers and peers keep the current channel configuration in
memory to facilitate all channel operations such as cutting a new block and
validating block transactions.

Because configurations are stored in blocks, updating a config happens through a
process called a "configuration transaction" (even though the process is a
little different from a normal transaction). Updating a config is a process of
pulling the config, translating into a format that humans can read, modifying it
and then submitting it for approval.

For a more in-depth look at the process for pulling a config and translating it
into JSON, check out [Adding an Org to a Channel](./channel_update_tutorial.html).
In this doc, we'll be focusing on the different ways you can edit a config and
the process for getting it signed.

## Editing a Config

Channels are highly configurable, but not infinitely so. Different configuration
elements have different modification policies (which specify the group of
identities required to sign the config update).

To see the scope of what's possible to change it's important to look at a config
in JSON format. The [Adding an Org to a Channel](./channel_update_tutorial.html)
tutorial generates one, so if you've gone through that doc you can simply refer to it.
For those who have not, we'll provide one here (for ease of readability, it might be
helpful to put this config into a viewer that supports JSON folding, like atom or
Visual Studio).

<details>
  <summary>
    **Click here to see the config**
  </summary>
  ```
  {
  "channel_group": {
    "groups": {
      "Application": {
        "groups": {
          "Org1MSP": {
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
                          "msp_identifier": "Org1MSP",
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
                    "admins": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNHRENDQWIrZ0F3SUJBZ0lRSWlyVmg3NVcwWmh0UjEzdmltdmliakFLQmdncWhrak9QUVFEQWpCek1Rc3cKQ1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRUJ4TU5VMkZ1SUVaeQpZVzVqYVhOamJ6RVpNQmNHQTFVRUNoTVFiM0puTVM1bGVHRnRjR3hsTG1OdmJURWNNQm9HQTFVRUF4TVRZMkV1CmIzSm5NUzVsZUdGdGNHeGxMbU52YlRBZUZ3MHhOekV4TWpreE9USTBNRFphRncweU56RXhNamN4T1RJME1EWmEKTUZzeEN6QUpCZ05WQkFZVEFsVlRNUk13RVFZRFZRUUlFd3BEWVd4cFptOXlibWxoTVJZd0ZBWURWUVFIRXcxVApZVzRnUm5KaGJtTnBjMk52TVI4d0hRWURWUVFEREJaQlpHMXBia0J2Y21jeExtVjRZVzF3YkdVdVkyOXRNRmt3CkV3WUhLb1pJemowQ0FRWUlLb1pJemowREFRY0RRZ0FFNkdVeDlpczZ0aG1ZRE9tMmVHSlA5eW1yaXJYWE1Cd0oKQmVWb1Vpak5haUdsWE03N2NsSE5aZjArMGFjK2djRU5lMzQweGExZVFnb2Q0YjVFcmQrNmtxTk5NRXN3RGdZRApWUjBQQVFIL0JBUURBZ2VBTUF3R0ExVWRFd0VCL3dRQ01BQXdLd1lEVlIwakJDUXdJb0FnWWdoR2xCMjBGWmZCCllQemdYT280czdkU1k1V3NKSkRZbGszTDJvOXZzQ013Q2dZSUtvWkl6ajBFQXdJRFJ3QXdSQUlnYmlEWDVTMlIKRTBNWGRobDZFbmpVNm1lTEJ0eXNMR2ZpZXZWTlNmWW1UQVVDSUdVbnROangrVXZEYkZPRHZFcFRVTm5MUHp0Qwp5ZlBnOEhMdWpMaXVpaWFaCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
                    ],
                    "crypto_config": {
                      "identity_identifier_hash_function": "SHA256",
                      "signature_hash_family": "SHA2"
                    },
                    "name": "Org1MSP",
                    "root_certs": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNRekNDQWVxZ0F3SUJBZ0lSQU03ZVdTaVM4V3VVM2haMU9tR255eXd3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpFdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekV1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGN4TVRJNU1Ua3lOREEyV2hjTk1qY3hNVEkzTVRreU5EQTIKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NUzVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1TNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQkJiTTVZS3B6UmlEbDdLWWFpSDVsVnBIeEl1TDEyaUcyWGhkMHRpbEg3MEljMGFpRUh1dG9rTkZsUXAzTWI0Zgpvb0M2bFVXWnRnRDJwMzZFNThMYkdqK2pYekJkTUE0R0ExVWREd0VCL3dRRUF3SUJwakFQQmdOVkhTVUVDREFHCkJnUlZIU1VBTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3S1FZRFZSME9CQ0lFSUdJSVJwUWR0QldYd1dEODRGenEKT0xPM1VtT1ZyQ1NRMkpaTnk5cVBiN0FqTUFvR0NDcUdTTTQ5QkFNQ0EwY0FNRVFDSUdlS2VZL1BsdGlWQTRPSgpRTWdwcDRvaGRMcGxKUFpzNERYS0NuOE9BZG9YQWlCK2g5TFdsR3ZsSDdtNkVpMXVRcDFld2ZESmxsZi9MZXczClgxaDNRY0VMZ3c9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
                    ],
                    "tls_root_certs": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNTVENDQWZDZ0F3SUJBZ0lSQUtsNEFQWmV6dWt0Nk8wYjRyYjY5Y0F3Q2dZSUtvWkl6ajBFQXdJd2RqRUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpFdVpYaGhiWEJzWlM1amIyMHhIekFkQmdOVkJBTVRGblJzCmMyTmhMbTl5WnpFdVpYaGhiWEJzWlM1amIyMHdIaGNOTVRjeE1USTVNVGt5TkRBMldoY05NamN4TVRJM01Ua3kKTkRBMldqQjJNUXN3Q1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRQpCeE1OVTJGdUlFWnlZVzVqYVhOamJ6RVpNQmNHQTFVRUNoTVFiM0puTVM1bGVHRnRjR3hsTG1OdmJURWZNQjBHCkExVUVBeE1XZEd4elkyRXViM0puTVM1bGVHRnRjR3hsTG1OdmJUQlpNQk1HQnlxR1NNNDlBZ0VHQ0NxR1NNNDkKQXdFSEEwSUFCSnNpQXVjYlcrM0lqQ2VaaXZPakRiUmFyVlRjTW9TRS9mSnQyU0thR1d5bWQ0am5xM25MWC9vVApCVmpZb21wUG1QbGZ4R0VSWHl0UTNvOVZBL2hwNHBlalh6QmRNQTRHQTFVZER3RUIvd1FFQXdJQnBqQVBCZ05WCkhTVUVDREFHQmdSVkhTVUFNQThHQTFVZEV3RUIvd1FGTUFNQkFmOHdLUVlEVlIwT0JDSUVJSnlqZnFoa0FvY3oKdkRpNnNGSGFZL1Bvd2tPWkxPMHZ0VGdFRnVDbUpFalZNQW9HQ0NxR1NNNDlCQU1DQTBjQU1FUUNJRjVOVVdCVgpmSjgrM0lxU3J1NlFFbjlIa0lsQ0xDMnlvWTlaNHBWMnpBeFNBaUE5NWQzeDhBRXZIcUFNZnIxcXBOWHZ1TW5BCmQzUXBFa1gyWkh3ODZlQlVQZz09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
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
                          "msp_identifier": "Org2MSP",
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
                    "admins": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNHVENDQWNDZ0F3SUJBZ0lSQU5Pb1lIbk9seU94dTJxZFBteStyV293Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpJdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekl1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGN4TVRJNU1Ua3lOREEyV2hjTk1qY3hNVEkzTVRreU5EQTIKV2pCYk1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFZk1CMEdBMVVFQXd3V1FXUnRhVzVBYjNKbk1pNWxlR0Z0Y0d4bExtTnZiVEJaCk1CTUdCeXFHU000OUFnRUdDQ3FHU000OUF3RUhBMElBQkh1M0ZWMGlqdFFzckpsbnBCblgyRy9ickFjTHFJSzgKVDFiSWFyZlpvSkhtQm5IVW11RTBhc1dyKzM4VUs0N3hyczNZMGMycGhFVjIvRnhHbHhXMUZubWpUVEJMTUE0RwpBMVVkRHdFQi93UUVBd0lIZ0RBTUJnTlZIUk1CQWY4RUFqQUFNQ3NHQTFVZEl3UWtNQ0tBSU1pSzdteFpnQVVmCmdrN0RPTklXd2F4YktHVGdLSnVSNjZqVmordHZEV3RUTUFvR0NDcUdTTTQ5QkFNQ0EwY0FNRVFDSUQxaEtRdk8KVWxyWmVZMmZZY1N2YWExQmJPM3BVb3NxL2tZVElyaVdVM1J3QWlBR29mWmVPUFByWXVlTlk0Z2JCV2tjc3lpZgpNMkJmeXQwWG9NUThyT2VidUE9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
                    ],
                    "crypto_config": {
                      "identity_identifier_hash_function": "SHA256",
                      "signature_hash_family": "SHA2"
                    },
                    "name": "Org2MSP",
                    "root_certs": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNSRENDQWVxZ0F3SUJBZ0lSQU1pVXk5SGRSbXB5MDdsSjhRMlZNWXN3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpJdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekl1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGN4TVRJNU1Ua3lOREEyV2hjTk1qY3hNVEkzTVRreU5EQTIKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NaTVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1pNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQk50YW1PY1hyaGwrQ2hzYXNSeklNWjV3OHpPWVhGcXhQbGV0a3d5UHJrbHpKWE01Qjl4QkRRVWlWNldJS2tGSwo0Vmd5RlNVWGZqaGdtd25kMUNBVkJXaWpYekJkTUE0R0ExVWREd0VCL3dRRUF3SUJwakFQQmdOVkhTVUVDREFHCkJnUlZIU1VBTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3S1FZRFZSME9CQ0lFSU1pSzdteFpnQVVmZ2s3RE9OSVcKd2F4YktHVGdLSnVSNjZqVmordHZEV3RUTUFvR0NDcUdTTTQ5QkFNQ0EwZ0FNRVVDSVFEQ3FFRmFqeU5IQmVaRworOUdWVkNFNWI1YTF5ZlhvS3lkemdLMVgyOTl4ZmdJZ05BSUUvM3JINHFsUE9HbjdSS3Yram9WaUNHS2t6L0F1Cm9FNzI4RWR6WmdRPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
                    ],
                    "tls_root_certs": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNTakNDQWZDZ0F3SUJBZ0lSQU9JNmRWUWMraHBZdkdMSlFQM1YwQU13Q2dZSUtvWkl6ajBFQXdJd2RqRUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpJdVpYaGhiWEJzWlM1amIyMHhIekFkQmdOVkJBTVRGblJzCmMyTmhMbTl5WnpJdVpYaGhiWEJzWlM1amIyMHdIaGNOTVRjeE1USTVNVGt5TkRBMldoY05NamN4TVRJM01Ua3kKTkRBMldqQjJNUXN3Q1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRQpCeE1OVTJGdUlFWnlZVzVqYVhOamJ6RVpNQmNHQTFVRUNoTVFiM0puTWk1bGVHRnRjR3hsTG1OdmJURWZNQjBHCkExVUVBeE1XZEd4elkyRXViM0puTWk1bGVHRnRjR3hsTG1OdmJUQlpNQk1HQnlxR1NNNDlBZ0VHQ0NxR1NNNDkKQXdFSEEwSUFCTWZ1QTMwQVVBT1ZKRG1qVlBZd1lNbTlweW92MFN6OHY4SUQ5N0twSHhXOHVOOUdSOU84aVdFMgo5bllWWVpiZFB2V1h1RCszblpweUFNcGZja3YvYUV5alh6QmRNQTRHQTFVZER3RUIvd1FFQXdJQnBqQVBCZ05WCkhTVUVDREFHQmdSVkhTVUFNQThHQTFVZEV3RUIvd1FGTUFNQkFmOHdLUVlEVlIwT0JDSUVJRnk5VHBHcStQL08KUGRXbkZXdWRPTnFqVDRxOEVKcDJmbERnVCtFV2RnRnFNQW9HQ0NxR1NNNDlCQU1DQTBnQU1FVUNJUUNZYlhSeApXWDZoUitPU0xBNSs4bFRwcXRMWnNhOHVuS3J3ek1UYXlQUXNVd0lnVSs5YXdaaE0xRzg3bGE0V0h4cmt5eVZ2CkU4S1ZsR09IVHVPWm9TMU5PT0U9Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
                    ]
                  },
                  "type": 0
                },
                "version": "0"
              }
            },
            "version": "1"
          },
          "Org3MSP": {
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
                          "msp_identifier": "Org3MSP",
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
                          "msp_identifier": "Org3MSP",
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
                          "msp_identifier": "Org3MSP",
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
                    "admins": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNHRENDQWIrZ0F3SUJBZ0lRQUlSNWN4U0hpVm1kSm9uY3FJVUxXekFLQmdncWhrak9QUVFEQWpCek1Rc3cKQ1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRUJ4TU5VMkZ1SUVaeQpZVzVqYVhOamJ6RVpNQmNHQTFVRUNoTVFiM0puTXk1bGVHRnRjR3hsTG1OdmJURWNNQm9HQTFVRUF4TVRZMkV1CmIzSm5NeTVsZUdGdGNHeGxMbU52YlRBZUZ3MHhOekV4TWpreE9UTTRNekJhRncweU56RXhNamN4T1RNNE16QmEKTUZzeEN6QUpCZ05WQkFZVEFsVlRNUk13RVFZRFZRUUlFd3BEWVd4cFptOXlibWxoTVJZd0ZBWURWUVFIRXcxVApZVzRnUm5KaGJtTnBjMk52TVI4d0hRWURWUVFEREJaQlpHMXBia0J2Y21jekxtVjRZVzF3YkdVdVkyOXRNRmt3CkV3WUhLb1pJemowQ0FRWUlLb1pJemowREFRY0RRZ0FFSFlkVFY2ZC80cmR4WFd2cm1qZ0hIQlhXc2lxUWxrcnQKZ0p1NzMxcG0yZDRrWU82aEd2b2tFRFBwbkZFdFBwdkw3K1F1UjhYdkFQM0tqTkt0NHdMRG5hTk5NRXN3RGdZRApWUjBQQVFIL0JBUURBZ2VBTUF3R0ExVWRFd0VCL3dRQ01BQXdLd1lEVlIwakJDUXdJb0FnSWNxUFVhM1VQNmN0Ck9LZmYvKzVpMWJZVUZFeVFlMVAyU0hBRldWSWUxYzB3Q2dZSUtvWkl6ajBFQXdJRFJ3QXdSQUlnUm5LRnhsTlYKSmppVGpkZmVoczRwNy9qMkt3bFVuUWVuNFkyUnV6QjFrbm9DSUd3dEZ1TEdpRFY2THZSL2pHVXR3UkNyeGw5ZApVNENCeDhGbjBMdXNMTkJYCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
                    ],
                    "crypto_config": {
                      "identity_identifier_hash_function": "SHA256",
                      "signature_hash_family": "SHA2"
                    },
                    "name": "Org3MSP",
                    "root_certs": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNRakNDQWVtZ0F3SUJBZ0lRUkN1U2Y0RVJNaDdHQW1ydTFIQ2FZREFLQmdncWhrak9QUVFEQWpCek1Rc3cKQ1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRUJ4TU5VMkZ1SUVaeQpZVzVqYVhOamJ6RVpNQmNHQTFVRUNoTVFiM0puTXk1bGVHRnRjR3hsTG1OdmJURWNNQm9HQTFVRUF4TVRZMkV1CmIzSm5NeTVsZUdGdGNHeGxMbU52YlRBZUZ3MHhOekV4TWpreE9UTTRNekJhRncweU56RXhNamN4T1RNNE16QmEKTUhNeEN6QUpCZ05WQkFZVEFsVlRNUk13RVFZRFZRUUlFd3BEWVd4cFptOXlibWxoTVJZd0ZBWURWUVFIRXcxVApZVzRnUm5KaGJtTnBjMk52TVJrd0Z3WURWUVFLRXhCdmNtY3pMbVY0WVcxd2JHVXVZMjl0TVJ3d0dnWURWUVFECkV4TmpZUzV2Y21jekxtVjRZVzF3YkdVdVkyOXRNRmt3RXdZSEtvWkl6ajBDQVFZSUtvWkl6ajBEQVFjRFFnQUUKZXFxOFFQMnllM08vM1J3UzI0SWdtRVdST3RnK3Zyc2pRY1BvTU42NEZiUGJKbmExMklNaVdDUTF6ZEZiTU9hSAorMUlrb21yY0RDL1ZpejkvY0M0NW9xTmZNRjB3RGdZRFZSMFBBUUgvQkFRREFnR21NQThHQTFVZEpRUUlNQVlHCkJGVWRKUUF3RHdZRFZSMFRBUUgvQkFVd0F3RUIvekFwQmdOVkhRNEVJZ1FnSWNxUFVhM1VQNmN0T0tmZi8rNWkKMWJZVUZFeVFlMVAyU0hBRldWSWUxYzB3Q2dZSUtvWkl6ajBFQXdJRFJ3QXdSQUlnTEgxL2xSZElWTVA4Z2FWeQpKRW01QWQ0SjhwZ256N1BVV2JIMzZvdVg4K1lDSUNPK20vUG9DbDRIbTlFbXhFN3ZnUHlOY2trVWd0SlRiTFhqCk5SWjBxNTdWCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
                    ],
                    "tls_root_certs": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNTVENDQWZDZ0F3SUJBZ0lSQU9xc2JQQzFOVHJzclEvUUNpalh6K0F3Q2dZSUtvWkl6ajBFQXdJd2RqRUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpNdVpYaGhiWEJzWlM1amIyMHhIekFkQmdOVkJBTVRGblJzCmMyTmhMbTl5WnpNdVpYaGhiWEJzWlM1amIyMHdIaGNOTVRjeE1USTVNVGt6T0RNd1doY05NamN4TVRJM01Ua3oKT0RNd1dqQjJNUXN3Q1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRQpCeE1OVTJGdUlFWnlZVzVqYVhOamJ6RVpNQmNHQTFVRUNoTVFiM0puTXk1bGVHRnRjR3hsTG1OdmJURWZNQjBHCkExVUVBeE1XZEd4elkyRXViM0puTXk1bGVHRnRjR3hsTG1OdmJUQlpNQk1HQnlxR1NNNDlBZ0VHQ0NxR1NNNDkKQXdFSEEwSUFCSVJTTHdDejdyWENiY0VLMmhxSnhBVm9DaDhkejNqcnA5RHMyYW9TQjBVNTZkSUZhVmZoR2FsKwovdGp6YXlndXpFalFhNlJ1MmhQVnRGM2NvQnJ2Ulpxalh6QmRNQTRHQTFVZER3RUIvd1FFQXdJQnBqQVBCZ05WCkhTVUVDREFHQmdSVkhTVUFNQThHQTFVZEV3RUIvd1FGTUFNQkFmOHdLUVlEVlIwT0JDSUVJQ2FkVERGa0JPTGkKblcrN2xCbDExL3pPbXk4a1BlYXc0MVNZWEF6cVhnZEVNQW9HQ0NxR1NNNDlCQU1DQTBjQU1FUUNJQlgyMWR3UwpGaG5NdDhHWXUweEgrUGd5aXQreFdQUjBuTE1Jc1p2dVlRaktBaUFLUlE5N2VrLzRDTzZPWUtSakR0VFM4UFRmCm9nTmJ6dTBxcThjbVhseW5jZz09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
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
        "version": "1"
      },
      "Orderer": {
        "groups": {
          "OrdererOrg": {
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
                    "admins": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNDakNDQWJDZ0F3SUJBZ0lRSFNTTnIyMWRLTTB6THZ0dEdoQnpMVEFLQmdncWhrak9QUVFEQWpCcE1Rc3cKQ1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRUJ4TU5VMkZ1SUVaeQpZVzVqYVhOamJ6RVVNQklHQTFVRUNoTUxaWGhoYlhCc1pTNWpiMjB4RnpBVkJnTlZCQU1URG1OaExtVjRZVzF3CmJHVXVZMjl0TUI0WERURTNNVEV5T1RFNU1qUXdObG9YRFRJM01URXlOekU1TWpRd05sb3dWakVMTUFrR0ExVUUKQmhNQ1ZWTXhFekFSQmdOVkJBZ1RDa05oYkdsbWIzSnVhV0V4RmpBVUJnTlZCQWNURFZOaGJpQkdjbUZ1WTJsegpZMjh4R2pBWUJnTlZCQU1NRVVGa2JXbHVRR1Y0WVcxd2JHVXVZMjl0TUZrd0V3WUhLb1pJemowQ0FRWUlLb1pJCnpqMERBUWNEUWdBRTZCTVcvY0RGUkUvakFSenV5N1BjeFQ5a3pnZitudXdwKzhzK2xia0hZd0ZpaForMWRhR3gKKzhpS1hDY0YrZ0tpcVBEQXBpZ2REOXNSeTBoTEMwQnRacU5OTUVzd0RnWURWUjBQQVFIL0JBUURBZ2VBTUF3RwpBMVVkRXdFQi93UUNNQUF3S3dZRFZSMGpCQ1F3SW9BZ3o3bDQ2ZXRrODU0NFJEanZENVB6YjV3TzI5N0lIMnNUCngwTjAzOHZibkpzd0NnWUlLb1pJemowRUF3SURTQUF3UlFJaEFNRTJPWXljSnVyYzhVY2hkeTA5RU50RTNFUDIKcVoxSnFTOWVCK0gxSG5FSkFpQUtXa2h5TmI0akRPS2MramJIVmgwV0YrZ3J4UlJYT1hGaEl4ei85elI3UUE9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
                    ],
                    "crypto_config": {
                      "identity_identifier_hash_function": "SHA256",
                      "signature_hash_family": "SHA2"
                    },
                    "name": "OrdererMSP",
                    "root_certs": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNMakNDQWRXZ0F3SUJBZ0lRY2cxUVZkVmU2Skd6YVU1cmxjcW4vakFLQmdncWhrak9QUVFEQWpCcE1Rc3cKQ1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRUJ4TU5VMkZ1SUVaeQpZVzVqYVhOamJ6RVVNQklHQTFVRUNoTUxaWGhoYlhCc1pTNWpiMjB4RnpBVkJnTlZCQU1URG1OaExtVjRZVzF3CmJHVXVZMjl0TUI0WERURTNNVEV5T1RFNU1qUXdObG9YRFRJM01URXlOekU1TWpRd05sb3dhVEVMTUFrR0ExVUUKQmhNQ1ZWTXhFekFSQmdOVkJBZ1RDa05oYkdsbWIzSnVhV0V4RmpBVUJnTlZCQWNURFZOaGJpQkdjbUZ1WTJsegpZMjh4RkRBU0JnTlZCQW9UQzJWNFlXMXdiR1V1WTI5dE1SY3dGUVlEVlFRREV3NWpZUzVsZUdGdGNHeGxMbU52CmJUQlpNQk1HQnlxR1NNNDlBZ0VHQ0NxR1NNNDlBd0VIQTBJQUJQTVI2MGdCcVJham9hS0U1TExRYjRIb28wN3QKYTRuM21Ncy9NRGloQVQ5YUN4UGZBcDM5SS8wMmwvZ2xiMTdCcEtxZGpGd0JKZHNuMVN6ZnQ3NlZkTitqWHpCZApNQTRHQTFVZER3RUIvd1FFQXdJQnBqQVBCZ05WSFNVRUNEQUdCZ1JWSFNVQU1BOEdBMVVkRXdFQi93UUZNQU1CCkFmOHdLUVlEVlIwT0JDSUVJTSs1ZU9uclpQT2VPRVE0N3crVDgyK2NEdHZleUI5ckU4ZERkTi9MMjV5Yk1Bb0cKQ0NxR1NNNDlCQU1DQTBjQU1FUUNJQVB6SGNOUmQ2a3QxSEdpWEFDclFTM0grL3R5NmcvVFpJa1pTeXIybmdLNQpBaUJnb1BVTTEwTHNsMVFtb2dlbFBjblZGZjJoODBXR2I3NGRIS2tzVFJKUkx3PT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
                    ],
                    "tls_root_certs": [
                      "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNORENDQWR1Z0F3SUJBZ0lRYWJ5SUl6cldtUFNzSjJacisvRVpXVEFLQmdncWhrak9QUVFEQWpCc01Rc3cKQ1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRUJ4TU5VMkZ1SUVaeQpZVzVqYVhOamJ6RVVNQklHQTFVRUNoTUxaWGhoYlhCc1pTNWpiMjB4R2pBWUJnTlZCQU1URVhSc2MyTmhMbVY0CllXMXdiR1V1WTI5dE1CNFhEVEUzTVRFeU9URTVNalF3TmxvWERUSTNNVEV5TnpFNU1qUXdObG93YkRFTE1Ba0cKQTFVRUJoTUNWVk14RXpBUkJnTlZCQWdUQ2tOaGJHbG1iM0p1YVdFeEZqQVVCZ05WQkFjVERWTmhiaUJHY21GdQpZMmx6WTI4eEZEQVNCZ05WQkFvVEMyVjRZVzF3YkdVdVkyOXRNUm93R0FZRFZRUURFeEYwYkhOallTNWxlR0Z0CmNHeGxMbU52YlRCWk1CTUdCeXFHU000OUFnRUdDQ3FHU000OUF3RUhBMElBQkVZVE9mdG1rTHdiSlRNeG1aVzMKZVdqRUQ2eW1UeEhYeWFQdTM2Y1NQWDlldDZyU3Y5UFpCTGxyK3hZN1dtYlhyOHM5K3E1RDMwWHl6OEh1OWthMQpSc1dqWHpCZE1BNEdBMVVkRHdFQi93UUVBd0lCcGpBUEJnTlZIU1VFQ0RBR0JnUlZIU1VBTUE4R0ExVWRFd0VCCi93UUZNQU1CQWY4d0tRWURWUjBPQkNJRUlJcjduNTVjTWlUdENEYmM5UGU0RFpnZ0ZYdHV2RktTdnBNYUhzbzAKSnpFd01Bb0dDQ3FHU000OUJBTUNBMGNBTUVRQ0lGM1gvMGtQRkFVQzV2N25JVVh6SmI5Z3JscWxET05UeVg2QQpvcmtFVTdWb0FpQkpMbS9IUFZ0aVRHY2NldUZPZTE4SnNwd0JTZ1hxNnY1K1BobEdsbU9pWHc9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
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
    "mod_policy": "",
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
  "sequence": "3",
  "type": 0
}
```
</details>

A config might look intimidating in this form, but once you study it you'll see
that it has a logical structure.

Beyond the definitions of the policies -- defining who can do certain things
at the channel level, and who has the permission to change who can change the
config -- channels also have other kinds of features that can be modified using
a config update. [Adding an Org to a Channel](./channel_update_tutorial.html)
takes you through one of the most important -- adding an org to a channel. Some
other things that are possible to change with a config update include:

* **Batch Size.** These parameters dictate the number and size of transactions
in a block. No block will appear larger than `absolute_max_bytes` large or
with more than `max_message_count` transactions inside the block. If it is
possible to construct a block under `preferred_max_bytes`, then a block will
be cut prematurely, and transactions larger than this size will appear in
their own block.

   ```
   {
     "absolute_max_bytes": 102760448,
     "max_message_count": 10,
     "preferred_max_bytes": 524288
   }
  ```

* **Batch Timeout.** The amount of time to wait after the first transaction
arrives for additional transactions before cutting a block. Decreasing this
value will improve latency, but decreasing it too much may decrease throughput
by not allowing the block to fill to its maximum capacity.

  ```
  { "timeout": "2s" }
  ```

* **Channel Restrictions.** The total number of channels the orderer is willing
to allocate may be specified as max_count. This is primarily useful in
pre-production environments with weak consortium `ChannelCreation` policies.

  ```
  {
   "max_count":1000
  }
  ```

* **Channel Creation Policy.** Defines the policy value which will be set as the
mod_policy for the Application group of new channels for the consortium it is defined in.
The signature set attached to the channel creation request will be checked against
the instantiation of this policy in the new channel to ensure that the channel
creation is authorized. Note that this config value is only set in the orderer
system channel.

  ```
  {
  "type": 3,
  "value": {
    "rule": "ANY",
    "sub_policy": "Admins"
    }
  }
  ```

* **Kafka brokers.** When `ConsensusType` is set to `kafka`, the `brokers` list
enumerates some subset (or preferably all) of the Kafka brokers for the
orderer to initially connect to at startup. *Note that it is not possible to
change your consensus type after it has been established (during the
bootstrapping of the genesis block)*.

  ```
  {
    "brokers": [
      "kafka0:9092",
      "kafka1:9092",
      "kafka2:9092",
      "kafka3:9092"
    ]
  }
  ```

* **Anchor Peers Definition.** Defines the location of the anchor peers for
each Org.

  ```
  {
    "host": "peer0.org2.example.com",
      "port": 9051
  }
  ```

* **Hashing Structure.** The block data is an array of byte arrays. The hash of
the block data is computed as a Merkle tree. This value specifies the width of
that Merkle tree. For the time being, this value is fixed to `4294967295`
which corresponds to a simple flat hash of the concatenation of the block data
bytes.

  ```
  { "width": 4294967295 }
  ```

* **Hashing Algorithm.** The algorithm used for computing the hash values
encoded into the blocks of the blockchain. In particular, this affects the
data hash, and the previous block hash fields of the block. Note, this field
currently only has one valid value (`SHA256`) and should not be changed.

  ```
  { "name": "SHA256" }
  ```

* **Block Validation.** This policy specifies the signature requirements for a
block to be considered valid. By default, it requires a signature from some
member of the ordering org.

  ```
  {
    "type": 3,
    "value": {
      "rule": "ANY",
      "sub_policy": "Writers"
    }
  }
  ```

* **Orderer Address.** A list of addresses where clients may invoke the orderer
`Broadcast` and `Deliver` functions. The peer randomly chooses among these
addresses and fails over between them for retrieving blocks.

  ```
  {
    "addresses": [
      "orderer.example.com:7050"
    ]
  }
  ```

Just as we add an Org by adding their artifacts and MSP information, you can remove
them by reversing the process.

**Note** that once the consensus type has been defined and the network has been
bootstrapped, it is not possible to change it through a configuration update.

There is another important channel configuration (especially for v1.1) known as
**Capability Requirements**. It has its own doc that can be found
[here](./capability_requirements.html).

Let's say you want to edit the block batch size for the channel (because this is
a single numeric field, it's one of the easiest changes to make). First to make
referencing the JSON path easy, we define it as an environment variable.

To establish this, take a look at your config, find what you're looking for, and
back track the path.

If you find batch size, for example, you'll see that it's a `value` of the
`Orderer`. `Orderer` can be found under `groups`, which is under
`channel_group`. The batch size value has a parameter under `value` of
`max_message_count`.

Which would make the path this:

```
 export MAXBATCHSIZEPATH=".channel_group.groups.Orderer.values.BatchSize.value.max_message_count"
```

Next, display the value of that property:

```
jq "$MAXBATCHSIZEPATH" config.json
```

Which should return a value of `10` (in our sample network at least).

Now, let's set the new batch size and display the new value:

```
 jq "$MAXBATCHSIZEPATH = 20" config.json > modified_config.json
 jq "$MAXBATCHSIZEPATH" modified_config.json
```

Once you've modified the JSON, it's ready to be converted and submitted. The
scripts and steps in [Adding an Org to a Channel](./channel_update_tutorial.html)
will take you through the process for converting the JSON, so let's look at the
process of submitting it.

## Get the Necessary Signatures

Once you've successfully generated the protobuf file, it's time to get it
signed. To do this, you need to know the relevant policy for whatever it is you're
trying to change.

By default, editing the configuration of:
* **A particular org** (for example, changing anchor peers) requires only the admin
signature of that org.
* **The application** (like who the member orgs are) requires a majority of the
application organizations' admins to sign.
* **The orderer** requires a majority of the ordering organizations' admins (of
which there are by default only 1).
* **The top level `channel` group** requires both the agreement of a majority of
application organization admins and orderer organization admins.

If you have made changes to the default policies in the channel, you'll need to
compute your signature requirements accordingly.

*Note: you may be able to script the signature collection, dependent on your
application. In general, you may always collect more signatures than are
required.*

The actual process of getting these signatures will depend on how you've set up
your system, but there are two main implementations. Currently, the Fabric
command line defaults to a "pass it along" system. That is, the Admin of the Org
proposing a config update sends the update to someone else (another Admin,
typically) who needs to sign it. This Admin signs it (or doesn't) and passes it
along to the next Admin, and so on, until there are enough signatures for the
config to be submitted.

This has the virtue of simplicity -- when there are enough signatures, the last
Admin can simply submit the config transaction (in Fabric, the `peer channel update`
command includes a signature by default). However, this process will only be
practical in smaller channels, since the "pass it along" method can be time
consuming.

The other option is to submit the update to every Admin on a channel and wait
for enough signatures to come back. These signatures can then be stitched
together and submitted. This makes life a bit more difficult for the Admin who
created the config update (forcing them to deal with a file per signer) but is
the recommended workflow for users which are developing Fabric management
applications.

Once the config has been added to the ledger, it will be a best practice to
pull it and convert it to JSON to check to make sure everything was added
correctly. This will also serve as a useful copy of the latest config.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
