# Considerations for getting to v2.0

## Chaincode lifecycle

The new chaincode lifecycle that debuts in v2.0 allows multiple organizations
to agree on how a chaincode will be operated before it can be used on a channel.
For more information about the new chaincode lifecycle, check out
[Chaincode for operators](./chaincode4noah.html).

It is a best practice to upgrade all of the peers on a channel before enabling
the `Channel` and `Application` capabilities that enable the new chaincode
lifecycle. Note that any peers that are not at v2.0 will crash after
enabling either capability, while any ordering nodes that are not at v2.0 will
crash after the `Channel` capability has been enabled. This crashing behavior is
intentional as it prevents the node from rejecting the config block (given that
it contains a value it does not understand) when nodes at the appropriate
node level would accept it.

After the `Application` capability has been updated to `V2_0`, you must use
the v2.0 lifecycle procedures to package, install, approve, and commit new
chaincodes on the channel. As a result, make sure to be prepared for the new
lifecycle before updating the capability.

The new lifecycle defaults to using the endorsement policy configured in the
channel config (e.g., a `MAJORITY` of orgs). Therefore this endorsement policy
should be added to the channel configuration when enabling capabilities on the
channel.

For information about how to edit the relevant channel configurations to enable
the new lifecycle by adding an endorsement policy for each organization, check
out [Enabling the new chaincode lifecycle](./enable_cc_lifecycle.html).

## Chaincode shim changes

Note that these changes only apply to chaincode written in Go.

The recommended approach is to vendor the shim in your v1.4 Go chaincode before
making upgrades to the peers and channels. If you do this, you do not need to
make any additional changes to your chaincode.

If you did not vendor the shim in your v1.4 chaincode, the old v1.4 chaincode
images will still technically work after upgrade, but you are in a risky state.
If the chaincode image gets deleted from your environment for whatever reason,
the next invoke on v2.0 peer will try to rebuild the chaincode image and you'll
get an error that the shim cannot be found.

At this point, you have two options:

1. If the entire channel is ready to upgrade chaincode, you can upgrade the
   chaincode on all peers and on the channel (using either the old or new
   lifecycle depending on the `Application` capability level you have enabled).
   The best practice at this point would be to vendor the new Go chaincode shim
   using modules.

2. If the entire channel is not yet ready to upgrade the chaincode, you can use
   peer environment variables to specify the v1.4 chaincode environment `ccenv`
   be used to rebuild the chaincode images. This v1.4 `ccenv` should still work
   with a v2.0 peer.

## Chaincode logger (Go chaincode only)

Support for user chaincodes to utilize the chaincode shim's logger via `NewLogger()`.
Chaincodes that used the shim's `NewLogger()` must now shift to their own
preferred logging mechanism.

For more information, check out [Logging control](./logging-control.html#chaincode).

## Peer databases upgrade

The databases of all peers (which include not just the state database but the
history database and other internal databases for the peer) must be rebuilt using
the v2.0 data format as part of the upgrade to v2.0. To trigger the rebuild, the
databases must be dropped before the peer is started. To do this, you must first
export the `FABRIC_CFG_PATH` to point to the `core.yaml` file for the peer. Then
issue:

```
peer node upgrade-dbs
```

Note that this process can take several hours, depending on the size of your
database. Once that has completed, issue the following command to start the node:

```
peer node start
```

Because rebuilding the databases can be a lengthy process, monitor the peer logs
to check the status of the rebuild. Every 1000th block you will see a message
like `[lockbasedtxmgr] CommitLostBlock -> INFO 041 Recommitting block [1000] to state database`
indicating the rebuild is ongoing.

If the database is not dropped as part of the upgrade process, the peer start
will return an error message stating that its databases are in the old format
and must be dropped. The node will then need to be restarted.

## Capabilities

As can be expected for a 2.0 release, there is a full complement of new
capabilities for 2.0.

* **Application** `V2_0`: enables the new chaincode lifecycle as described in
  [Chaincode for Operators](./chaincode4noah.html). Note that there are special
  considerations for the chaincode shim as part of the new lifecycle that have
  to be taken in account when upgrading. For more information, see the "Capability
  update and new lifecycle" section below.

* **Channel** `V2_0`: requires orderer endpoints to be configured at the org
   level. For more information, see the "Define orderer endpoint per org" section
   below.

* **Orderer** `V2_0`: controls `UseChannelCreationPolicyAsAdmins`,
  which determines whether the orderer should use the name  `Admins` instead of
  `ChannelCreationPolicy` in the new channel config template.

As with any update of the capability levels, make sure to upgrade your peer and
ordering node binaries before attempting to update the capability level of any
channels.

For information about how to set new capabilities, check out [Updating the capability level of a channel](./updating_capabilities.html).

## Define ordering node endpoint per org

Starting with version v1.4.2, it was recommended to define orderer endpoints at
the organization level (new `OrdererEndpoints` stanza within the channel
configuration of an organization) and not at the global `OrdererAddresses`
section of channel configuration. If at least one organization has an ordering
service endpoint defined at an organizational level, all orderers and peers will
ignore the channel level endpoints when connecting to ordering nodes.

Utilizing organization level orderer endpoints is required when using service
discovery with ordering nodes provided by multiple organizations. This allows
clients to provide the correct organization TLS certificates.

The support for `OrdererAddresses` was previously deprecated and has been
pulled in v2.0.

If your channel configuration does not yet include `OrdererEndpoints` per org,
you will need to perform a channel configuration update to add them to the config.
First, create a JSON file that includes the new configuration stanza.

In this example, we will create a stanza for a single org called `OrdererOrg`.
Note that if you have multiple ordering service organizations, they will all
have to be updated to include endpoints. Let's call our JSON file `orglevelEndpoints.json`.

```
{
  "OrdererOrgEndpoint": {
      "Endpoints": {
          "mod_policy": "Admins",
          "value": {
              "addresses": [
                 "127.0.0.1:30000"
              ]
          }
      }
   }
}
```

Then, export the following environment variables:

* `CH_NAME`: the name of the system channel being updated.
* `CORE_PEER_LOCALMSPID`: the MSP ID of the organization proposing the channel
  update. This will be the MSP of one of the orderer organizations.
* `CORE_PEER_MSPCONFIGPATH`: the absolute path to the MSP representing your
  organization.
* `TLS_ROOT_CA`: the absolute path to the root CA certificate of the organization
  proposing the system channel update.
* `ORDERING_NODE_ADDRESS`: the url of an ordering node.
  When targeting the ordering service, you can target any particular node in the
  ordering service. Your requests will be forwarded to the leader automatically.
* `ORGNAME`: The name of the organization you are currently updating. For example.
  `OrdererOrg`.

Once you have set the environment variables, navigate to
[Step 1: Pull and translate the config](./config_update.html#step-1-pull-and-translate-the-config).

Once you have a `modified_config.json`, add the lifecycle organization policy
(as listed in `orglevelEndpoints.json`) using this command:

```
jq -s ".[0] * {\"channel_group\":{\"groups\":{\"Orderer\": {\"groups\": {\"$ORGNAME\": {\"values\": .[1].${ORGNAME}Endpoint}}}}}}" config.json ./orglevelEndpoints.json > modified_config.json
```

Then, follow the steps at [Step 3: Re-encode and submit the config](./config_update.html#step-3-re-encode-and-submit-the-config).

If every ordering service organization performs their own channel edit, they can
edit the configuration without needing further signatures (by default, the only
signature needed to edit parameters within an organization is an admin of that
organization). If a different organization proposes the update, then the
organization being edited will need to sign the channel update request.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
