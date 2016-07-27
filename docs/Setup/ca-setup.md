## Certificate Authority (CA) Setup

The _Certificate Authority_ (CA) provides a number of certificate services to users of a blockchain. More specifically, these services relate to _user enrollment_, _transactions_ invoked on the blockchain, and _TLS_-secured connections between users or components of the blockchain.

This guide builds on either the [fabric developer's setup](../dev-setup/devenv.md) or the prerequisites articulated in the [fabric network setup](Network-setup.md) guide. If you have not already set up your environment with one of those guides, please do so before continuing.

### Enrollment Certificate Authority

The _enrollment certificate authority_ (ECA) allows new users to register with the blockchain network and enables registered users to request an _enrollment certificate pair_. One certificate is for data signing, one is for data encryption. The public keys to be embedded in the certificates have to be of type ECDSA, whereby the key for data encryption is then converted by the user to be used in an [ECIES](https://en.wikipedia.org/wiki/Integrated_Encryption_Scheme) (Elliptic Curve Integrated Encryption System) fashion.

### Transaction Certificate Authority

Once a user is enrolled, he or she can also request _transaction certificates_ from the _transaction certificate authority_ (TCA). These certificates are to be used for deploying Chaincode and for invoking Chaincode transactions on the blockchain. Although a single _transaction certificate_ can be used for multiple transactions, for privacy reasons it is recommended that a new _transaction certificate_ be used for each transaction.

### TLS Certificate Authority

In addition to _enrollment certificates_ and _transaction certificates_, users will need _TLS certificates_ to secure their communication channels. _TLS certificates_ can be requested from the _TLS certificate authority_ (TLSCA).

## Configuration

All CA services are provided by a single process, which can be configured by setting parameters in the CA configuration file `membersrvc.yaml`, which is located in the same directory as the CA binary. More specifically, the following parameters can be set:

- `server.gomaxprocs`: limits the number of operating system threads used by the CA.
- `server.rootpath`: the root path of the directory where the CA stores its state.
- `server.cadir`: the name of the directory where the CA stores its state.
- `server.port`: the port at which all CA services listen (multiplexing of services over the same port is provided by [GRPC](http://www.grpc.io)).

Furthermore, logging levels can be enabled/disabled by adjusting the following settings:

- `logging.trace` (off by default, useful for debugging the code only)
- `logging.info`
- `logging.warning`
- `logging.error`
- `logging.panic`

Alternatively, these fields can be set via environment variables, which---if set---have precedence over entries in the yaml file. The corresponding environment variables are named as follows:

```
    MEMBERSRVC_CA_SERVER_GOMAXPROCS
    MEMBERSRVC_CA_SERVER_ROOTPATH
    MEMBERSRVC_CA_SERVER_CADIR
    MEMBERSRVC_CA_SERVER_PORT
```

In addition, the CA may be preloaded with registered users, where each user's name, roles, and password are specified:

```
    eca:
    	users:
    		alice: 2 DRJ20pEql15a
    		bob: 4 7avZQLwcUe9q
```
The role value is simply a bitmask of the following:
```
    CLIENT = 1;
    PEER = 2;
    VALIDATOR = 4;
    AUDITOR = 8;
```

For example, a peer that is also a validator would have a role value of 6.

When the CA is started for the first time, it will generate all of its required state (e.g., internal databases, CA certificates, blockchain keys, etc.) and writes this state to the directory given in its configuration. The certificates for the CA services (i.e., for the ECA, TCA, and TLSCA) are self-signed as the current default. If those certificates shall be signed by some root CA, this can be done manually by using the `*.priv` and `*.pub` private and public keys in the CA state directory, and replacing the self-signed `*.cert` certificates with root-signed ones. The next time the CA is launched, it will read and use those root-signed certificates.

## Operating the CA

You can either [build and run](#build-and-run) the CA from source. Or, you can use Docker Compose and work with the published images on DockerHub, or some other Docker registry. Using Docker Compose is by far the simplest approach.

### Docker Compose

Here's a sample docker-compose.yml for the CA.

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

If you are launching one or more `peer` nodes in the same docker-compose.yml, then you will want to add a delay to the start of the peer to allow sufficient time for the CA to start, before the peer attempts to connect to it.

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

### Build and Run

The CA can be built with the following command executed in the `membersrvc` directory:

```
cd $GOPATH/src/github.com/hyperledger/fabric
make membersrvc
```

The CA can be started with the following command:

```
build/bin/membersrvc
```

**Note:** the CA must be started before any of the fabric peer nodes, to allow the CA to have initialized before any peer nodes attempt to connect to it.

The CA looks for an `membersrvc.yaml` configuration file in $GOPATH/src/github.com/hyperledger/fabric/membersrvc. If the CA is started for the first time, it creates all its required state (e.g., internal databases, CA certificates, blockchain keys, etc.) and writes that state to the directory given in the CA configuration.

<!-- This needs some serious attention

If starting the peer with security/privacy enabled, environment variables for security, CA address and peer's ID and password must be included. Additionally, the fabric-membersrvc container must be started before the peer(s) are launched. Hence we will need to insert a delay in launching the peer command. Here's the docker-compose.yml for a single peer with membership services running in a **Vagrant** environment:

```
vp0:
  image: hyperledger/fabric-peer
  environment:
  - CORE_PEER_ADDRESSAUTODETECT=true
  - CORE_VM_ENDPOINT=http://172.17.0.1:2375
  - CORE_LOGGING_LEVEL=DEBUG
  - CORE_PEER_ID=vp0
  - CORE_PEER_TLS_ENABLED=true
  - CORE_PEER_TLS_SERVERHOSTOVERRIDE=OBC
  - CORE_PEER_TLS_CERT_FILE=./bddtests/tlsca.cert
  - CORE_PEER_TLS_KEY_FILE=./bddtests/tlsca.priv
  command: sh -c "sleep 5; peer node start"

membersrvc:
   image: hyperledger/fabric-membersrvc
   command: membersrvc
```

```
docker run --rm -it -e CORE_VM_ENDPOINT=http://172.17.0.1:2375 -e CORE_PEER_ID=vp0 -e CORE_PEER_ADDRESSAUTODETECT=true -e CORE_SECURITY_ENABLED=true -e CORE_SECURITY_PRIVACY=true -e CORE_PEER_PKI_ECA_PADDR=172.17.0.1:7054 -e CORE_PEER_PKI_TCA_PADDR=172.17.0.1:7054 -e CORE_PEER_PKI_TLSCA_PADDR=172.17.0.1:7054 -e CORE_SECURITY_ENROLLID=vp0 -e CORE_SECURITY_ENROLLSECRET=vp0_secret  hyperledger/fabric-peer peer node start
```

Additionally, the validating peer `enrollID` and `enrollSecret` (`vp0` and `vp0_secret`) has to be added to [membersrvc.yaml](https://github.com/hyperledger/fabric/blob/master/membersrvc/membersrvc.yaml).
-->
