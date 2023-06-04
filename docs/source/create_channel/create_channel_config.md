# Using configtx.yaml to build a channel configuration

A channel is created by building a channel genesis block artifact that specifies the initial configuration of the channel. The **channel configuration** is stored on the ledger, and governs all the subsequent blocks that are added to the channel. The channel configuration specifies which organizations are channel members, the ordering nodes that can add new blocks on the channel, as well as the policies that govern channel updates. The initial channel configuration, stored in the channel genesis block, can be updated through channel configuration updates. If a sufficient number of organizations approve a channel update, a new channel config block will govern the channel after it is committed to the channel.

While it is possible to build the channel genesis block file manually, it is easier to create a channel by using the `configtx.yaml` file and the [configtxgen](../commands/configtxgen.html) tool. The `configtx.yaml` file contains the information that is required to build the channel configuration in a format that can be easily read and edited by humans. The `configtxgen` tool reads the information in the `configtx.yaml` file and writes it to the [protobuf format](https://developers.google.com/protocol-buffers) that can be read by Fabric.

## Overview

You can use this tutorial to learn how to use the `configtx.yaml` file to build the initial channel configuration that is stored in the genesis block. The tutorial will discuss the portion of channel configuration that is built by each section of file.

- [Organizations](#organizations)
- [Capabilities](#capabilities)
- [Application](#application)
- [Orderer](#orderer)
- [Channel](#channel)
- [Profiles](#profiles)

Because different sections of the file work together to create the policies that govern the channel, we will discuss channel policies in [their own tutorial](channel_policies.html).

Building off of the [Creating a channel tutorial](create_channel_participation.html), we will use the `configtx.yaml` file that is used to deploy the Fabric test network as an example. Open a command terminal on your local machine and navigate to the `test-network` directory in your local clone of the Fabric samples:
```
cd fabric-samples/test-network
```

The `configtx.yaml` file used by the test network is located in the `configtx` folder. Open the file in a text editor. You can refer back to this file as the tutorial goes through each section. You can find a more detailed version of the `configtx.yaml` file in the [Fabric sample configuration](https://github.com/hyperledger/fabric/blob/{BRANCH}/sampleconfig/configtx.yaml).

## Organizations

The most important information contained in the channel configuration are the organizations that are channel members.  Each organization is identified by an MSP ID and a [channel MSP](../membership/membership.html). The channel MSP is stored in the channel configuration and contains the certificates that are used to the identify the nodes, applications, and administrators of an organization. The **Organizations** section of `configtx.yaml` file is used to create the channel MSP and accompanying MSP ID for each member of the channel.

The `configtx.yaml` file used by the test network contains three organizations. Two organizations are peer organizations, Org1 and Org2, that can be added to application channels. One organization, OrdererOrg, is the administrator of the ordering service. Because it is a best practice to use different certificate authorities to deploy peer nodes and ordering nodes, organizations are often referred to as peer organizations or ordering organizations, even if they are in fact run by the same company.

You can see the part of `configtx.yaml` that defines Org1 of the test network below:
  ```yaml
  - &Org1
      # DefaultOrg defines the organization which is used in the sampleconfig
      # of the fabric.git development environment
      Name: Org1MSP

      # ID to load the MSP definition as
      ID: Org1MSP

      MSPDir: ../organizations/peerOrganizations/org1.example.com/msp

      # Policies defines the set of policies at this level of the config tree
      # For organization policies, their canonical path is usually
      #   /Channel/<Application|Orderer>/<OrgName>/<PolicyName>
      Policies:
          Readers:
              Type: Signature
              Rule: "OR('Org1MSP.admin', 'Org1MSP.peer', 'Org1MSP.client')"
          Writers:
              Type: Signature
              Rule: "OR('Org1MSP.admin', 'Org1MSP.client')"
          Admins:
              Type: Signature
              Rule: "OR('Org1MSP.admin')"
          Endorsement:
              Type: Signature
              Rule: "OR('Org1MSP.peer')"

      # OrdererEndpoints is a list of all orderers this org runs which clients
      # and peers may to connect to to push transactions and receive blocks respectively.
      OrdererEndpoints:
          - "orderer.example.com:7050"
  ```  

  - The `Name` field is an informal name used to identify the organization.

  - The `ID` field is the organization's MSP ID. The MSP ID acts as a unique identifier for your organization, and is referred to by channel policies and is included in the transactions submitted to the channel.

  - The `MSPDir` is the path to an MSP folder that was created by the organization. The `configtxgen` tool will use this MSP folder to create the channel MSP. This MSP folder needs to contain the following information, which will be transferred to the channel MSP and stored in the channel configuration:
    - A CA root certificate that establishes the root of trust for the organization. The CA root cert is used to verify if an application, node, or administrator belongs to a channel member.
    - A root cert from the TLS CA that issued the TLS certificates of the peer or orderer nodes. The TLS root cert is used to identify the organization by the gossip protocol.
    - If Node OUs are enabled, the MSP folder needs to contain a `config.yaml` file that identifies the administrators, nodes, and clients based on the OUs of their x509 certificates.
    - If Node OUs are not enabled, the MSP needs to contain an admincerts folder that contains the signing certificates of the organizations administrator identities.

    The MSP folder that is used to create the channel MSP only contains public certificates. As a result, you can build the MSP folder locally, and then send the MSP to the organization that is creating the channel.

  - The `Policies` section is used to define a set of signature policies that reference the channel member. We will discuss these policies in more detail when we discuss [channel policies](channel_policies.html).

  - The `OrdererEndpoints` indicates the orderer node endpoints that this organization makes available to clients and peers. Service discovery uses this information so that clients can pass the appropriate TLS certificates when connecting to an orderer endpoint.

## Capabilities

Fabric channels can be joined by orderer and peer nodes that are running different versions of Hyperledger Fabric. Channel capabilities allow organizations that are running different Fabric binaries to participate on the same channel by only enabling certain features. For example, organizations that are running Fabric v1.4 and organizations that are running Fabric v2.x can join the same channel as long as the channel capabilities levels are set to V1_4_X or below. None of the channel members will be able to use the features introduced in Fabric v2.0. Note that this is true as long as the new binaries still support a feature that existed in a previous version, and do not use a new feature that does not exist in a previous version. For example, the system channel is no longer supported in version v3.0. Thus, a v3.0 binary will not interoperate with a v2.x binary that still operates with the system channel.

If you examine the `configtx.yaml` file, you will see three capability groups:

- **Application** capabilities govern the features that are used by peer nodes, such as the Fabric chaincode lifecycle, and set the minimum version of the Fabric binaries that can be run by peers joined to the channel.

- **Orderer** capabilities govern the features that are used by orderer nodes, such as Raft consensus, and set the minimum version of the Fabric binaries that can be run by ordering nodes that belong to the channel consenter set.

- **Channel** capabilities set the minimum version of the Fabric that can be run by peer and ordering nodes.

Because both of the peers and the ordering node of the Fabric test network run version v2.x, every capability group is set to `V2_0`. As a result, the test network cannot be joined by nodes that run a lower version of Fabric than v2.0. For more information, see the [capabilities](../capabilities_concept.html) concept topic.  

## Application

The application section defines the policies that govern how peer organizations can interact with application channels. These policies govern the number of peer organizations that need to approve a chaincode definition or sign a request to update the channel configuration. These policies are also used to restrict access to channel resources, such as the ability to write to the channel ledger or to query channel events.

The test network uses the default application policies provided by Hyperledger Fabric. If you use the default policies, all peer organizations will be able to read and write data to the ledger. The default policies also require that a majority of channel members sign channel configuration updates and that a majority of channel members need to approve a chaincode definition before a chaincode can be deployed to a channel. The contents of this section are discussed in more detail in the [channel policies](channel_policies.html) tutorial.

## Orderer

Each channel configuration includes the orderer nodes in the channel [consenter set](../glossary.html#consenter-set). The consenter set is the group of ordering nodes that have the ability to create new blocks and distribute them to the peers joined to the channel. The endpoint information of each ordering node that is a member of the consenter set is stored in the channel configuration.

 The test network uses the **Orderer** section of the `configtx.yaml` file to create a single node Raft ordering service.

- The `OrdererType` field is used to select Raft as the consensus type:
  ```
  OrdererType: etcdraft
  ```

  Raft ordering services are defined by the list of consenters that can participate in the consensus process. Because the test network only uses a single ordering node, the consenters list contains only one endpoint:
  ```yaml
  EtcdRaft:
      Consenters:
      - Host: orderer.example.com
        Port: 7050
        ClientTLSCert: ../organizations/ordererOrganizations/example.com/orderers/orderer.example.com/tls/server.crt
        ServerTLSCert: ../organizations/ordererOrganizations/example.com/orderers/orderer.example.com/tls/server.crt
  ```

  Each ordering node in the list of consenters is identified by their endpoint address and their client and server TLS certificate. If you are deploying a multi-node ordering service, you would need to provide the hostname, port, and the path to the TLS certificates used by each node.

- You can use the `BatchTimeout` and `BatchSize` fields to tune the latency and throughput of the channel by changing the maximum size of each block and how often a new block is created. The `BatchSize` property includes `MaxMessageCount`, `AbsoluteMaxBytes`, and `PreferredMaxBytes` settings. A block will be cut when any of the `BatchTimeout` or `BatchSize` criteria has been met. For `AbsoluteMaxBytes` it is recommended not to exceed 49 MB, given the default gRPC maximum message size of 100 MB configured on orderer and peer nodes (and allowing for message expansion during communication).

- The `Policies` section creates the policies that govern the channel consenter set. The test network uses the default policies provided by Fabric, which require that a majority of orderer administrators approve the addition or removal of ordering nodes, organizations, or an update to the block cutting parameters.

Because the test network is used for development and testing, it uses an ordering service that consists of a single ordering node. Networks that are deployed in production should use a multi-node ordering service for security and availability. To learn more, see [Configuring and operating a Raft ordering service](../raft_configuration.html).  

## Channel

The channel section defines that policies that govern the highest level of the channel configuration. In an application channel, these policies govern the hashing algorithm, the data hashing structure used to create new blocks, and the channel capability level.

The test network uses the default policies provided by Fabric, which require that changes would need to be approved by a majority of orderer organizations and a majority of channel members. Most users will not need to change these values.

## Profiles

The `configtxgen` tool reads the channel profiles in the **Profiles** section to build a channel configuration. Each profile uses YAML syntax to gather data from other sections of the file. The `configtxgen` tool uses this configuration to create a genesis block for joining orderers to a new channel via the channel participation API. To learn more about YAML syntax, [Wikipedia](https://en.wikipedia.org/wiki/YAML) provides a good place to get started.

The `configtx.yaml` used by the test network contains one channel profile `TwoOrgsApplicationGenesis`.
The `TwoOrgsApplicationGenesis` profile is used by the test network to create application channels:
```yaml
TwoOrgsApplicationGenesis:
  <<: *ChannelDefaults
  Orderer:
    <<: *OrdererDefaults
    Organizations:
      - *OrdererOrg
    Capabilities: *OrdererCapabilities
  Application:
    <<: *ApplicationDefaults
    Organizations:
      - *Org1
      - *Org2
    Capabilities: *ApplicationCapabilities
```

### SampleAppChannelEtcdRaft

The `SampleAppChannelEtcdRaft` profile is provided as an example of creating a channel by using the `osnadmin CLI`.

```
SampleAppChannelEtcdRaft:
    <<: *ChannelDefaults
    Orderer:
        <<: *OrdererDefaults
        OrdererType: etcdraft
        Organizations:
            - <<: *SampleOrg
              Policies:
                  <<: *SampleOrgPolicies
                  Admins:
                      Type: Signature
                      Rule: "OR('SampleOrg.member')"
    Application:
        <<: *ApplicationDefaults
        Organizations:
            - <<: *SampleOrg
              Policies:
                  <<: *SampleOrgPolicies
                  Admins:
                      Type: Signature
                      Rule: "OR('SampleOrg.member')"
```
Check out the [Create a channel](create_channel_participation.html) tutorial to learn more about channel creation.