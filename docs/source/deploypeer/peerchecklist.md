#  Checklist for a production peer

As you prepare to build a production peer, you need to customize the configuration by editing the [core.yaml](https://github.com/hyperledger/fabric/blob/{BRANCH}/sampleconfig/core.yaml) file, also visible inside the peer binary `\config` folder. Ideally, peer configuration happens before the peer is started.

While in a production environment you could override the environment variables in the `core.yaml` file in your Docker container or your Kubernetes job, these instructions show how to edit `core.yaml` instead. It’s important to understand the parameters in the configuration file and their dependancies on other parameter settings in the file. Blindly overriding one setting using an environment variable could affect the functionality of another setting. Therefore, the recommendation is that before starting the peer, you make the modifications to the settings in the configuration file to become familiar with the available settings and how they work. Afterwards, you may choose to override these parameters using environment variables.

This checklist covers key configuration parameters for setting up a production network. Of course, you can always refer to the `core.yaml` file for additional parameters or more information. The list of parameters that you need to understand and that are described in this topic include:

- [id](#id)
- [networkId](#networkid)
- [listenAddress](#listenaddress)
- [chaincodeListenAddress](#chaincodelistenaddress)
- [chaincodeAddress](#chaincodeaddress)
- [address](#address)
- [mspConfigPath](#mspconfigpath)
- [localMspId](#localmspid)
- [fileSystemPath](#filesystempath)
- [gossip](#gossip)
- [tls](#tls)
- [bccsp](#bccsp)
- [externalBuilders](#externalBuilders)
- [ledger](#ledger)
- [operations](#operations)
- [metrics](#metrics)

## id

```
# The peer id provides a name for this peer instance and is used when
# naming docker resources.
id: jdoe
```
- **`id`**: (Default value should be overridden.) Start by giving your peer an ID (which is analogous to giving it a name). Often the name indicates the organization that the peer belongs to, for example `peer0.org1.example.com`. The `peer.id` parameter is the name used by service discovery and peer gossip to address the peer. It is used for naming this peer, associated chaincode images, and peer containers.

## networkId

```
# The networkId allows for logical separation of networks and is used when
# naming docker resources.
networkId: dev
```

- **`networkId`**: (Default value should be overridden.) Specify any name that you want. One recommendation would be to differentiate the network by naming it based on its planned usage (for example, “dev”, “staging”, “test”, ”production”, etc). This value is also used to build the name of the chaincode images and peer containers.

## listenAddress

```
# The Address at local network interface this Peer will listen on.
# By default, it will listen on all network interfaces
listenAddress: 0.0.0.0:7051
```
- **`listenAddress`**: (Default value should be overridden.) Specify the address that the peer will listen on, for example, `127.0.0.1:7051`.

## chaincodeListenAddress

```
# The endpoint this peer uses to listen for inbound chaincode connections.
# If this is commented-out, the listen address is selected to be
# the peer's address (see below) with port 7052
chaincodeListenAddress: 0.0.0.0:7052
```

- **`chaincodeListenAddress`**: (Default value should be overridden.) Uncomment this parameter and specify the address where this peer listens for chaincode requests. It needs to be different than the `peer.listenAddress`, for example, `127.0.0.1:7052`. This is the network-addressable name that other peers should use.

## chaincodeAddress
```
# The endpoint the chaincode for this peer uses to connect to the peer.
# If this is not specified, the chaincodeListenAddress address is selected.
# And if chaincodeListenAddress is not specified, address is selected from
# peer address (see below). If specified peer address is invalid then it
# will fallback to the auto detected IP (local IP) regardless of the peer
# addressAutoDetect value.
chaincodeAddress: 0.0.0.0:7052
```
- **`chaincodeAddress`**: (Default value should be overridden.) Uncomment this parameter and specify the address that chaincode containers can use to connect to this peer, for example, `peer0.org1.example.com:7052`.

## address
```
# When used as peer config, this represents the endpoint to other peers
# in the same organization. For peers in other organization, see
# gossip.externalEndpoint for more info.
# When used as CLI config, this means the peer's endpoint to interact with
address: 0.0.0.0:7051
```
- **`address`**: (Default value should be overridden.) Specify the address that other peers in the organization use to connect to this peer, for example, `peer0.org1.example.com:7051`.

## mspConfigPath
```
mspConfigPath: msp
```
- **`mspConfigPath`**: (Default value should be overridden.) This is the path to the peer's local MSP, which must be created before the peer can be deployed. The path can be absolute or relative to `FABRIC_CFG_PATH` (by default, it is `/etc/hyperledger/fabric` in the peer image). Unless an absolute path is specified to a folder named something other than "msp", the peer defaults to looking for a folder called “msp” at the path (in other words, `FABRIC_CFG_PATH/msp`) and when using the peer image: `/etc/hyperledger/fabric/msp`. If you are using the recommended folder structure described in the [Registering and enrolling identities with a CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html) topic, it would be relative to the FABRIC_CFG_PATH as follows:
`config/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/msp`

## localMspId
```
# Identifier of the local MSP
# ----!!!!IMPORTANT!!!-!!!IMPORTANT!!!-!!!IMPORTANT!!!!----
# Deployers need to change the value of the localMspId string.
# In particular, the name of the local MSP ID of a peer needs
# to match the name of one of the MSPs in each of the channel
# that this peer is a member of. Otherwise this peer's messages
# will not be identified as valid by other nodes.
localMspId: SampleOrg
```

- **`localMspId`**: (Default value should be overridden.) This is the value of the MSP ID of the organization the peer belongs to. Because peers can only be joined to a channel if the organization the peer belongs to is a channel member, this MSP ID must match the name of at least one of the MSPs in each of the channels that this peer is a member of.

## fileSystemPath
```
# Path on the file system where peer will store data (eg ledger). This
# location must be access control protected to prevent unintended
# modification that might corrupt the peer operations.
fileSystemPath: /var/hyperledger/production
```
- **`fileSystemPath`**: (Default value should be overridden.) This is the path to the ledger and installed chaincodes on the local filesystem of the peer. It can be an absolute path or relative to FABRIC_CFG_PATH. It defaults to `/var/hyperledger/production`. The user running the peer needs to own and have write access to this directory.

## gossip

```
gossip:
    # Bootstrap set to initialize gossip with.
    # This is a list of other peers that this peer reaches out to at startup.
    # Important: The endpoints here have to be endpoints of peers in the same
    # organization, because the peer would refuse connecting to these endpoints
    # unless they are in the same organization as the peer.
    bootstrap: 127.0.0.1:7051

    # Overrides the endpoint that the peer publishes to peers
    # in its organization. For peers in foreign organizations
    # see 'externalEndpoint'
    endpoint:

    # This is an endpoint that is published to peers outside of the organization.
    # If this isn't set, the peer will not be known to other organizations.
    externalEndpoint:

    # NOTE: orgLeader and useLeaderElection parameters are mutual exclusive.
    # Setting both to true would result in the termination of the peer
    # since this is undefined state. If the peers are configured with
    # useLeaderElection=false, make sure there is at least 1 peer in the
    # organization that its orgLeader is set to true.

    # Defines whenever peer will initialize dynamic algorithm for
    # "leader" selection, where leader is the peer to establish
    # connection with ordering service and use delivery protocol
    # to pull ledger blocks from ordering service.
    useLeaderElection: false

    # Statically defines peer to be an organization "leader",
    # where this means that current peer will maintain connection
    # with ordering service and disseminate block across peers in
    # its own organization. Multiple peers or all peers in an organization
    # may be configured as org leaders, so that they all pull
    # blocks directly from ordering service.
    orgLeader: true

    # Gossip state transfer related configuration
    state:
        # indicates whenever state transfer is enabled or not
        # default value is true, i.e. state transfer is active
        # and takes care to sync up missing blocks allowing
        # lagging peer to catch up to speed with rest network
        enabled: false

    pvtData:
      implicitCollectionDisseminationPolicy:
          # requiredPeerCount defines the minimum number of eligible peers to which the peer must successfully
          # disseminate private data for its own implicit collection during endorsement. Default value is 0.
          requiredPeerCount: 0

          # maxPeerCount defines the maximum number of eligible peers to which the peer will attempt to
          # disseminate private data for its own implicit collection during endorsement. Default value is 1.
          maxPeerCount: 1
```

Peers leverage the Gossip data dissemination protocol to broadcast ledger and channel data in a scalable fashion. Gossip messaging is continuous, and each peer on a channel is constantly receiving current and consistent ledger data from multiple peers. While there are many Gossip parameters that can be customized, there are three groups of settings you need to pay attention to at a minimum:

* **Endpoints** Gossip is required for block dissemination (not recommended other than for private data transactions) and service discovery (recommended). To use any of these features, you must configure the gossip `bootstrap`, `endpoint`, and `externalEndpoint` parameters in addition to **setting at least one anchor peer** in the peer’s channel configuration.

  - **`bootstrap`**: (Default value should be overridden.) Provide the list of other peer [addresses](#address) in this organization to discover.

  - **`endpoint`**: (Default value should be overridden.) Specify the address that other peers _in this organization_ should use to connect to this peer. For example, `peer0.org1.example.com:7051`.

  - **`externalEndpoint:`** (Default value should be overridden.) Specify the address that peers in _other organizations_ should use to connect to this peer, for example, `peer0.org1.example.com:7051`.

* **Block dissemination** As of Fabric v2.2, in order to reduce network traffic, it is recommended that peers get their blocks from non-private data transactions from the ordering service instead of from other peers in their organization. The combination of the `useLeaderElection:`, `orgLeader:`, and `state.enabled` parameters in this section control this behavior.

  - **`useLeaderElection:`** (Defaults to `false` as of v2.2, which is recommended so that peers get blocks from ordering service.) When `useLeaderElection` is set to false, you must configure at least one peer to be the org leader by setting `peer.gossip.orgLeader` to true. Set `useLeaderElection` to true if you prefer that peers use Gossip for block dissemination among peers in the organization.

  - **`orgLeader:`** (Defaults to `true` as of v2.2, which is recommended so that peers get blocks from ordering service.) Set this value to `false` if you want to use Gossip for block dissemination among peers in the organization.

  - **`state.enabled:`** (Defaults to `false` as of v2.2 which is recommended so that peers get blocks from ordering service.) Set this value to `true` when you want to use Gossip to sync up missing blocks, which allows a lagging peer to catch up with other peers on the network.

* **Implicit data** Fabric v2.0 introduced the concept of private data implicit collections on a peer. If you’d like to utilize per-organization private data patterns, you don’t need to define any collections when deploying chaincode in Fabric v2.x. Implicit organization-specific collections can be used without any upfront definition. When you plan to take advantage of this new feature, you need to configure the values of the `pvtData.implicitCollectionDisseminationPolicy.requiredPeerCount` and `pvtData.implicitCollectionDisseminationPolicy.maxPeerCount`. For more details, review the [Private data tutorial](../private_data_tutorial.html).
  - **`pvtData.implicitCollectionDisseminationPolicy.requiredPeerCount`:** (Recommended that you override this value when using private data implicit collections.) **New in Fabric 2.0.** It defaults to 0, but you will need to increase it based on the number of peers belonging to your organization. The value represents the maximum number of peers within your own organization that the data must be disseminated to.

  - **`pvtData.implicitCollectionDisseminationPolicy.maxPeerCount`:** (Recommended that you override this value when using private data implicit collections.) **New in Fabric 2.0.** This is an organization-specific collection setting that is used to ensure the private data is disseminated elsewhere in case this peer endorses a request and then goes down for some reason. The default is set to `1` but in a production environment with `n` peers in an organization, the recommended setting is `n-1`.

## tls

```
tls:
# Require server-side TLS
enabled:  false
# Require client certificates / mutual TLS.
# Note that clients that are not configured to use a certificate will
# fail to connect to the peer.
clientAuthRequired: false
# X.509 certificate used for TLS server
cert:
    file: tls/server.crt
# Private key used for TLS server (and client if clientAuthEnabled
# is set to true
key:
    file: tls/server.key
# Trusted root certificate chain for tls.cert
rootcert:
    file: tls/ca.crt
# Set of root certificate authorities used to verify client certificates
clientRootCAs:
    files:
      - tls/ca.crt
```

Configure this section to enable TLS communications for the peer. After TLS is enabled, all nodes that transact with the peer will also need to enable TLS. Review the topic on [Registering and enrolling identities with a CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html) for instructions on how to generate the peer TLS certificates.

- **`tls.enabled`:** (Default value should be overridden.) To ensure your production environment is secure, TLS should be enabled for all communications between nodes by setting `enabled: true` in the `tls` section of the config file. While this field is disabled by default, which may be acceptable for a test network, it should to be enabled when in production. This setting will configure **server-side TLS**, meaning that TLS will guarantee the identity of the _server_ to the client and provides a two-way encrypted channel between them.

- **`tls.cert.file`:** (Default value should be overridden.) Every peer needs to register and enroll with its TLS CA before it can transact securely with other nodes in the organization. Therefore, before you can deploy a peer, you must first register a user for the peer and enroll the peer identity with the TLS CA to generate the peer's TLS signed certificate. If you are using the recommended folder structure from the [Registering and enrolling identities with a CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html) topic, this file needs to be copied into `config/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls`

- **`tls.key.file`:** (Default value should be overridden.) Similar to the `tls.cert.file`, provide the name and location of the generated TLS private key for this peer, for example, `/msp/keystore/87bf5eff47d33b13d7aee81032b0e8e1e0ffc7a6571400493a7c_sk`. If you are using the recommended folder structure from the [Registering and enrolling identities with a CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html) topic, this file needs to be copied into `config/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls`.  If you are using an [HSM](#bccsp) to store the private key for the peer, this field will be blank.

- **`tls.rootcert.file`:**  (Default value should be overridden.) This value contains the name and location of the peer organization CA root certificate. If you are using the recommended folder structure from the [Registering and enrolling identities with a CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html) topic, this file needs to be copied into `config/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls`.

The next two parameters only need to be provided when mutual TLS is required:

- **`tls.clientAuthRequired`:** Defaults to `false`. Set to `true` for a higher level of security by using **mutual TLS**, which can be configured as an extra verification step of the client-side TLS certificate. Where server-side TLS is considered the minimally necessary level of security, mutual TLS is an additional and optional level of security.

- **`tls.clientRootCAs.files`:** Specify the list of client root CA certificate files that can be used to verify client certificates.

## BCCSP

```
BCCSP:
        Default: SW
        # Settings for the SW crypto provider (i.e. when DEFAULT: SW)
        SW:
            # TODO: The default Hash and Security level needs refactoring to be
            # fully configurable. Changing these defaults requires coordination
            # SHA2 is hardcoded in several places, not only BCCSP
            Hash: SHA2
            Security: 256
            # Location of Key Store
            FileKeyStore:
                # If "", defaults to 'mspConfigPath'/keystore
                KeyStore:
        # Settings for the PKCS#11 crypto provider (i.e. when DEFAULT: PKCS11)
        PKCS11:
            # Location of the PKCS11 module library
            Library:
            # Token Label
            Label:
            # User PIN
            Pin:
            Hash:
            Security:
```

(Optional) This section is used to configure the Blockchain crypto provider.

- **`BCCSP.Default:`** If you plan to use a Hardware Security Module (HSM), then this must be set to `PKCS11`.

- **`BCCSP.PKCS11.*:`** Provide this set of parameters according to your HSM configuration. Refer to this [example]((../hsm.html) of an HSM configuration for more information.

## externalBuilders

```
# List of directories to treat as external builders and launchers for
    # chaincode. The external builder detection processing will iterate over the
    # builders in the order specified below.
    externalBuilders: []
        # - path: /path/to/directory
        #   name: descriptive-builder-name
        #   propagateEnvironment:
        #      - ENVVAR_NAME_TO_PROPAGATE_FROM_PEER
        #      - GOPROXY
```

(Optional) **New in Fabric 2.0.** This section is used to configure a set of paths where your chaincode builders reside. Each external builder definition must include a name (used for logging) and the path to parent of the `bin` directory containing the builder scripts. Also, you can optionally specify a list of environment variable names to propagate from the peer when it invokes the external builder scripts. For details see [Configuring external builders and launchers](../cc_launcher.html).

- **`externalBuilders.path:`** Specify the path to the builder.
- **`externalBuilders.name:`** Give this builder a name.
- **`externalBuilders.propagateEnvironment:`** Specify the list of environment variables that you want to propagate to your peer.

## ledger

```
ledger:

  state:
    # stateDatabase - options are "goleveldb", "CouchDB"
    # goleveldb - default state database stored in goleveldb.
    # CouchDB - store state database in CouchDB
    stateDatabase: goleveldb

    couchDBConfig:
       # It is recommended to run CouchDB on the same server as the peer, and
       # not map the CouchDB container port to a server port in docker-compose.
       # Otherwise proper security must be provided on the connection between
       # CouchDB client (on the peer) and server.
       couchDBAddress: 127.0.0.1:5984

       # This username must have read and write authority on CouchDB
       username:

       # The password is recommended to pass as an environment variable
       # during start up (eg CORE_LEDGER_STATE_COUCHDBCONFIG_PASSWORD).
       # If it is stored here, the file must be access control protected
       # to prevent unintended users from discovering the password.
       password:

       # CacheSize denotes the maximum mega bytes (MB) to be allocated for the in-memory state
       # cache. Note that CacheSize needs to be a multiple of 32 MB. If it is not a multiple
       # of 32 MB, the peer would round the size to the next multiple of 32 MB.
       # To disable the cache, 0 MB needs to be assigned to the cacheSize.
       cacheSize: 64
```

This section is used to select your ledger database type, either `goleveldb` `CouchDB`. To avoid errors all peers should use the same database **type**.  CouchDB is an appropriate choice when ledger states are structured as JSON documents because CouchDB supports rich queries and update of richer data types often found in business transactions. While CouchDB runs in a separate operating system process, there is still a 1:1 relation between a peer node and a CouchDB instance, meaning that each peer will have a single database and that database will only be associated with that peer. The choice of the database is invisible to a smart contract.

- **`ledger.state.stateDatabase:`** (Override this value when you plan to use CouchDB.) Defaults to goleveldb which is appropriate when ledger states are simple key-value pairs. A LevelDB database is co-located with the peer node – it is embedded within the same operating system process.

- **`ledger.state.couchDBConfig.couchDBAddress:`** (Required when using CouchDB.) Specify the address and port where CouchDB is running.

- **`ledger.state.couchDBConfig.username:`** (Required when using CouchDB.) Specify the CouchDB user with read and write authority to the database.

- **`ledger.state.couchDBConfig.password:`** (Required when using CouchDB.) Best practice is to specify an environment variable that represents the password for the CouchDB user with read and write authority to the database.

- **`ledger.state.couchDBConfig.cacheSize:`** (Recommended to override) **New in Fabric 2.0.** When using CouchDB as the state database, read delays during endorsement and validation phases have historically been a performance bottleneck. In Fabric v2.0, a new peer cache replaces many of these expensive lookups with fast local cache reads. The cache size can be configured by using this parameter.

## operations
```
operations:
    # host and port for the operations server
    listenAddress: 127.0.0.1:9443

    # TLS configuration for the operations endpoint
    tls:
        # TLS enabled
        enabled: false

        # path to PEM encoded server certificate for the operations server
        cert:
            file:

        # path to PEM encoded server key for the operations server
        key:
            file:

        # most operations service endpoints require client authentication when TLS
        # is enabled. clientAuthRequired requires client certificate authentication
        # at the TLS layer to access all resources.
        clientAuthRequired: false

        # paths to PEM encoded ca certificates to trust for client authentication
        clientRootCAs:
            files: []
```

The operations service is used for monitoring the health of the peer and relies on mutual TLS to secure its communication. Therefore, you need to set `operations.tls.clientAuthRequired` to `true`. When this parameter is set to `true`, clients attempting to ascertain the health of the node are required to provide a valid certificate for authentication. If the client does not provide a certificate or the service cannot verify the client’s certificate, the request is rejected. This means that the clients will need to register with the peer's TLS CA and provide their TLS signing certificate on the requests. See [The Operations Service](../operations_service.html) to learn more.

If you plan to use Prometheus [metrics](#metrics) to monitor your peer, you must configure the operations service here.

In the unlikely case where two peers are running on the same node, you need to modify the `listenAddress`: for the second peer to use a different port. Otherwise, when you start the second peer, it will fail to start, reporting that the bind address is already in use.

- **`operations.listenAddress:`** (Required when using the operations service.) Specify the address and port of the operations server.
- **`operations.tls.cert.file*:`** (Required when using the operations service). Can be the same file as the `peer.tls.cert.file`.
- **`operations.tls.key.file*:`** (Required when using the operations service). Can be the same file as the `peer.tls.key.file`.
- **`operations.tls.clientAuthRequired*:`** (Required when using the operations service). Must be set tp `true` to enable mutual TLS between the client and the server.
- **`operations.tls.clientRootCAs.files*:`** (Required when using the operations service). Similar to the [peer.tls.clientRootCAs.files](#tls), it contains a list of client root CA certificates that can be used to verify client certificates. If the client enrolled with the peer organization CA, then this value is the peer organization root CA cert.

## metrics

```
metrics:
    # metrics provider is one of statsd, prometheus, or disabled
    provider: disabled
    # statsd configuration
    statsd:
        # network type: tcp or udp
        network: udp

        # statsd server address
        address: 127.0.0.1:8125
```
By default this is disabled, but if you want to monitor the metrics for the peer, you need to choose either `statsd` or `Prometheus` as your metric provider. `Statsd` uses a "push" model, pushing metrics from the peer to a `statsd` endpoint. Because of this, it does not require configuration of the operations service itself. See the list of [Available metrics for the peer](../metrics_reference.html#peer-metrics).

- **`provider`:** (Required to use `statsd` or `Prometheus` metrics for the peer.) Because Prometheus utilizes a "pull" model there is not any configuration required from the Fabric CA server side. Rather, Prometheus will sends requests to the operations URL to poll for available metrics.
- **`address:`** (Required when using `statsd`.) When `statsd` is enabled, you will need to configure the hostname and port of the server.

## Next steps

After deciding on your peer configuration, you are ready to deploy your peers. Follow instructions in the [Deploy the peer](./peerdeploy.html) topic for instructions on how to deploy your peer.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
