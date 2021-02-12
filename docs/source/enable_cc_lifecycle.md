# Enabling the new chaincode lifecycle

Users upgrading from v1.4.x to v2.x will have to edit their channel configurations to enable the new lifecycle features. This process involves a series of [channel configuration updates](./config_update.html) the relevant users will have to perform.

Note that the `Channel` and `Application` [capabilities](./capabilities_concept.html) of your application channels will have to be updated to `V2_0` for the new chaincode lifecycle to work. Check out [Considerations for getting to 2.0](./upgrade_to_newest_version.html#chaincode-lifecycle) for more information.

Updating a channel configuration is, at a high level, a three step process (for each channel):

1. Get the latest channel config
2. Create a modified channel config
3. Create a config update transaction

We will be performing these channel configuration updates by leveraging a file called `enable_lifecycle.json`, which contains all of the updates we will be making in the channel configurations. Note that in a production setting it is likely that multiple users would be making these channel update requests. However, for the sake of simplicity, we are presenting all of the updates as how they would appear in a single file.

Note: this topic describes a network that does not use a "system channel", a channel that the ordering service is bootstrapped with and the ordering service exclusively controls. Since the release of v2.3, using a system channel is now considered the legacy process as compared to the new process to [Create a channel](./create_channel/create_channel_participation.html) without a system channel. For a version of this topic that includes information about the system channel, check out [Enabling the new chaincode lifecycle](https://hyperledger-fabric.readthedocs.io/en/release-2.2/enable_cc_lifecycle.html) from the v2.2 documentation.

## Create `enable_lifecycle.json`

Note that in addition to using `enable_lifecycle.json`, this tutorial also uses `jq` to apply the edits to the modified config file. The modified config can also be edited manually (after it has been pulled, translated, and scoped). Check out this [sample channel configuration](./config_update.html#sample-channel-configuration) for reference.

However, the process described here (using a JSON file and a tool like `jq`) does have the advantage of being scriptable, making it suitable for proposing configuration updates to a large number of channels, and is the recommended process for editing a channel configuration.

Note that the `enable_lifecycle.json` uses sample values, for example `org1Policies` and the `Org1ExampleCom`, which will be specific to your deployment):

```
{
  "org1Policies": {
      "Endorsement": {
           "mod_policy": "Admins",
           "policy": {
               "type": 1,
               "value": {
               "identities": [
                  {
	                 "principal": {
	           	         "msp_identifier": "Org1ExampleCom",
	           	         "role": "PEER"
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
  "org2Policies": {
      "Endorsement": {
           "mod_policy": "Admins",
           "policy": {
               "type": 1,
               "value": {
               "identities": [
                  {
	                 "principal": {
	           	         "msp_identifier": "Org2ExampleCom",
	           	         "role": "PEER"
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
   "appPolicies": {
 		"Endorsement": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "MAJORITY",
					"sub_policy": "Endorsement"
				}
			},
			"version": "0"
		},
		"LifecycleEndorsement": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "MAJORITY",
					"sub_policy": "Endorsement"
				}
			},
			"version": "0"
		}
   },
   "acls": {
		"_lifecycle/CheckCommitReadiness": {
			"policy_ref": "/Channel/Application/Writers"
		},
		"_lifecycle/CommitChaincodeDefinition": {
			"policy_ref": "/Channel/Application/Writers"
		},
		"_lifecycle/QueryChaincodeDefinition": {
			"policy_ref": "/Channel/Application/Writers"
		},
		"_lifecycle/QueryChaincodeDefinitions": {
			"policy_ref": "/Channel/Application/Writers"
		}
   }
}
```

**Note: the "role" field of these new policies should say `'PEER'` if [NodeOUs](./msp.html#organizational-units) are enabled for the org, and `'MEMBER'` if they are not.**

## Edit the channel configurations

To fully enable the new chaincode lifecycle, you must first edit the configuration of your own organization as it exists in a channel configuration, and then you must update the channel itself to include a default endorsement policy for the channel. You can then optionally update your channel access control list.

Note: this topic leverages the instructions on how to update a channel configuration that are found in the [Updating a channel configuration](./config_update.html) tutorial. The environment variables listed here work in conjunction with those commands to update your channels.

### Edit the peer organizations

By default, peer organizations are able to make configuration update requests to their own organization on an application channel without needing the approval of any other peer organizations. However, if you are attempting to make a change to a different organization, that organization will have to approve the change.

You will need to export the following variables:

* `CH_NAME`: the name of the application channel being updated.
* `ORGNAME`: The name of the organization you are currently updating.
* `TLS_ROOT_CA`: the absolute path to the TLS cert of your ordering node.
* `CORE_PEER_MSPCONFIGPATH`: the absolute path to the MSP representing your organization.
* `CORE_PEER_LOCALMSPID`: the MSP ID of the organization proposing the channel update. This will be the MSP of one of the peer organizations.
* `ORDERER_CONTAINER`: the name of an ordering node container. When targeting the ordering service, you can target any particular node in the ordering service. Your requests will be forwarded to the leader automatically.

Once you have set the environment variables, navigate to [Step 1: Pull and translate the config](./config_update.html#step-1-pull-and-translate-the-config).

Then, add the lifecycle organization policy (as listed in `enable_lifecycle.json`) to a file called `modified_config.json` using this command:

```
jq -s ".[0] * {\"channel_group\":{\"groups\":{\"Application\": {\"groups\": {\"$ORGNAME\": {\"policies\": .[1].${ORGNAME}Policies}}}}}}" config.json ./enable_lifecycle.json > modified_config.json
```

Then, follow the steps at [Step 3: Re-encode and submit the config](./config_update.html#step-3-re-encode-and-submit-the-config).

### Edit the application channels

After all of the application channels have been [updated to include V2_0 capabilities](./upgrade_to_newest_version.html#capabilities), endorsement policies for the new chaincode lifecycle must be added to each channel.

You can set the same environment variables you set when updating the peer organizations. Note that in this case you will not be updating the configuration of an org in the configuration, so the `ORGNAME` variable will not be used.

Once you have set the environment variables, navigate to [Step 1: Pull and translate the config](./config_update.html#step-1-pull-and-translate-the-config).

Then, add the lifecycle organization policy (as listed in `enable_lifecycle.json`) to a file called `modified_config.json` using this command:

```
jq -s '.[0] * {"channel_group":{"groups":{"Application": {"policies": .[1].appPolicies}}}}' config.json ./enable_lifecycle.json > modified_config.json
```

Then, follow the steps at [Step 3: Re-encode and submit the config](./config_update.html#step-3-re-encode-and-submit-the-config).

For this channel update to be approved, the policy for modifying the `Channel/Application` section of the configuration must be satisfied. By default, this is a `MAJORITY` of the peer organizations on the channel.

#### Edit channel ACLs (optional)

The following [Access Control List (ACL)](./access_control.html) in `enable_lifecycle.json` are the default values for the new lifecycle, though you have the option to change them depending on your use case.

```
"acls": {
 "_lifecycle/CheckCommitReadiness": {
   "policy_ref": "/Channel/Application/Writers"
 },
 "_lifecycle/CommitChaincodeDefinition": {
   "policy_ref": "/Channel/Application/Writers"
 },
 "_lifecycle/QueryChaincodeDefinition": {
   "policy_ref": "/Channel/Application/Readers"
 },
 "_lifecycle/QueryChaincodeDefinitions": {
   "policy_ref": "/Channel/Application/Readers"
```

You can leave the same environment in place as when you previously edited application channels.

Once you have the environment variables set, navigate to [Step 1: Pull and translate the config](./config_update.html#step-1-pull-and-translate-the-config).

Then, add the ACLs (as listed in `enable_lifecycle.json`) and create a file called `modified_config.json` using this command:

```
jq -s '.[0] * {"channel_group":{"groups":{"Application": {"values": {"ACLs": {"value": {"acls": .[1].acls}}}}}}}' config.json ./enable_lifecycle.json > modified_config.json
```

Then, follow the steps at [Step 3: Re-encode and submit the config](./config_update.html#step-3-re-encode-and-submit-the-config).

For this channel update to be approved, the policy for modifying the `Channel/Application` section of the configuration must be satisfied. By default, this is a `MAJORITY` of the peer organizations on the channel.

## Enable new lifecycle in `core.yaml`

If you follow [the recommended process](./upgrading_your_components.html#overview) for using a tool like `diff` to compare the new version of `core.yaml` packaged with the binaries with your old one, you will not need to add `_lifecycle: enable` to the list of enabled system chaincodes because the new `core.yaml` has added it under `chaincode/system`.

However, if you are updating your old node YAML file directly, you will have to add `_lifecycle: enable` to the list of enabled system chaincodes.

For more information about upgrading nodes, check out [Upgrading your components](./upgrading_your_components.html).

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
