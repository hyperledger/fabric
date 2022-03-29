## Certificate Management Guide

**Audience**: Fabric network admins

This guide provides a brief overview, resources, and technical details for Fabric network administrators to manage certificates (certs).

### Key Concepts

**Registration** – A registration is a username (enrollment ID) and password stored in the certificate authority (CA). This is created by a CA admin level user and has no expiration. The registration also contains any roles and attributes required.

**Enrollment** – Based on the registration username and password, an enrollment (or identity), can be created. The enrollment contains a public/private key pair and the (encoded) roles and attributes from the registration. The public key in the enrollment is referred to as a certificate, which is a signed certificate that is required to use Hyperledger Fabric.

### Certificate types

Hyperledger Fabric implements two basic types of certificates: 1) **enrollment** (or identity) certificates, and 2) **TLS** certificates.

#### Enrollment Certificates

Enrollment certificates are classed by type into the following four roles:

* **client**
* **admin**
* **peer**
* **orderer**

(Details on these enrollment certificate roles are described [here](https://hyperledger-fabric.readthedocs.io/en/latest/membership/membership.html#node-ou-roles-and-msps). 

The roles are used for the following purposes:

<b>admin:</b>  These certificates are signed certificates used by Hyperledger Fabric to identify identities that are administrative level user certificates.  These identities are required in order to make changes to Hyperledger Fabric configurations.

<b>peer and orderer:</b>  These certificates are signed certificates used by Hyperledger Fabric nodes (peers, orderers) and are node enrollment certificates.  These certificates are located physically on the node or mapped so they are visible to the node.  Hyperledger Fabric nodes must have a valid certificate with the proper attributes in order for the node to start.

<b>client:</b>  Client certificates are signed certificates that are that allow signed requests passed to the Hyperledger Fabric nodes.  These can be requests that do not require admin level certificates or client applications that need to pass signed requests to Hyperledger Fabric.

#### TLS Certificates

TLS certificates are signed certificates that allow Hyperledger Fabric nodes and clients to sign and encrypt communications.  A valid TLS certificate is required for all communications.


### Certificate Expiration

All certificates (public keys) associated with enrollments are given an expiration date by the issuing certificate authority (CA).  These dates must be monitored and enrollments must be renewed by a re-enrollment before the expiration date of the certificate.


### Prerequisites and resources

The following background resources (links) for identities, MSPs and CAs provides context for understanding this certs management guide – read or review them as necessary based on your level of identity and certs experience. Then you can continue to the certs table below for the technical details on each Fabric certificate name, definition, location, and further reading.

The following is recommended reading for understanding certificates used by Hyperledger Fabric:

Documentation for identity within Hyperledger Fabric:<br>
https://hyperledger-fabric.readthedocs.io/en/latest/identity/identity.html#identity

Documentation for MSPs Hyperledger Fabric:<br>
https://hyperledger-fabric.readthedocs.io/en/latest/membership/membership.html#membership-service-provider-msp

Overview of Registration and Enrollment:<br>
https://hyperledger-fabric-ca.readthedocs.io/en/latest/deployguide/use_CA.html#overview-of-registration-and-enrollment

Registering an Identity:<br>
https://hyperledger-fabric-ca.readthedocs.io/en/latest/deployguide/use_CA.html#register-an-identity

Enroll an Identity:<br>
https://hyperledger-fabric-ca.readthedocs.io/en/latest/deployguide/use_CA.html#enroll-an-identity


It is key to understand that an identity contains the public certificate and private key used for encryption.  The public certificate is a signed certificate from the CA and is referred to generically as a certificate in the following table.

The table shows the types of certificates used by Hyperledger Fabric.


## Technical details

**Note:** Certificates for Organization CAs and the TLS CA are being listed separately.  This is due to the differing purposes for the CAs.   The Organization CA supplies signing certificates and the TLS CA supplies TLS certificates for securing connections.

### Organization CA Certificates

| Certificate | Description | Location | Impact if expired | Reference |
|----------------|-----|---------------------------|-----------|---|
| Root Certificate |This is the controlling certificate used to sign all certificates issue by the CA. | Stored on disk in the CA directory.<br><br>ca-cert.pem| CA root certificates are issued for 15 years.  A New certificate must be issued  |  |
|CA Admin Certificate|The certificate used when making administrative requests to the CA, for example registering new users.| Depends on implementation <br><br><code>msp <br />&nbsp;&nbsp;&nbsp;├── IssuerPublicKey <br>&nbsp;&nbsp;&nbsp;├── IssuerRevocationPublicKey <br>&nbsp;&nbsp;&nbsp;├── cacerts <br>&nbsp;&nbsp;&nbsp;│&nbsp;&nbsp;&nbsp;└── localhost-7053.pem <br>&nbsp;&nbsp;&nbsp;├── keystore <br>&nbsp;&nbsp;&nbsp;│&nbsp;&nbsp;&nbsp;└── key.pem <br>&nbsp;&nbsp;&nbsp;├── signcerts <br>&nbsp;&nbsp;&nbsp;│&nbsp;&nbsp;&nbsp;└── cert.pem <br>&nbsp;&nbsp;&nbsp;└── user <br></code> |Cannot register or enroll new identities, but transaction traffic does not stop.| https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-orderer-org-s-ca-admin |

### TLS CA Certificates

| Certificate | Description | Location | Impact if expired | Reference |
|----------------|-----|---------------------------|-----------|---|
| Root Certificate |The certificate used when making administrative requests to the CA, for example registering new users| Stored on disk in the CA directory.<br><br>ca-cert.pem| CA root certificates are issued for 15 years.  A New certificate must be issued  |  |
|CA Signing Certificate|The certificate used when making administrative requests to the CA, for example registering new users| Depends on implementation <br /><br /><code>msp <br />&nbsp;&nbsp;&nbsp;├── IssuerPublicKey <br>&nbsp;&nbsp;&nbsp;├── IssuerRevocationPublicKey <br>&nbsp;&nbsp;&nbsp;├── cacerts <br>&nbsp;&nbsp;&nbsp;│&nbsp;&nbsp;&nbsp;└── localhost-7053.pem <br>&nbsp;&nbsp;&nbsp;├── keystore <br>&nbsp;&nbsp;&nbsp;│&nbsp;&nbsp;&nbsp;└── key.pem <br>&nbsp;&nbsp;&nbsp;├── signcerts <br>&nbsp;&nbsp;&nbsp;│&nbsp;&nbsp;&nbsp;└── cert.pem <br>&nbsp;&nbsp;&nbsp;└── user <br></code> |Cannot register or enroll new identities, but transaction traffic does not stop. | https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-tls-ca-s-admin |


### Peer Certificates

| Certificate | Description | Location | Impact if expired | Reference |
|----------------|--------------|---------------------------|------------------|-------------------|
|Peer Enrollment Certificate<br><br>type=peer|The public key that the used to managed changes to the CA. |Depends on implementation &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;<br><br><code>org1ca/<br />└── peer1<br />&nbsp;&nbsp;├── msp <br />&nbsp;&nbsp;│   &nbsp;&nbsp;├── cacerts <br />&nbsp;&nbsp;│   &nbsp;&nbsp;│   └── localhost-7053.pem <br />&nbsp;&nbsp;│   &nbsp;&nbsp;├── keystore <br />&nbsp;&nbsp;│   &nbsp;&nbsp;│   └── key.pem <br />&nbsp;&nbsp;│   &nbsp;&nbsp;├── signcerts <br />&nbsp;&nbsp;│   &nbsp;&nbsp;│   └── cert.pem <br />&nbsp;&nbsp;│   &nbsp;&nbsp;└── user <br />&nbsp;&nbsp;└── tls </code> <br />|Production outage <br /><br />Peers do not start without a valid enrollment certificate.|https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-peer1 |
|Peer TLS Certificate|This is the TLS certificate for the peer.| Depends on implementation<br><br><code>org1ca/ <br />└── peer1 <br />&nbsp;&nbsp;├── msp <br />&nbsp;&nbsp;└── tls <br />&nbsp;&nbsp;  &nbsp;&nbsp;  ├── cacerts <br />&nbsp;&nbsp;  &nbsp;&nbsp;  ├── keystore <br />&nbsp;&nbsp;  &nbsp;&nbsp;   │   └── key.pem <br />&nbsp;&nbsp;  &nbsp;&nbsp;  ├── signcerts <br />&nbsp;&nbsp;  &nbsp;&nbsp;  │   └── cert.pem <br />&nbsp;&nbsp;  &nbsp;&nbsp;  ├── tlscacerts <br />&nbsp;&nbsp;  &nbsp;&nbsp;  │   └── tls-localhost-7053.pem <br />&nbsp;&nbsp;  &nbsp;&nbsp;  └── user <br /></code>|Production outage <br><br>No communication is possible to the peer.|https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-peer1 |


### Orderer Certificates

| Certificate | Description | Location | Impact if expired | Reference |
|----------------|-----|---------------------------|-----------|---|
|Orderer Enrollment Certificate<br><br>Type=orderer | The public key that the orderer uses to sign transactions.|Depends on implementation&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;<br><br><code>ordererca/ <br />└── orderer1 <br />&nbsp;&nbsp;├── msp <br />&nbsp;&nbsp;│   &nbsp;&nbsp;├── cacerts <br />&nbsp;&nbsp;│   &nbsp;&nbsp;│   &nbsp;&nbsp;└── localhost-7053.pem <br />&nbsp;&nbsp;│   &nbsp;&nbsp;├── keystore <br />&nbsp;&nbsp;│   &nbsp;&nbsp;│   &nbsp;&nbsp;└── key.pem <br />&nbsp;&nbsp;│   &nbsp;&nbsp;├── signcerts <br />&nbsp;&nbsp;│   &nbsp;&nbsp;│   &nbsp;&nbsp;└── cert.pem <br />&nbsp;&nbsp;│   &nbsp;&nbsp;└── user <br />&nbsp;&nbsp;└── tls <br /></code>|Production outage<br><br> Orderers do not start without a valid enrollment certificate.|https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-orderer|
|Orderer TLS Certificate|This is the TLS certificate for the ordering node.|Depends on implementation<br><br><code>ordererca/ <br />└── orderer1 <br />&nbsp;&nbsp;├── msp <br />&nbsp;&nbsp;└── tls <br />&nbsp;&nbsp;&nbsp;&nbsp;├── cacerts <br />&nbsp;&nbsp;&nbsp;&nbsp;├── keystore <br />&nbsp;&nbsp;&nbsp;&nbsp;│   &nbsp;&nbsp;└── key.pem <br />&nbsp;&nbsp;&nbsp;&nbsp;├── signcerts <br />&nbsp;&nbsp;&nbsp;&nbsp;│   &nbsp;&nbsp;└── cert.pem <br />&nbsp;&nbsp;&nbsp;&nbsp;├── tlscacerts <br />&nbsp;&nbsp;&nbsp;&nbsp;│   &nbsp;&nbsp;└── tls-localhost-7053.pem <br />&nbsp;&nbsp;&nbsp;&nbsp;└── user <br /></code>|Production outage<br><br>Ordering nodes are no longer allowed to participate in cluster.  Quorum is lost, environment goes down.|https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-orderer |



### Admin Certificates

| Certificate | Description | Location | Impact if expired | Reference |
|----------------|-----|---------------------------|-----------|---|
|Ordering service organization channel admin certificate<br><br>Type=admin|The admin certificate that is used to administer the ordering service and submit or approve channel updates. The associated admin identity is enrolled with the organization CA before the node and organization MSP are created.|Depends on implementation&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;<br><br><code>ordererca/ <br />└── ordereradmin <br />&nbsp;&nbsp;└── msp <br />&nbsp;&nbsp; &nbsp;&nbsp;├── admincerts <br />&nbsp;&nbsp; &nbsp;&nbsp;│    &nbsp;&nbsp;└── cert.pem <br />&nbsp;&nbsp; &nbsp;&nbsp;├── cacerts <br />&nbsp;&nbsp; &nbsp;&nbsp;│    &nbsp;&nbsp;└── localhost-7053.pem <br />&nbsp;&nbsp; &nbsp;&nbsp;├── keystore <br />&nbsp;&nbsp; &nbsp;&nbsp;│    &nbsp;&nbsp;└── key.pem <br />&nbsp;&nbsp; &nbsp;&nbsp;├── signcerts <br />&nbsp;&nbsp; &nbsp;&nbsp;│    &nbsp;&nbsp;└── cert.pem <br />&nbsp;&nbsp; &nbsp;&nbsp;└── user <br /></code>|Transactions can continue to work successfully.<br><br>But you are unable to modify channels from a client application or manage the orderer from the console.|https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-org0-s-admin|
|Peer service Organization channel admin certificate<br><br>Type=admin|The admin certificate that is used to administer the peer and install smart contracts on the peer. It can also be configured to manage the application channel that the peer belongs to. The associated admin user is registered with the organization CA before the peer and organization MSP are created.|Depends on implementation<br><br><code>org1ca/ <br />└── org1admin <br />&nbsp;&nbsp;└── msp <br />&nbsp;&nbsp; &nbsp;&nbsp;├── admincerts <br />&nbsp;&nbsp; &nbsp;&nbsp;│   &nbsp;&nbsp;└── cert.pem <br />&nbsp;&nbsp; &nbsp;&nbsp;├── cacerts <br />&nbsp;&nbsp; &nbsp;&nbsp;│   &nbsp;&nbsp;└── localhost-7053.pem <br />&nbsp;&nbsp; &nbsp;&nbsp;├── keystore <br />&nbsp;&nbsp; &nbsp;&nbsp;│   &nbsp;&nbsp;└── key.pem <br />&nbsp;&nbsp; &nbsp;&nbsp;├── signcerts <br />&nbsp;&nbsp; &nbsp;&nbsp;│   &nbsp;&nbsp;└── cert.pem <br />&nbsp;&nbsp; &nbsp;&nbsp;└── user <br /> </code>|Transactions can continue to work successfully. <br> <br>But you are unable to install new smart contracts from a client application or manage the peer from the console.|https://hyperledger-fabric-ca.readthedocs.io/en/latest/operations_guide.html#enroll-org1-s-admin |

### Client Certificates

Client certificates are created by applications using the Fabric SDKs or command line CA utilities.  These certificates expire after one year if using the Hyperledger Fabric CA default settings.   Certificates must be re-enrolled before expiration or the client application will not be able to connect to the Fabric nodes.

Client certificates can be re-enrolled either by using command line Hyperledger Fabric CA utilities or by using the reenrollment method in the Fabric CA client SDK.

https://hyperledger.github.io/fabric-sdk-node/release-2.2/FabricCAClient.html#reenroll__anchor


### Certificate Decoding

Signed certificates are distinct from registrations. Registrations do not have an expiration. Signed certificates are created based on the registration and have information from the certificate authority that includes metadata indicating the parent certificate authority as well as encoded information regard the purpose of the certificate.

Certificates can be decoded to show the certificate details by using the openssl utility:

```
# openssl x509 -in cert.pem -text -noout
```


The following is an example of a decoded certificate:

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

The most important detail for certificates is the "Not After" element.  This is the expiration date of the certificate.  All certificates should be reenrolled before the certificate expiration.
