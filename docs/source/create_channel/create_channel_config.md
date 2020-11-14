# Using configtx.yaml to build a channel configuration

A channel is created by building a channel creation transaction artifact that specifies the initial configuration of the channel. The **channel configuration** is stored on the ledger, and governs all the subsequent blocks that are added to the channel. The channel configuration specifies which organizations are channel members, the ordering nodes that can add new blocks on the channel, as well as the policies that govern channel updates. The initial channel configuration, stored in the channel genesis block, can be updated through channel configuration updates. If a sufficient number of organizations approve a channel update, a new channel config block will govern the channel after it is committed to the channel.

While it is possible to build the channel creation transaction file manually, it is easier to create a channel by using the `configtx.yaml` file and the [configtxgen](../commands/configtxgen.html) tool. The `configtx.yaml` file contains the information that is required to build the channel configuration in a format that can be easily read and edited by humans. The `configtxgen` tool reads the information in the `configtx.yaml` file and writes it to the [protobuf format](https://developers.google.com/protocol-buffers) that can be read by Fabric.

## Overview

You can use this tutorial to learn how to use the `configtx.yaml` file to build the initial channel configuration that is stored in the genesis block. The tutorial will discuss the portion of channel configuration that is built by each section of file.

- [Organizations](#organizations)
- [Capabilities](#capabilities)
- [Application](#application)
- [Orderer](#orderer)
- [Channel](#channel)
- [Profiles](#profiles)

Because different sections of the file work together to create the policies that govern the channel, we will discuss channel policies in [their own tutorial](channel_policies.html).

Building off of the [Creating a channel tutorial](create_channel.html), we will use the `configtx.yaml` file that is used to deploy the Fabric test network as an example. Open a command terminal on your local machine and navigate to the `test-network` directory in your local clone of the Fabric samples:
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

      # leave this flag set to true.
      AnchorPeers:
          # AnchorPeers defines the location of peers which can be used
          # for cross org gossip communication.  Note, this value is only
          # encoded in the genesis block in the Application section context
          - Host: peer0.org1.example.com
            Port: 7051
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

  - The `AnchorPeers` field lists the anchor peers for an organization. Anchor peers are required in order to take advantage of features such as private data and service discovery. It is recommended that organizations select at least one anchor peer. While an organization can select their anchor peers on the channel for the first time using the `configtxgen` tool, it is recommended that each organization set anchor peers by using the `configtxlator` tool to [update the channel configuration](create_channel.html#set-anchor-peers). As a result, this field is not required.

## Capabilities

Fabric channels can be joined by orderer and peer nodes that are running different versions of Hyperledger Fabric. Channel capabilities allow organizations that are running different Fabric binaries to participate on the same channel by only enabling certain features. For example, organizations that are running Fabric v1.4 and organizations that are running Fabric v2.x can join the same channel as long as the channel capabilities levels are set to V1_4_X or below. None of the channel members will be able to use the features introduced in Fabric v2.0.

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
      Addresses:
      - orderer.example.com:7050
  ```

  Each ordering node in the list of consenters is identified by their endpoint address and their client and server TLS certificate. If you are deploying a multi-node ordering service, you would need to provide the hostname, port, and the path to the TLS certificates used by each node. You would also need to add the endpoint address of each ordering node to the list of `Addresses`.

- You can use the `BatchTimeout` and `BatchSize` fields to tune the latency and throughput of the channel by changing the maximum size of each block and how often a new block is created.

- The `Policies` section creates the policies that govern the channel consenter set. The test network uses the default policies provided by Fabric, which require that a majority of orderer administrators approve the addition or removal of ordering nodes, organizations, or an update to the block cutting parameters.

Because the test network is used for development and testing, it uses an ordering service that consists of a single ordering node. Networks that are deployed in production should use a multi-node ordering service for security and availability. To learn more, see [Configuring and operating a Raft ordering service](../raft_configuration.html).  

## Channel

The channel section defines that policies that govern the highest level of the channel configuration. For an application channel, these policies govern the hashing algorithm, the data hashing structure used to create new blocks, and the channel capability level. In the system channel, these policies also govern the creation or removal of consortiums of peer organizations.

The test network uses the default policies provided by Fabric, which require that a majority of orderer service administrators would need to approve updates to these values in the system channel. In an application channel, changes would need to be approved by a majority of orderer organizations and a majority of channel members. Most users will not need to change these values.

## Profiles

The `configtxgen` tool reads the channel profiles in the **Profiles** section to build a channel configuration. Each profile uses YAML syntax to gather data from other sections of the file. The `configtxgen` tool uses this configuration to create a channel creation transaction for an applications channel, or to write the channel genesis block for a system channel. To learn more about YAML syntax, [Wikipedia](https://en.wikipedia.org/wiki/YAML) provides a good place to get started.

The `configtx.yaml` used by the test network contains two channel profiles, `TwoOrgsOrdererGenesis` and `TwoOrgsChannel`:

### TwoOrgsOrdererGenesis

The `TwoOrgsOrdererGenesis` profile is used to create the system channel genesis block:
```yaml
TwoOrgsOrdererGenesis:
    <<: *ChannelDefaults
    Orderer:
        <<: *OrdererDefaults
        Organizations:
            - *OrdererOrg
        Capabilities:
            <<: *OrdererCapabilities
    Consortiums:
        SampleConsortium:
            Organizations:
                - *Org1
                - *Org2
```

The system channel defines the nodes of the ordering service and the set of organizations that are ordering service administrators. The system channel also includes a set of peer organizations that belong to the blockchain [consortium](../glossary.html#consortium). The channel MSP of each member of the consortium is included in the system channel, allowing them to create new application channels and add consortium members to the new channel.

The profile creates a consortium named `SampleConsortium` that contains the two peer organizations in the `configtx.yaml` file, Org1 and Org2. The `Orderer` section of the profile uses the single node Raft ordering service defined in the **Orderer:** section of the file. The OrdererOrg from the **Organizations:** section is made the only administrator of the ordering service. Because our only ordering node is running Fabric 2.x, we can set the orderer system channel capability to `V2_0`. The system channel uses default policies from the **Channel** section and enables `V2_0` as the channel capability level.

### TwoOrgsChannel

The `TwoOrgsChannel` profile is used by the test network to create application channels:
```yaml
TwoOrgsChannel:
    Consortium: SampleConsortium
    <<: *ChannelDefaults
    Application:
        <<: *ApplicationDefaults
        Organizations:
            - *Org1
            - *Org2
        Capabilities:
            <<: *ApplicationCapabilities
```

The system channel is used by the ordering service as a template to create application channels. The nodes of the ordering service that are defined in the system channel become the default consenter set of new channels, while the administrators of the ordering service become the orderer administrators of the channel. The channel MSPs of channel members are transferred to the new channel from the system channel. After the channel is created, ordering nodes can be added or removed from the channel by updating the channel configuration. You can also update the channel configuration to [add other organizations as channel members](../channel_update_tutorial.html).

The `TwoOrgsChannel` provides the name of the consortium, `SampleConsortium`, hosted by the test network system channel. As a result, the ordering service defined in the `TwoOrgsOrdererGenesis` profile becomes channel consenter set. In the `Application` section, both organizations from the consortium, Org1 and Org2, are included as channel members. The channel uses `V2_0` as the application capabilities, and uses the default policies from the **Application** section to govern how peer organizations will interact with the channel. The application channel also uses the default policies from the **Channel** section and enables `V2_0` as the channel capability level.
