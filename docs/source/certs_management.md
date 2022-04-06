## Certificate Management Guide

**Audience**: Hyperledger Fabric network admins

This guide provides overview information and details for a network admin to manage certificates (certs) in Hyperledger Fabric.

### Prerequisites and Resources

The following Fabric documentation resources on identities, Managed Service Providers (MSPs) and Certificate Authorities (CAs) provide context for understanding certificate management:

* [Identity in Hyperledger Fabric](https://hyperledger-fabric.readthedocs.io/en/latest/identity/identity.html#identity)
* [MSPs in Hyperledger Fabric](https://hyperledger-fabric.readthedocs.io/en/latest/membership/membership.html#membership-service-provider-msp)
* [Registration and Enrollment](https://hyperledger-fabric-ca.readthedocs.io/en/latest/deployguide/use_CA.html#overview-of-registration-and-enrollment)
* [Registering an Identity](https://hyperledger-fabric-ca.readthedocs.io/en/latest/deployguide/use_CA.html#register-an-identity)
* [Enrolling an Identity](https://hyperledger-fabric-ca.readthedocs.io/en/latest/deployguide/use_CA.html#enroll-an-identity)


### Key Concepts

**Registration** – A username (enrollment ID) and password pair, stored in the Certificate Authority (CA). This registration is created by a CA admin user, has no expiration, and contains any required roles and attributes.

**Enrollment** – An identity based on the registration username and password. The enrollment contains a public/private key pair and an X.509 certificate issued by the organization Certificate Authority (CA). The certificate encodes roles and attributes from the registration.

**Identity** - A public certificate and its private key used for encryption. The public certificate is the X.509 certificate from the CA, while the private key is issued and stored by the MSP.


### Certificate Types

Hyperledger Fabric implements two types of certificates: 1) **Enrollment** certificates for identities and 2) **TLS** certificates for node and client communications.

#### Enrollment Certificates

Enrollment certificates are classed into fours types:

* **Admin**
* **Peer**
* **Orderer**
* **Client**

Each enrollment certificate type has a specific role:

**Admin:** X.509 certificates used to authenticate admin identities, which are required to make changes to  Fabric configurations.

**Peer:** X.509 certificates used to enroll peer nodes, located physically on the node or mapped to the node. For a Fabric peer node to start, it must have a valid enrollment certificate with the required attributes.

**Orderer:** X.509 certificates used to enroll orderer nodes, located physically on the node or mapped to the node. For a Fabric orderer node to start, it must have a valid enrollment certificate with the required attributes.

**Client:** X.509 certificates that allow signed requests to be passed from clients to Fabric nodes. These requests require a signature for admin level certificates.


#### TLS Certificates

TLS certificates allow Fabric nodes and clients to sign and encrypt communications. A valid TLS Certificate is required for any channel communication.


### Enrollment Certificate Expiration

All Enrollment certificates are assigned an expiration date by the issuing Certificate Authority (CA).  Expiration dates must be monitored, and certificates must be re-enrolled before expiration. The most important Enrollment Certificate parameter is the **Not After** element, which indicates its expiration date.

## Certificates and Locations

Organization CAs supply X.509 Enrollment Certificates for identities and the TLS CAs supply TLS Certificates for securing node and client communications.


### Organization CA Certificates

Organization CA Root certificates and Organization CA Admin certificates are issued for each organization.

#### Organization CA Root Certificate

**Description**: Public certificate that permits verification of all certificates issued by the Organization CA.

**Location**: Stored on disk in the Organization CA directory, or in the channel configuration (ca-cert.pem).

**Impact if expired**: A new Organization CA Root certificate must be issued. Organization CA Root certificates are valid for 15 years.


#### Organization CA Admin Certificate

**Description**: Certificate used when making admin requests to the Organization CA.

**Location**: Dependent on implementation:

`
msp <br>
 ├── IssuerPublicKey <br>
 ├── IssuerRevocationPublicKey <br>
 ├── cacerts <br>
 │   └── localhost-7053.pem <br>
 ├── keystore <br>
 │   └── key.pem <br>
 ├── signcerts <br>
 │   └── cert.pem <br>
 └── user
 `

**Impact if expired**: Cannot register or enroll new identities, but transaction traffic does not stop.

[Reference](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-orderer-org-s-ca-admin)


### TLS CA Certificates

TLS CA Root certificates and TLS CA Admin Certificates are issued for each organization.


#### TLS CA Root Certificate

**Description**: Certificate used when making admin requests to the TLS CA.

**Location**: Stored on disk in the TLS CA directory or in the channel configuration (ca-cert.pem).

**Impact if expired**: A new TLS CA Root certificate must be issued. TLS CA Root certificates are valid for 15 years.


#### TLS CA Admin Certificate

**Description**: Certificate used for admin requests to the TLS CA.

**Location**: Dependent on implementation:

`
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
`

**Impact if expired**: Cannot register or enroll new identities, but transaction traffic does not stop.

[Reference](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-tls-ca-s-admin)



### Peer Certificates

Peer Enrollment certificates and Peer TLS certificates are issued for each organization.

#### Peer Enrollment Certificate

**Description**: Authenticates the identity of the peer node when endorsing transactions.

**Location**: Dependent on implementation:

`
org1ca
├── msp
│    ├── cacerts
│    │   └── localhost-7053.pem
│    ├── keystore
│    │   └── key.pem
│    ├── signcerts
│    │   └── cert.pem
│    |── user
|    |   └── tls
`

**Impact if expired**: Production outage. Peers do not start without a valid Enrollment certificate.

[Reference](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-peer1)


#### Peer TLS Certificate

**Description**: Authenticates node component communication on the channel.

**Location**: Dependent on implementation:

`
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
`

**Impact if expired**: Production outage. No communication to the peer is possible.

[Reference](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-peer1)


### Orderer Certificates

Orderer Enrollment certificates and Orderer TLS certificates are issued for each organization.

#### Orderer Enrollment Certificate

**Description**: The public key that the orderer uses to sign transactions.

**Location**: Dependent on implementation:

`
ordererca/
└── orderer1
├── msp
│   ├── cacerts
│   │   └── localhost-7053.pem
│   ├── keystore
│   │   └── key.pem
│   ├── signcerts
│   │   └── cert.pem
│   |── user
|   |    └── tls
`

**Impact if expired**: Production outage. Orderers do not start without a valid Enrollment certificate.

[Reference](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-orderer)


#### Orderer TLS Certificate

**Description**: TLS certificate for the ordering node.

**Location**: Dependent on implementation:

`
ordererca/
└── orderer1
├── msp
└── tls
    ├── cacerts
    ├── keystore
        └── key.pem
    ├── signcerts
    │   └── cert.pem
    ├── tlscacerts
    │   └── tls-localhost-7053.pem
    └── user
  `

**Impact if expired**: Production outage. Ordering nodes are no longer allowed to participate in cluster.

[Reference](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-orderer)


### Admin Certificates

Ordering Service Organization Channel Admin certificates and Peer Service Organization Channel Admin certificates are issued for each organization.

#### Ordering Service Organization Channel Admin Certificate

**Description**: Admin certificate used to administer the ordering service and submit or approve channel updates. The associated admin identity is enrolled with the Organization CA before the node and Organization MSP are created.

**Location**: Dependent on implementation:

`
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
`

**Impact if expired**: Transactions can continue to work successfully. Cannot modify channels from a client application or manage the orderer from the console.

[Reference](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-org0-s-admin)


#### Peer Service Organization Channel Admin Certificate

**Description** - Admin certificate used to administer the peer and install smart contracts on the peer. Can also be configured to manage the application channel that the peer belongs to. The associated admin user is registered with the Organization CA before the peer and Organization MSP are created.

**Location** - Dependent on implementation:

`
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
`

**Impact if expired**: Transactions can continue to work successfully. Cannot install new smart contracts from a client application or manage the peer from the console.

[Reference](https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-org1-s-admin)



### Client Certificates

**Description**: Three types of Client certificates are issued for each organization:

1. Certificates used to authenticate an identity to the Gateway when submitting transactions
2. Client-side TLS certificates used to establish a secure network link to a Gateway peer
3. Certificate for a client SDK user if no gateway is accessed.

Both the client and the CA have to sign a client certificate for it to be valid.

Client certificates expire after one year, using the Hyperledger Fabric CA default settings. Client certificates can be re-enrolled using either command line Hyperledger Fabric CA utilities or the Fabric CA client SDK.

**Impact if expired**: Client certificates must be re-enrolled before expiration or the client application will not be able to connect to the Fabric nodes.

[Reference](https://hyperledger.github.io/fabric-sdk-node/release-2.2/FabricCAClient.html#reenroll__anchor)


### Certificate Decoding

X.509 certificates are distinct from registrations, which do not have an expiration. X.509 certificates are created based on the registration and contain information from the CA. This information includes metadata indicating the parent CA and encoded information describing the purpose of the certificate.

Certificate details can be decoded using the OpenSSL utility:

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
