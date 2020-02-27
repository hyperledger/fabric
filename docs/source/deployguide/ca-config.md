#  Checklist for a production CA server

As you prepare to build a production Fabric CA server, you need to consider configuration of the following fields in the fabric-ca-server-config.yaml file.
When you [initialize](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/users-guide.html#initializing-the-server) the CA server, this file is generated for you so that you can customize it before actually starting the server.

This checklist covers key configuration parameters for setting up a production network. Of course you can always refer to the [fabric-ca-server-config.yaml
file](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/serverconfig.html) for additional parameters or more information. The list of parameters in this topic includes:

- [ca](#ca)
- [tls](#tls)
- [cafiles](#cafiles)
- [intermediate ca](#intermediate-ca)
- [port](#port)
- [user registry](#user-registry)
- [registry database](#registry-database)
- [LDAP](#ldap)
- [affiliations](#affiliations)
- [csr](#csr)
- [signing](#signing)
- [bccsp](#bccsp)
- [cors](#cors)
- [cfg](#cfg)
- [operations](#operations)
- [metrics](#metrics)

## ca

```
ca:
  # Name of this CA
  name:
  # Key file (is only used to import a private key into BCCSP)
  keyfile:
  # Certificate file (default: ca-cert.pem)
  certfile:
  # Chain file
  chainfile:
```

Start by giving your CA a name. Often the name indicates the organization that this CA will serve. Or, if this is a TLS CA you might want to indicate that in the name. This `ca.name` is used when targeting this server for requests by the Fabric CA Client `--caname` parameter.

## TLS

```
tls:
  # Enable TLS (default: false)
  enabled: true
  # TLS for the server's listening port
  certfile:
  keyfile:
  clientauth:
    type: noclientcert
    certfiles:
```

Configure this section to enable TLS communications for the organization.

- **`tls.enabled`**: For a secure production environment, TLS should be enabled for secure communications between nodes by setting `enabled: true` in the `tls` section of the config file. (Note that it is disabled by default which may be acceptable for a test network but for production it needs to be enabled.) This setting will configure `server-side TLS`, meaning that TLS will guarantee the identity of the _server_ to the client and provides a two-way encrypted channel between them.

- **`tls.certfile`**: Every node in the organization needs to enroll with the TLS CA for the organization before it can transact securely with other nodes in the organization. Therefore, before you can deploy an enrollment CA or an intermediate CA, you must first enroll an admin identity with the TLS CA to get the TLS signing certificate. Provide the name and location of the generated TLS signing certificate. For example: `/msp/signcerts/cert.pem`. If this is a root TLS CA, this field can be blank.

- **`tls.certfile`**: Similar to the `tls.certfile`, provide the name and location of the generated TLS private key for this CA. For example: `/msp/keystore/87bf5eff47d33b13d7aee81032b0e8e1e0ffc7a6571400493a7c_sk`. If you are using an HSM or if this is a root TLS CA, this field will be blank.

**Note:** In order to use the Fabric [Service Discovery](../discovery-overview.html) and [Private Data](../private-data-arch.html) features, `server-side TLS` needs to be enabled.

If server-side TLS is sufficient for your needs, you are done with this section.

- **`tls.clientauth.type`**: If the server additionally needs to authenticate the identity of the _client_, then **mutual TLS** (mTLS) is required. When mTLS is configured, the client is required to send its certificate during a TLS handshake. To configure your CA for mTLS, set the `clientauth.type` to `RequireAndVerifyClientCert`.

- **`tls.clientauth.certfiles`**: For mTLS only, provide the PEM-encoded list of root certificate authorities that the server uses when verifying client certificates.

## cafiles

```
cafiles:
```

As mentioned in the topic on [Planning for a CA](ca-deploy-topology.html), the `cafiles` parameter can be used to configure a dual-headed CA -- a single CA that under the covers includes both a root CA and a TLS CA. This usage pattern can be for convenience, allowing each CA to maintain its own configuration but still share the same back-end user database. In the `cafiles` parameter, enter the path to the `fabric-ca-server-config.yaml` of the second CA server, for example the TLS CA. The configuration of the secondary CA can contain all of the same elements as are found in the primary CA server config file except `port`, `ca.name`, and `tls` sections.

If this is not a desired configuration for your CA, you can leave the value of this parameter blank.

## intermediate CA
```
intermediate:
  parentserver:
    url:
    caname:

  enrollment:
    hosts:
    profile:
    label:

  tls:
    certfiles:
    client:
      certfile:
      keyfile:
```

Intermediate CAs are not required, but to reduce the risk of your root CA becoming compromised, you may want to include one or more intermediate CAs in your network.

**Important:** Before setting up an intermediate CA, you need to verify that the `csr.ca.pathlength` parameter in the parent CA, is at least `1`. When the `pathlength` is set to `1`, the root CA can issue intermediate CA certificates, but these intermediate CAs may not in turn enroll other intermediate CAs. If you want your intermediate CA to be able to enroll other intermediate CAs, the `pathlength` needs to be set to `2`. If the intermediate CA _under_ this intermediate CA will be able to enroll other intermediate CAs, then the pathlength of the root CA would need to be `3`.

- **`parentserver.url`**: Specify the parent server url in the format `https://<PARENT-CA-ENROLL-ID>:<PARENT-CA-SECRET>@<PARENT-CA-CSR-HOST>:<PARENT-CA-PORT>`.
- **`parentserver.caname`**: Enter `ca.name` of the parent CA server.
- **`enrollment.profile:`**: Enter the value of the `signing.profile` for the parent CA. Normally this would be `ca` unless this is an intermediate TLS CA server.
- **`tls.certfiles`**: Enter the location and name of the parent CA ca-cert.pem file. For example, `tls/ca-cert.pem`. This location is relative to where the configuration .yaml file exists.

In addition to editing this `intermediate` section, you also need to edit the following sections of the configuration .yaml file for this intermediate CA:
- `csr` - Ensure that the `csr.cn` field is blank.
- `port` - Be sure to set a unique port for the intermediate CA.

## port
```
port:
```

Each CA must be running on its own unique port and obviously must not conflict with any other service running on that port. You need to decide what ports you want to use for your CAs ahead of time and configure that port in the .yaml file.

## user registry
```
registry:
  # Maximum number of times a password/secret can be reused for enrollment
  # (default: -1, which means there is no limit)
  maxenrollments: -1

  # Contains identity information which is used when LDAP is disabled
  identities:
     - name: <<<adminUserName>>>
       pass: <<<adminPassword>>>
       type: client
       affiliation: ""
       attrs:
          hf.Registrar.Roles: "*"
          hf.Registrar.DelegateRoles: "*"
          hf.Revoker: true
          hf.IntermediateCA: true
          hf.GenCRL: true
          hf.Registrar.Attributes: "*"
          hf.AffiliationMgr: true
```
Before a certificate can be generated for an identity, the associated user must be registered. If you are not using an LDAP user registry, then this section along with the associated registry database `db:` section are required.

* `maxenrollments`: Used to restrict the number of times certificates can be generated for a user.
* `identities`: This section is used to define the attributes for any user that is being registered from the Fabric CA Client CLI. If no attributes are specified on the CLI request, then these attributes are used.
* `identities.type`: For Fabric, the list of valid types is `client`, `peer`, `admin`, `orderer`, and `member`.
* `affiliation`: Select the affiliation to be associated with this user. The list of possible affiliations are defined in the `affiliations:` section.
* `attrs`: The list of roles included above would be for "admin" users, meaning they can register and enroll other users. If you are registering a non-admin user, you would not give them these permissions. The `hf` attributes associated with an identity affect that identity's ability to register other users. You should review the topic on [Registering a new identity](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/users-guide.html#registering-a-new-identity) to understand the patterns that are required.

When the user is subsequently "enrolled", the `type`, `affiliation`, and `attrs` are visible inside the user's signing certificate and are used by policies to enforce authorization. Recall that `enrollment` is a process whereby the Fabric CA issues a certificate key-pair, comprised of a sign-cert and a private key that forms the identity. The private and public key are first generated locally by the Fabric CA client, and the public key is then sent to the CA which returns an encoded certificate, the signing certificate.

After a user has been registered, you can use the [`Identity`](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/clientcli.html#identity-command) command to modify the properties of the user.

## registry database
```
db:
  type: sqlite3
  datasource: fabric-ca-server.db
  tls:
      enabled: false
      certfiles:
      client:
        certfile:
        keyfile:
```

The Fabric CA stores user identities, affiliations, credentials, and public certificates in a database. Use this section to specify the type of database to be used to store the CA data. Fabric supports three database **types**:

- `sqlite` (SQLite Version 3)
- `postgres` (PostgresSQL)
- `mysql` (MySQL)

If you are running the database in a cluster, you must choose `postgres` or `mysql` as the database type.

If LDAP is being used as the user registry, then this section is ignored.

## LDAP
```
ldap:
   # Enables or disables the LDAP client (default: false)
   # If this is set to true, the "registry" section is ignored.
   enabled: false
   # The URL of the LDAP server
   url: ldap://<adminDN>:<adminPassword>@<host>:<port>/<base>
   # TLS configuration for the client connection to the LDAP server
   tls:
      certfiles:
      client:
         certfile:
         keyfile:
   # Attribute related configuration for mapping from LDAP entries to Fabric CA attributes
   attribute:
      # 'names' is an array of strings containing the LDAP attribute names which are
      # requested from the LDAP server for an LDAP identity's entry
      names: ['uid','member']
      # The 'converters' section is used to convert an LDAP entry to the value of
      # a fabric CA attribute.
      # For example, the following converts an LDAP 'uid' attribute
      # whose value begins with 'revoker' to a fabric CA attribute
      # named "hf.Revoker" with a value of "true" (because the boolean expression
      # evaluates to true).
      #    converters:
      #       - name: hf.Revoker
      #         value: attr("uid") =~ "revoker*"
      converters:
         - name:
           value:
      # The 'maps' section contains named maps which may be referenced by the 'map'
      # function in the 'converters' section to map LDAP responses to arbitrary values.
      # For example, assume a user has an LDAP attribute named 'member' which has multiple
      # values which are each a distinguished name (i.e. a DN). For simplicity, assume the
      # values of the 'member' attribute are 'dn1', 'dn2', and 'dn3'.
      # Further assume the following configuration.
      #    converters:
      #       - name: hf.Registrar.Roles
      #         value: map(attr("member"),"groups")
      #    maps:
      #       groups:
      #          - name: dn1
      #            value: peer
      #          - name: dn2
      #            value: client
      # The value of the user's 'hf.Registrar.Roles' attribute is then computed to be
      # "peer,client,dn3".  This is because the value of 'attr("member")' is
      # "dn1,dn2,dn3", and the call to 'map' with a 2nd argument of
      # "group" replaces "dn1" with "peer" and "dn2" with "client".
      maps:
         groups:
            - name:
              value:
```   

Before a certificate can be generated for an identity, the associated user must be registered. You have the option of configuring a connection to an LDAP registry (in which case the `registry` section is ignored) or the user ids and passwords can be stored in a database.

When LDAP is configured, Fabric authenticates an identity against the LDAP user registry prior to generating the certificates for that user, otherwise known as `enrollment`. The LDAP server is also used to retrieve an identity's attribute values which are used later for authorization on the network. Additionally, these attributes are useful for making access control decisions in smart contracts.

## affiliations
```
affiliations:
   org1:
      - department1
      - department2
   org2:
      - department1
```      

Affiliations are useful to designate sub-departments for organizations. They are useful for policy definitions for example when you might want to have transactions endorsed by a peer who not simply a member of ORG1, but is a member of ORG1.MANUFACTURING. Note that the affiliation of the registrar must be equal to or a prefix of the affiliation of the identity being registered. If you are considering using affiliations you should review the topic on [Registering a new identity](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/users-guide.html#registering-a-new-identity) for requirements.

The default affiliations listed above are added to the Fabric CA database the first time the server is started. If you prefer not to have these affiliations on your server, you need to edit this config file and remove or replace them _before_ you start the server for the first time. Otherwise, you must use the Fabric CA client [Affiliation command](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/clientcli.html#affiliation-command) to modify the list of affiliations.  

## csr (certificate signing request)

```
csr:
   cn: <<<COMMONNAME>>>
   keyrequest:
     algo: ecdsa
     size: 256
   names:
      - C: US
        ST: "North Carolina"
        L:
        O: Hyperledger
        OU: Fabric
   hosts:
     - <<<MYHOST>>>
     - localhost
   ca:
      expiry: 131400h
      pathlength: <<<PATHLENGTH>>>
```
The CSR section controls the creation of the root CA certificate. Therefore, if you want to customize any values, it is recommended to configure this section before you start your server for the first time. The values you specify here will be included in the signing certificates that are generated. If you customize values for the CSR after you start the server, you need to delete the `ca.cert` file and `ca.key` files and then run the `fabric-ca-server init -b admin:adminpw` command again.

- **csr.cn**: This field must be set to the ID of the CA bootstrap identity and can be left blank. It defaults to the CA server bootstrap identity.
- **csr.keyrequest**: Use these values ff you want to customize the crypto [algorithm and key sizes](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/users-guide.html#initializing-the-server).
- **csr.names**: Specify the values you want to use for the certificate Issuer, visible in the signing certificate.
- **csr.expiry**: Specify when the this CA's root certificate expires. The default value `131400h` is 15 years.
- **csr.pathlength**: If you will have intermediate CAs that will issue certificates.  If there is one intermediate CA, the pathlength needs to be set to `2`. If there will be another intermediate CA _under_ this intermediate CA, then the pathlength of the root CA would need to be `3` and so on.

## signing

```
signing:
    default:
      usage:
        - digital signature
      expiry: 8760h
    profiles:
      ca:
         usage:
           - cert sign
           - crl sign
         expiry: 43800h
         caconstraint:
           isca: true
           maxpathlen: 0
      tls:
         usage:
            - signing
            - key encipherment
            - server auth
            - client auth
            - key agreement
         expiry: 8760h
```

The defaults in this section are normally sufficient for a production server. However, you might want to modify the default expiration of the generated enrollment CA and TLS certificates. Note that this is different than the `expiry` specified in the `csr` section for the CA root certificate.  

If this is a TLS CA, it is recommended that you remove the `ca` section from `profiles:` since a TLS CA should only be issuing TLS certificates.

If you plan to have an intermediate CA in addition to the root CA, then you must set  `maxpathlength` to `1` or higher in the configuration .yaml file for the root CA.

## bccsp

```
bccsp:
    default: SW
    sw:
        hash: SHA2
        security: 256
        filekeystore:
            # The directory used for the software file-based keystore
            keystore: msp/keystore
```

If you plan to use a [Hardware Security Module](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/users-guide.html#example) (HSM) you need to configure the `hash` and `security` of the HSM crypto library. When certificates are generated, the private key is generated by and stored in the HSM instead of the `msp/keystore` folder.


## cors
```
cors:
    enabled: false
    origins:
      - "*"
```

Cross-Origin Resource Sharing (CORS) can be configured to to use additional HTTP headers to tell browsers to give a web application running at one origin access to selected resources from a different origin.

## cfg
```
cfg:
  affiliations:
    allowremove: false
  identities:
    allowremove: false  
```

These two parameters are not listed in the sample configuration file, but are important to understand. With the default configuration set to false, you will be unable to remove affiliations or identities without a server restart. If you anticipate the need to remove affiliations or identities from your production environment without a server restart, both of these fields should be set to `true` before starting your server. Note that after the server is started, affiliations and identities can only be modified by using the Fabric CA client CLI commands.  

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

        # require client certificate authentication to access all resources
        clientAuthRequired: false

        # paths to PEM encoded ca certificates to trust for client authentication
        clientRootCAs:
            files: []
```

The operations service can be used for health monitoring of the CA and relies on mutual TLS for communication with the node.
Therefore, you need to set `operations.tls.clientAuthRequired` to `true`. When this is set to `true`, clients attempting to ascertain the health of the node are  required to provide a valid certificate for authentication. If the client does not provide a certificate or the service cannot verify the client’s certificate, the request is rejected. This means that the clients will need to register with the TLS CA and provide their TLS signing certificate on the requests.

In the case where two CAs are running on the same machine, you need to modify the `listenAddress:` for the second CA to use a different port. Otherwise, when you start the second CA, it will fail to start, reporting that `the bind address is already in use`.

## metrics

```
metrics:
    # statsd, prometheus, or disabled
    provider: disabled

    # statsd configuration
    statsd:
        # network type: tcp or udp
        network: udp

        # statsd server address
        address: 127.0.0.1:8125

        # the interval at which locally cached counters and gauges are pushsed
        # to statsd; timings are pushed immediately
        writeInterval: 10s

        # prefix is prepended to all emitted statsd merics
        prefix: server
```

If you want to monitor the metrics for the CA, choose your metrics provider:

- **provider**: `Statsd` is a push model, `Prometheus` is a pull model. Because Prometheus is a pull model there is not any configuration required from the Fabric CA server side. Rather, Prometheus sends requests to the operations URL to poll for metrics. [Available metrics](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/metrics_reference.html).

## Next steps

After deciding on your CA configuration, you are ready to deploy your CAs. Follow instructions in the Create a CA tutorial to start your CA.


<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
