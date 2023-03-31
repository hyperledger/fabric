# Considerations for getting to v2.x

In this topic we'll cover recommendations for upgrading from a prior v2.x release to the v2.5 long-term support (LTS) release, as well as from the v1.4 release.

## Upgrading from a prior v2.x release to the v2.5 LTS release.

Upgrading nodes from a prior v2.x release to the v2.5 LTS release requires no special considerations, simply follow the steps in [Upgrading your components](./upgrading_your_components.html). Upon the first start each v2.5 peer will rebuild some internal indexes for managing private data.

The 2.5 release featured one new application capability:

* **Application** capability `V2_5` enables purging the history of private data from a peer while preserving a hash of the private data as immutable evidence on the blockchain as described in the [Purging private data](./private-data/private-data.html#purging-private-data-in-chaincode) concept topic.

If you would like to take advantage of the new feature, make sure to upgrade your peer binaries before updating the **Application** capability to `V2_5`. If you do not need the new feature, the **Application** capability can remain at `V2_0`.

For information about how to set new capabilities, check out [Updating the capability level of a channel](./updating_capabilities.html).

## Upgrading from v1.4.x release to a v2.x release

Before attempting to upgrade from v1.4.x to v2.x, make sure to consider the following before, during, and after your peer upgrades.

### Prior to upgrading peer nodes

It is important to ensure chaincode utilizes good practices consistent with v2.x prior to upgrading nodes and enabling the new chaincode lifecycle.
Consider updating chaincode based on the below guidelines prior to upgrading nodes.

#### Chaincode shim changes (Go chaincode only)

The v2.x `ccenv` image that is used to build Go chaincodes no longer automatically vendors the Go chaincode shim dependency like the v1.4 `ccenv` image did.
The recommended approach is to vendor the shim in your v1.4 Go chaincode before making upgrades to the peers and channels, since this approach works with both a v1.4.x and v2.x peer. If you are already using an existing tool such as ``govendor`` you may continue using it to vendor the chaincode shim. Best practice, however, would be to use Go modules to vendor the chaincode shim, as modules are now the de facto standard for dependency management in the Go ecosystem. Note that since Fabric v2.0, chaincode using Go modules without vendored dependencies is also supported. If you do this, you do not need to make any additional changes to your chaincode.

If you did not vendor the shim in your v1.4 chaincode, the old v1.4 chaincode images will still technically work after peer upgrade, but you are in a risky state. If the chaincode image gets deleted from your environment for whatever reason, the next invoke on v2.x peer will try to rebuild the chaincode image and you'll get an error that the shim cannot be found.

At this point, you have two options:

1. If the entire channel is ready to upgrade chaincode, you can upgrade the chaincode on all peers and on the channel (using either the old or new lifecycle depending on the `Application` capability level you have enabled). The best practice at this point would be to vendor the new Go chaincode shim using modules.

2. If the entire channel is not yet ready to upgrade the chaincode, you can use peer environment variables to specify the v1.4 chaincode environment `ccenv` be used to rebuild the chaincode images. This v1.4 `ccenv` should still work with a v2.x peer.

To eliminate these concerns, the recommendation is to make sure the chaincode shim is vendored and the chaincode is upgraded, prior to upgrading a peer to v2.x.

#### Chaincode logger (Go chaincode only)

Support for user chaincodes to utilize the chaincode shim's logger via `NewLogger()` has been removed. Chaincodes that used the shim's `NewLogger()` must now shift to their own preferred logging mechanism.

For more information, check out [Logging control](./logging-control.html#chaincode).

#### Node.js v1.4 Chaincode

For v1.4 Node.js chaincode libraries, the supported node runtime is v8. Though not formally supported, the v1.4 libraries can work on later versions. For example the Node.js 12 runtime that was used with the Fabric v2.2 `fabric-nodeenv` image.  Fabric v2.4 `fabric-nodeenv` has upgraded to Node.js 16 and Fabric v2.5 `fabric-nodeenv` has upgraded to Node.js 18. It is _not possible_ to run the v1.4 libraries on Node.js 16 or Node.js 18.  Therefore any chaincode written with the v1.4 libraries will need to be updated prior to upgrading to Fabric v2.4 or Fabric v2.5.

Also please note that the v1.4 libraries themselves are not supported, and will no longer get updates. Please migrate to a v2.x Node.js chaincode library.

For detailed information please refer to the [compatibility](https://github.com/hyperledger/fabric-chaincode-node/blob/main/COMPATIBILITY.md) document in the `fabric-chaincode-node` repository.

#### Chaincode initialization

The new chaincode lifecycle does not have an option to automatically call a chaincode `Init` function upon chaincode deployment.
Instead, the new chaincode lifecycle has a mechanism to flag that chaincode initialization is required,
by using the `--init-required` flag, and providing a `--isInit` flag on the `peer chaincode invoke` command.
However, in most scenarios it is recommended to embed initialization logic into chaincode rather than use the chaincode lifecycle mechanism.
Chaincode functions often perform checks against existing state, and initialization state can be implemented like any other chaincode state and be checked in subsequent chaincode function calls.
Handling initialization state within chaincode logic rather than with the chaincode lifecycle
mechanism has the benefit that you are not limited to a single initialization function,
rather you are in full control of initialization logic and can call your own
functions that initialize state from an application consistent with how all other application functions are called.

### While upgrading peer nodes

#### Peer databases upgrade

For information about how to upgrade peers, check out our documentation on [upgrading components](./upgrading_your_components.html). During the process for [upgrading your peers](./upgrading_your_components.html#upgrade-the-peers), you will need to perform one additional step to upgrade the peer databases. The databases of all peers (which include not just the state database but the history database and other internal databases for the peer) must be rebuilt using the v2.x data format as part of the upgrade to v2.x. To trigger the rebuild, the databases must be dropped before the peer is started. The instructions below utilize the `peer node upgrade-dbs` command to drop the local databases managed by the peer and prepare them for upgrade, so that they can be rebuilt the first time the v2.x peer starts. If you are using CouchDB as the state database, the peer has support to automatically drop this database as of v2.2. To leverage the support, you must configure the peer with CouchDB as the state database and start CouchDB before running the `upgrade-dbs` command. In v2.0 and v2.1, the peer does not automatically drop the CouchDB state database; therefore you must drop it yourself.

Follow the commands to upgrade a peer until the `docker run` command to launch the new peer container (you can skip the step where you set an `IMAGE_TAG`, since the `upgrade-dbs` command is for the v2.x release of Fabric only, but you will need to set the `PEER_CONTAINER` and `LEDGERS_BACKUP` environment variables). Instead of the `docker run` command to launch the peer, run this command instead to drop and prepare the local databases managed by the peer (substitute `2.1` for `2.0` in these commands if you are upgrading to that binary version from the 1.4.x LTS):

```
docker run --rm -v /opt/backup/$PEER_CONTAINER/:/var/hyperledger/production/ \
            -v /opt/msp/:/etc/hyperledger/fabric/msp/ \
            --env-file ./env<name of node>.list \
            --name $PEER_CONTAINER \
            hyperledger/fabric-peer:2.0 peer node upgrade-dbs
```

In v2.0 and v2.1, if you are using CouchDB as the state database, also drop the CouchDB database. This can be done by removing the CouchDB /data volume directory.

Then issue this command to start the peer using the `2.0` tag:

```
docker run -d -v /opt/backup/$PEER_CONTAINER/:/var/hyperledger/production/ \
            -v /opt/msp/:/etc/hyperledger/fabric/msp/ \
            --env-file ./env<name of node>.list \
            --name $PEER_CONTAINER \
            hyperledger/fabric-peer:2.0 peer node start
```

The peer will rebuild the databases using the v2.x data format the first time it starts. Because rebuilding the databases can be a lengthy process (several hours, depending on the size of your databases), monitor the peer logs to check the status of the rebuild. Every 1000th block you will see a message like `[lockbasedtxmgr] CommitLostBlock -> INFO 041 Recommitting block [1000] to state database` indicating the rebuild is ongoing.

If the database is not dropped as part of the upgrade process, the peer start will return an error message stating that its databases are in the old format and must be dropped using the `peer node upgrade-dbs` command above (or dropped manually if using CouchDB state database). The node will then need to be restarted again.

### After upgrading nodes

#### Chaincode lifecycle

The new chaincode lifecycle that debuted in v2.0 allows multiple organizations to agree on how a chaincode will be operated before it can be used on a channel. For more information about the new chaincode lifecycle, check out [Fabric chaincode lifecycle](./chaincode_lifecycle.html) concept topic.

It is a best practice to upgrade all of the peers on a channel before enabling the `Channel` and `Application` capabilities that enable the new chaincode lifecycle (the `Channel` capability is not strictly required, but it makes sense to update it at this time). Note that any peers that are not at v2.x will crash after enabling either capability, while any ordering nodes that are not at v2.x will crash after the `Channel` capability has been enabled. This crashing behavior is intentional, as the peer or orderer cannot safely participate in the channel if it does not support the required capabilities.

After the `Application` capability has been updated to `V2_0` on a channel, chaincodes that were deployed using the v1.x chaincode lifecycle will continue to run, and will continue to utilize the same chaincode endorsement policy as before.
However, the next time that you would like to update these chaincodes, you must utilize the v2.x lifecycle procedures to package, install, approve, and commit chaincodes on the channel. This will effectively shift the chaincode from the v1.x lifecycle framework to the v2.x lifecycle framework.
Additionally, any new chaincode deployments must also utilize the v2.x lifecycle procedures to package, install, approve, and commit chaincodes on the channel.
As a result, make sure to be prepared for the new lifecycle before updating the capability.

The new lifecycle defaults to using the endorsement policy configured in the channel config (e.g., a `MAJORITY` of orgs). Therefore this endorsement policy should be added to the channel configuration when enabling capabilities on the channel.

For information about how to edit the relevant channel configurations to enable the new lifecycle by adding an endorsement policy for each organization, check out [Enabling the new chaincode lifecycle](./enable_cc_lifecycle.html).

#### Capabilities

The 2.0 release featured three new capabilities.

* **Application** `V2_0`: enables the new chaincode lifecycle as described in [Fabric chaincode lifecycle](./chaincode_lifecycle.html) concept topic.

* **Channel** `V2_0`: this capability has no changes, but is used for consistency with the application and orderer capability levels.

* **Orderer** `V2_0`: controls `UseChannelCreationPolicyAsAdmins`, changing the way that channel creation transactions are validated. When combined with the `-baseProfile` option of configtxgen, values which were previously inherited from the orderer system channel may now be overridden.

As with any update of the capability levels, make sure to upgrade your peer binaries before updating the `Application` and `Channel` capabilities, and make sure to upgrade your orderer binaries before updating the `Orderer` and `Channel` capabilities.

For information about how to set new capabilities, check out [Updating the capability level of a channel](./updating_capabilities.html).

#### Define ordering node endpoint per org (recommend)

Starting with version v1.4.2, it was recommended to define orderer endpoints in all channels at the organization level by adding a new `OrdererEndpoints` stanza within the channel configuration of an organization, replacing the global `OrdererAddresses` section of channel configuration. If at least one organization has an ordering service endpoint defined at an organizational level, all orderers and peers will ignore the channel level endpoints when connecting to ordering nodes.

Utilizing organization level orderer endpoints is required when using service discovery with ordering nodes provided by multiple organizations. This allows clients to provide the correct organization TLS certificates.

If your channel configuration does not yet include `OrdererEndpoints` per org, you will need to perform a channel configuration update to add them to the config. First, create a JSON file that includes the new configuration stanza.

In this example, we will create a stanza for a single org called `OrdererOrg`. Note that if you have multiple ordering service organizations, they will all have to be updated to include endpoints. Let's call our JSON file `orglevelEndpoints.json`.

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

* `CH_NAME`: the name of the channel being updated.
* `CORE_PEER_LOCALMSPID`: the MSP ID of the organization proposing the channel update. This will be the MSP of one of the orderer organizations.
* `CORE_PEER_MSPCONFIGPATH`: the absolute path to the MSP representing your organization.
* `TLS_ROOT_CA`: the absolute path to the root CA certificate of the organization proposing the channel update.
* `ORDERER_CONTAINER`: the name of an ordering node container. When targeting the ordering service, you can target any particular node in the ordering service. Your requests will be forwarded to the leader automatically.
* `ORGNAME`: The name of the organization you are currently updating. For example, `OrdererOrg`.

Once you have set the environment variables, navigate to [Step 1: Pull and translate the config](./config_update.html#step-1-pull-and-translate-the-config).

Then, add the lifecycle organization policy (as listed in `orglevelEndpoints.json`) to a file called `modified_config.json` using this command:

```
jq -s ".[0] * {\"channel_group\":{\"groups\":{\"Orderer\": {\"groups\": {\"$ORGNAME\": {\"values\": .[1].${ORGNAME}Endpoint}}}}}}" config.json ./orglevelEndpoints.json > modified_config.json
```

Then, follow the steps at [Step 3: Re-encode and submit the config](./config_update.html#step-3-re-encode-and-submit-the-config).

If every ordering service organization performs their own channel edit, they can edit the configuration without needing further signatures (by default, the only signature needed to edit parameters within an organization is an admin of that organization). If a different organization proposes the update, then the organization being edited will need to sign the channel update request.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
