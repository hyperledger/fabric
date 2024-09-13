# Considerations for getting to v3.x

In this topic we'll cover recommendations for upgrading from a v2.x release to a v3.x release.

## Upgrading from a v2.x release to a v3.x release

### Before upgrading nodes

Fabric v3.x has removed some features that were deprecated in v2.x. Before upgrading components to v3.x ensure that you have shifted off of these removed features:

- **Support for 'Solo' consensus has been removed in Fabric v3.0.** The 'Solo' consensus type was intended for test environments only in prior releases and has never been supported for production environments. For trial environments you can utilize a single node Raft ordering service as demonstrated in the [test network tutorial](https://hyperledger-fabric.readthedocs.io/en/latest/test_network.html).
- **Support for 'Kafka' consensus has been removed in Fabric v3.0.** If you used Kafka consensus in prior releases, you must migrate to Raft consensus prior to upgrading to v3.x. For details about the migration process, see the [Migrating from Kafka to Raft documentation](https://hyperledger-fabric.readthedocs.io/en/release-2.5/kafka_raft_migration.html).
- **The legacy chaincode lifecycle from v1.x has been removed in Fabric v3.0.** Prior to upgrading peers to v3.x, you must update all channels to utilize the v2.x lifecycle by setting the channel application capability to either V2_0 or V2_5, and redeploying all chaincodes using the v2.x lifecycle. The new chaincode lifecycle provides a more flexible and robust governance model
for chaincodes. For more details see the [documentation for enabling the new lifecycle](https://hyperledger-fabric.readthedocs.io/en/release-2.5/enable_cc_lifecycle.html).

### Upgrading nodes

Upgrading nodes from a v2.x release to a v3.x release requires no special considerations, simply follow the steps in [Upgrading your components](./upgrading_your_components.html).

### After upgrading nodes

#### Capabilities

The 3.0 release features one new capability.

* **Channel** `V3_0`: this channel capability enables SmartBFT consensus and adds support for Ed25519 cryptographic algorithm in MSP credentials. Both features impact orderer node and peer node behavior.

Make sure to upgrade all orderer binaries and peer binaries before updating the `Channel` capability. Additionally, ensure that `OrdererEndpoints` are set at the organization level before updating to the V3_0 channel capability. See the next section **Define ordering node endpoint per org**.

For information about how to set new capabilities, check out [Updating the capability level of a channel](./updating_capabilities.html).

#### Define ordering node endpoint per org

Starting with version v1.4.2, it was recommended to define orderer endpoints in all channels at the organization level by adding a new `OrdererEndpoints` stanza within the channel configuration of an organization, replacing the global `OrdererAddresses` section of channel configuration. This update is required prior to enabling the V3_0 channel capability.

If at least one organization has an ordering service endpoint defined at an organizational level, all orderers and peers will ignore the channel level endpoints when connecting to ordering nodes.

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

#### Migrate to SmartBFT consensus (optional)

You may continue utilizing Raft consensus for a Crash Fault Tolerant ordering service, or [migrate to SmartBFT consensus](./raft_bft_migration.html) for a Byzantine Fault Tolerant ordering service after updating to the `V3_0` channel capability. SmartBFT may make sense if you distribute ordering service nodes across multiple organizations, but note that throughput may go down due to the additional consensus requirements.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
