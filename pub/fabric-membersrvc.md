## Certificate Authority aka Membership Services

The Certificate Authority (CA) aka Membership Services provides a number of certificate services to users of a blockchain. More specifically, these services relate to user enrollment, transactions invoked on the blockchain, and TLS-secured connections between users or components of the blockchain.

### Enrollment Certificate Authority

The enrollment certificate authority (ECA) allows new users to register with the blockchain network and enables registered users to request an enrollment certificate pair. One certificate is for data signing, one is for data encryption. The public keys to be embedded in the certificates have to be of type ECDSA, whereby the key for data encryption is then converted by the user to be used in an ECIES (Elliptic Curve Integrated Encryption System) fashion.

### Transaction Certificate Authority

Once a user is enrolled, he or she can also request transaction certificates from the transaction certificate authority (TCA). These certificates are to be used for deploying Chaincode and for invoking Chaincode transactions on the blockchain. Although a single transaction certificate can be used for multiple transactions, for privacy reasons it is recommended that a new transaction certificate be used for each transaction.

### TLS Certificate Authority

In addition to enrollment certificates and transaction certificates, users will need TLS certificates to secure their communication channels. TLS certificates can be requested from the TLS certificate authority (TLSCA).

## Usage

Here’s a sample docker-compose.yml for the CA.

```
membersrvc:
  image: hyperledger/fabric-membersrvc
  command: membersrvc
```

The corresponding docker-compose.yml for running Docker on Mac or Windows natively looks like this:

```
membersrvc:
  image: hyperledger/fabric-membersrvc
  ports:
    - "7054:7054"
  command: membersrvc
```

If you are launching one or more peer nodes in the same docker-compose.yml, then you will want to add a delay to the start of the peer to allow sufficient time for the CA to start, before the peer attempts to connect to it.

```
membersrvc:
  image: hyperledger/fabric-membersrvc
  command: membersrvc
vp0:
  image: hyperledger/fabric-peer
  environment:
    - CORE_PEER_ADDRESSAUTODETECT=true
    - CORE_VM_ENDPOINT=http://172.17.0.1:2375
    - CORE_LOGGING_LEVEL=DEBUG
    - CORE_PEER_ID=vp0
    - CORE_SECURITY_ENROLLID=test_vp0
    - CORE_SECURITY_ENROLLSECRET=MwYpmSRjupbT
  links:
    - membersrvc
  command: sh -c "sleep 5; peer node start"
```

The corresponding docker-compose.yml for running Docker on Mac or Windows natively looks like this:

```
membersrvc:
  image: hyperledger/fabric-membersrvc
  ports:
    - "7054:7054"
  command: membersrvc
vp0:
  image: hyperledger/fabric-peer
  ports:
    - "7050:7050"
    - "7051:7051"
    - "7052:7052"
  environment:
    - CORE_PEER_ADDRESSAUTODETECT=true
    - CORE_VM_ENDPOINT=unix:///var/run/docker.sock
    - CORE_LOGGING_LEVEL=DEBUG
    - CORE_PEER_ID=vp0
    - CORE_SECURITY_ENROLLID=test_vp0
    - CORE_SECURITY_ENROLLSECRET=MwYpmSRjupbT
  links:
    - membersrvc
  command: sh -c "sleep 5; peer node start"
```

### Advanced Configuration

Please visit the Hyperledger Fabric [documentation](http://hyperledger-fabric.readthedocs.io/en/latest/Setup/ca-setup/) for more advanced configurations.
