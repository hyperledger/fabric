# Certificates Management Guide

**Audience**: Hyperledger Fabric network admins

This guide provides overview information and details for a network administrator to manage certificates (certs) in Hyperledger Fabric.

## Prerequisites and Resources

The following Fabric documentation resources on identities, Membership Service Providers (MSPs) and Certificate Authorities (CAs) provide context for understanding certificate management:

* [Identity](./identity/identity.html#identity)
* [MSP Overview](./membership/membership.html)
* [MSP Configuration](./msp.html)
* [Registration and Enrollment](https://hyperledger-fabric-ca.readthedocs.io/en/latest/deployguide/use_CA.html#overview-of-registration-and-enrollment)
* [Registering an Identity](https://hyperledger-fabric-ca.readthedocs.io/en/latest/deployguide/use_CA.html#register-an-identity)
* [Enrolling an Identity](https://hyperledger-fabric-ca.readthedocs.io/en/latest/deployguide/use_CA.html#enroll-an-identity)


## Key Concepts

**Registration** – A username and password pair, stored in the Certificate Authority (CA). This registration is created by a CA admin user, has no expiration, and contains any required roles and attributes.

**Enrollment** – A public/private key pair and an X.509 certificate issued by the organization's Certificate Authority (CA). The certificate encodes roles, attributes, and metadata, which represent an identity in a Fabric network. An enrollment is associated with a CA registration by username and password.

**Identity** - A public certificate and its private key used for encryption. The public certificate is the X.509 certificate issued by the CA, while the private key is stored out of band, on a secure storage.

**TLS** - A public Transport Layer Security (TLS) Certificate that authorizes client and node communications. On Fabric, registration and enrollment are the same for X.509 Certificates and TLS Certificates.


## Certificate Types

Hyperledger Fabric implements two types of certificates: 1) **Enrollment** Certificates for identities and 2) **TLS** Certificates for node and client communications.

### Enrollment Certificates

Enrollment Certificates are classed into four types:

* **Admin**
* **Peer**
* **Orderer**
* **Client**

Each Enrollment Certificate type has a specific role:

**Admin:** X.509 Certificates used to authenticate admin identities, which are required to make changes to  Fabric configurations.

**Peer:** X.509 Certificates used to enroll peer nodes, located physically on the node or mapped to the node. For a Fabric peer node to start, it must have a valid Enrollment Certificate with the required attributes.

**Orderer:** X.509 Certificates used to enroll orderer nodes, located physically on the node or mapped to the node. For a Fabric orderer node to start, it must have a valid Enrollment Certificate with the required attributes.

**Client:** X.509 Certificates that allow signed requests to be passed from clients to Fabric nodes. Client certs define the identities of client applications submitting transactions to a Fabric network.


### TLS Certificates

TLS Certificates allow Fabric nodes and clients to sign and encrypt communications. A valid TLS Certificate is required for any channel communication.

### Certificate Expiration

Enrollment and TLS Certificates are assigned an expiration date by the issuing Certificate Authority (CA).  Expiration dates must be monitored, and certificates must be re-enrolled before expiration. The most important certificate parameter is the **Not After** element, which indicates its expiration date.


## Certificates and Locations

Organization CAs supply X.509 Enrollment Certificates for identities and the TLS CAs supply TLS Certificates for securing node and client communications.


### Organization CA Certificates

Organization CA Root Certificates and Organization CA Admin Certificates provide authorization to interact with the certificate authority for the organization, as described below.

#### Organization CA Root Certificate

**Description**: Public Certificate that permits verification of all certificates issued by the Organization CA. Organization CA Root Certificates are self-signed certificates if creating a new Certificate Authority (CA), or provided by an external CA.

**Location**: Stored on disk in the Organization CA directory (ca-cert.pem), and copied into the channel configuration to verify identifies for the organization.

**Impact if expired**: A new Organization CA Root Certificate must be issued. Organization CA Root Certificates are valid for 15 years.


#### Organization CA Admin Certificate

**Description**: Certificate used when making admin requests to the Organization CA.

**Location**: Dependent on implementation:

<blockquote>**Note**: Each identity has a local **msp** directory structure which contains its certificate in the **signcerts** directory and its private key in the **keystore** directory. For details on the **msp** directory, refer to [MSP Structure](https://hyperledger-fabric.readthedocs.io/en/latest/membership/membership.html#msp-structure).</blockquote>

<pre>
msp
 ├── IssuerPublicKey
 ├── IssuerRevocationPublicKey
 ├── cacerts
 │   └── localhost-7053.pem
 ├── keystore
 │   └── key.pem
 ├── signcerts
 │   └── cert.pem
 └── user
</pre>

**Impact if expired**: The Organization Administrator cannot register new identities with the CA, but transaction traffic does not stop.

[Reference - Enroll Orderer Org CA Admin](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-orderer-org-s-ca-admin)


### TLS CA Certificates

TLS CA Root Certificates and TLS CA Admin Certificates provide authorization to interact with the certificate authority for the TLS, as described below.


#### TLS CA Root Certificate

**Description**: Public certificate that permits verification of all certificates issued by the TLS CA. TLS CA Root Certificates are self-signed certificates if creating a new Certificate Authority (CA), or provided by an external CA.

**Location**: Stored on disk in the TLS CA directory (ca-cert.pem), and copied into the channel configuration to verify TLS Certificates for the organization.

**Impact if expired**: A new TLS CA Root Certificate must be issued. TLS CA Root Certificates are valid for 15 years.


#### TLS CA Admin Certificate

**Description**: Certificate used for admin requests to the TLS CA.

**Location**: Dependent on implementation:

<pre>
msp
 ├── IssuerPublicKey
 ├── IssuerRevocationPublicKey
 ├── cacerts
 │   └── localhost-7053.pem
 ├── keystore
 │   └── key.pem
 ├── signcerts
 │   └── cert.pem
 └── user
</pre>

**Impact if expired**: The Fabric Administrator will no longer be able to register TLS certificates in the TLS CA for nodes in the network.

[Reference - Enroll TLS CA Admin](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-tls-ca-s-admin)


### Peer Certificates

A Peer Enrollment Certificate and a Peer TLS Certificate are issued for each peer in an organization.

#### Peer Enrollment Certificate

**Description**: Authenticates the identity of the peer node when endorsing transactions.

**Location**: Dependent on implementation:

<pre>
org1ca
└── peer1
    ├── msp
    │    ├── admincerts
    │    │   └── cert.pem
    │    ├── cacerts
    │    │   └── localhost-7053.pem
    │    ├── keystore
    │    │   └── key.pem
    │    ├── signcerts
    │    │   └── cert.pem
    │    └── user
    |── tls
</pre>

**Impact if expired**: Production outage. Peers do not start without a valid Enrollment Certificate.

[Reference - Enroll peer](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-peer1)


#### Peer TLS Certificate

**Description**: Authenticates node component communication on the channel.

**Location**: Dependent on implementation:

<pre>
org1ca/
└── peer1
    ├── msp
    └── tls
        ├── cacerts
        ├── keystore
        │   └── key.pem
        ├── signcerts
        │   └── cert.pem
        ├── tlscacerts
        │   └── tls-localhost-7053.pem
        └── user
</pre>

**Impact if expired**: Production outage. No communication to the peer is possible.

[Reference - Enroll peer](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-peer1)


### Orderer Certificates

Orderer Enrollment Certificates and Orderer TLS Certificates are issued for each ordering service node in an organization.

#### Orderer Enrollment Certificate

**Description**: The public key that the orderer uses to sign blocks.

**Location**: Dependent on implementation:

<pre>
 └── orderer1
     ├── msp
     │   ├── admincerts
     │   │   └── cert.pem
     │   ├── cacerts
     │   │   └── localhost-7053.pem
     │   ├── keystore
     │   │   └── key.pem
     │   ├── signcerts
     │   │   └── cert.pem
     │   |── user
     └── tls
</pre>

**Impact if expired**: Production outage. Orderers do not start without a valid Enrollment Certificate.

[Reference - Enroll orderer](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-orderer)


#### Orderer TLS Certificate

**Description**: TLS Certificate for the ordering node communication.

**Location**: Dependent on implementation:

<pre>
ordererca/
└── orderer1
    ├── msp
    └── tls
        ├── cacerts
        ├── keystore
        |   └── key.pem
        ├── signcerts
        │   └── cert.pem
        ├── tlscacerts
        │   └── tls-localhost-7053.pem
        └── user
  </pre>

**Impact if expired**: Production outage. Ordering nodes are no longer allowed to participate in cluster.

[Reference - Enroll orderer](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-orderer)


### Admin Certificates

Ordering Service Organization Channel Admin Certificates and Peer Service Organization Channel Admin Certificates are issued for each organization.

#### Ordering Service Organization Channel Admin Certificate

**Description**: Certificate for an organization administrator to manage ordering service and channel updates.

**Location**: Dependent on implementation:

<pre>
ordererca/
└── ordereradmin
└── msp
    ├── admincerts
    │   └── cert.pem
    ├── cacerts
    │   └── localhost-7053.pem
    ├── keystore
    │   └── key.pem
    ├── signcerts
    │   └── cert.pem
    └── user
</pre>

**Impact if expired**: Transactions can continue to work successfully. Cannot modify channels from a client application or manage the orderer from the console.

[Reference - Enroll Org Admin](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-org0-s-admin)


#### Peer Service Organization Channel Admin Certificate

**Description** - Certificate for an organization administrator to manage a peer, including channel and chaincode services.

**Location** - Dependent on implementation:

<pre>
org1ca/
└── org1admin
└── msp
├── admincerts
│     └── cert.pem
├── cacerts
│     └── localhost-7053.pem
├── keystore
│     └── key.pem
├── signcerts
│     └── cert.pem
└── user
</pre>

**Impact if expired**: Transactions can continue to work successfully. Cannot install new smart contracts from a client application or manage the peer from the console.

[Reference - Enroll Org Admin](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-org1-s-admin)


### Client Certificates

**Description**: Two types of Client Certificates are issued for each organization:

1. **Organization Enrollment Certificate** - Authenticates the client identity for interactions with peers and orderers.
2. **TLS Certificate** - Authenticates client communications, and only required if mutual TLS is configured.

Client Certificates expire after one year, using the Hyperledger Fabric CA default settings. Client Certificates can be re-enrolled using either command line Hyperledger Fabric CA utilities or the Fabric CA client SDK.

**Impact if expired**: Client Certificates must be re-enrolled before expiration or the client application will not be able to interact with the Fabric nodes.

[Reference - Re-enroll user](https://hyperledger.github.io/fabric-sdk-node/release-2.2/FabricCAClient.html#reenroll__anchor)


### Certificate Decoding

X.509 Certificates are created by an enrollment of the certificate, based on its registration. The X.509 Certificate contains metadata describing its purpose and identifying the parent CA. The cert expiration is specified in the **Not After** field.

The certificate details can be decoded using the OpenSSL utility:

```
# openssl x509 -in cert.pem -text -noout
```

The following example shows a decoded certificate:

```
Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number:
            47:4d:5d:f6:db:92:6b:54:98:8d:9c:44:0c:ad:b6:77:c5:de:d2:ed
        Signature Algorithm: ecdsa-with-SHA256
        Issuer: C = US, ST = North Carolina, O = Hyperledger, OU = Fabric, CN = orderer1ca
        Validity
            Not Before: Feb  4 14:55:00 2022 GMT
            Not After : Feb  4 15:51:00 2023 GMT
        Subject: C = US, ST = North Carolina, O = Hyperledger, OU = orderer, CN = orderer1
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
                Public-Key: (256 bit)
                pub:
                    04:29:ec:d5:53:3e:03:9d:64:a4:a4:28:a5:fe:12:
                    e2:f0:dd:e4:ee:b9:3f:3e:01:b2:3a:d4:68:b1:b2:
                    4f:82:1a:3a:33:db:92:6d:10:c9:c2:3b:3d:fc:7a:
                    f0:fa:cc:8b:44:e8:03:cb:a1:6e:eb:b3:6c:05:a2:
                    f8:fc:3c:af:24
                ASN1 OID: prime256v1
                NIST CURVE: P-256
        X509v3 extensions:
            X509v3 Key Usage: critical
                Digital Signature
            X509v3 Basic Constraints: critical
                CA:FALSE
            X509v3 Subject Key Identifier:
                63:97:F5:CA:BB:B7:4B:26:84:D9:65:40:E3:43:14:A4:7B:EE:79:FF
            X509v3 Authority Key Identifier:
                keyid:BA:2A:F8:EA:A5:7D:DF:1D:0F:CF:47:37:41:82:03:7E:04:61:D0:D8
            X509v3 Subject Alternative Name:
                DNS:server1.testorg.com
            1.2.3.4.5.6.7.8.1:
                {"attrs":{"hf.Affiliation":"","hf.EnrollmentID":"orderer1","hf.Type":"orderer"}}
    Signature Algorithm: ecdsa-with-SHA256
         30:45:02:21:00:e1:93:f6:3c:08:f2:b9:fb:06:c9:02:d0:cf:
         e1:a6:23:a3:05:78:10:d9:41:2c:1e:2c:91:80:fd:52:ad:62:
         9c:02:20:51:33:42:5e:a0:8a:2a:ec:f5:83:46:f0:99:6a:7e:
         eb:a8:97:1f:30:99:9d:ae:8d:ef:36:07:da:bb:67:ed:80
```

## Certificate Renewal

All nodes and clients should have their enrollment and TLS certificates renewed before expiring to avoid disruption of service.
Use the Fabric CA enroll or reenroll function to get updated certificates from the issuing Fabric Certificate Authority (CA).
The reenroll function allows the reuse of an existing private key, which is especially important for orderer TLS certificates.

### Renewal of Expired Certificates

Certificates can still be renewed by using the reenroll function after they expire, but Fabric CA must be at version v1.5.5 or above, and must be configured to allow it.
Enable the `reenrollignorecertexpiry` option on the Fabric CA server prior to reenrollment by adding the `reenrollignorecertexpiry` parameter to the fabric-ca-server-config.yaml, as follows:

```
ca:
  certfile: /crypto/tlsca/cert.pem
  chainfile: /crypto/tlsca/chain.pem
  keyfile: /crypto/tlsca/key.pem
  name: tlsca
  reenrollignorecertexpiry: true
```

Alternatively, `reenrollignorecertexpiry` can be also be set with an environment variable or Fabric CA server startup flag:

* Set the FABRIC_CA_SERVER_CA_REENROLLIGNORECERTEXPIRY environment variable to true at CA startup
* Start the CA with the flag --ca.reenrollignorecertexpiry=true

Restart the Fabric CA servers.

### Renew Peer Certificates

For peer enrollment certificates and TLS certificates you can use either the `enroll` or `reenroll` function from the Fabric CA client. Use `reenroll` if you would like to reuse a private key by passing the `csr.keyrequest.reusekey` option and indicating the `mspdir` location of the existing private key (the private key is found in the <mspdir>/keystore directory):

```
fabric-ca-client enroll --<OTHER-OPTIONS>

fabric-ca-client reenroll --csr.keyrequest.reusekey --mspdir <LOCATION-OF-IDENTITY'S-MSP-DIRECTORY-THAT-CONTAINS-THE-EXISTING-PRIVATE-KEY> --<OTHER-OPTIONS>
```

Replace the peer's enrollment certificate or TLS certificate.

The peer's enrollment certificate is configured to be in the peer's `<peer.mspConfigPath>/signcerts` directory (if private key was not reused the new private key should also be updated in the `<peer.mspConfigPath>/keystore` directory).

The peer's TLS certificate is configured to be in the peer's `<peer.tls.cert.file>` directory/file (if private key was not reused the new private key should also be updated in the `<peer.tls.key.file>`).

Restart the peer.

### Renew Orderer Certificates

For orderer enrollment certificates you can also use the `enroll` or `reenroll` function, using the steps described above for peers.

Orderer TLS certificates must be reenrolled with the option to reuse the private key. This is due to the orderer TLS certificates being configured in the system and application channels (orderer to orderer communication is verified using the public key in the configured TLS certificate). As of v1.4.9 and v2.2.1, orderer nodes verify a matching key rather than the entire configured TLS certificate, enabling orderer TLS certificate renewal without requiring a channel configuration update to update the certificate.

Use `reenroll` and reuse the TLS certificate's private key by passing the `csr.keyrequest.reusekey` option and indicating the `mspdir` location of the existing private key (the private key is found in the `<mspdir>/keystore` directory):

```
fabric-ca-client reenroll --csr.keyrequest.reusekey --mspdir <LOCATION-OF-IDENTITY'S-MSP-DIRECTORY-THAT-CONTAINS-THE-EXISTING-PRIVATE-KEY> --<OTHER-OPTIONS>`
```

Replace the orderer's enrollment certificate or TLS certificate.

The orderer's enrollment certificate is configured to be in the orderer's `<General.LocalMSPDir>/signcerts` directory (if private key was not reused the new private key should also be updated in the `<General.LocalMSPDir>/keystore` directory).

The orderer's TLS certificate is configured to be in the orderer's `<General.TLS.Certificate>` directory/file (don't update the private key since it was reused).

Restart the orderer node.

### Renew Orderer TLS Certificates Without Reusing the Private Key

While it is recommended to reuse the private key for orderer TLS certificate renewal, this may not be possible in all situations. Additional steps and planning are required since each ordering service channel (including the system channel, if used) must be updated for the new orderer TLS certificate.

Suppose you do not reuse the orderer TLS private key during re-enrollment, and the original orderer TLS certificates have not yet expired. In that case, you must update the orderer TLS certificates one at a time on each node and in each channel configuration and then verify the orderer function before moving on to other orderer node TLS certificate updates.

If you do not reuse the orderer TLS private key and the original orderer TLS certificates have expired, the ordering service will not be able to form consensus and will therefore not be able to process transactions including channel configuration updates. The ordering service recovery process is complicated as you must temporarily utilize the orderer `TLSHandshakeTimeShift` property on all nodes and restart them in order to form consensus with the expired certificates so that channel configuration updates can be processed to update the TLS certificates. Update a majority of the orderer TLS certificates (e.g. 3 out of 5) one at a time on each node and in the channel configuration. Once a majority have been updated the new certificate expiration dates will no longer fall within the `TLSHandshakeTimeShift` and therefore the updated ordering nodes will fall out of the consenter set causing loss of consensus again. Next, remove the `TLSHandshakeTimeShift` setting from all nodes. Upon restart the majority of updated orderer nodes will now form consensus and you can then update the remaining orderer TLS certificates (e.g. 2 out of 5) one at a time on each node and in the channel configuration.
