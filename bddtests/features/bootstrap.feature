#
# Test Bootstrap function
#
# Tags that can be used and will affect test internals:
#  @doNotDecompose will NOT decompose the named compose_yaml after scenario ends.  Useful for setting up environment and reviewing after scenario.
#  @chaincodeImagesUpToDate use this if all scenarios chaincode images are up to date, and do NOT require building.  BE SURE!!!

#@chaincodeImagesUpToDate
@bootstrap
Feature: Bootstrap
  As a blockchain entrepreneur
  I want to bootstrap a new blockchain network

    @doNotDecompose
  Scenario Outline: Bootstrap a development network with 1 peer and 1 orderer, each having a single independent root of trust (No COP)
    #creates 1 self-signed key/cert pair per orderer organization
    Given the orderer network has organizations:
      | Organization  |
      | ordererOrg0   |

    And user requests role of orderer admin by creating a key and csr for orderer and acquires signed certificate from organization:
      | User           | Orderer  | Organization  |
      | orderer0Signer | orderer0 | ordererOrg0   |

    And the peer network has organizations:
      | Organization  |
      |  peerOrg0     |
      |  peerOrg1     |
      |  peerOrg2     |

    And a ordererBootstrapAdmin is identified and given access to all public certificates and orderer node info

    And the ordererBootstrapAdmin generates a GUUID to identify the orderer system chain and refer to it by name as "OrdererSystemChainId"

    # Actually creating proto ordere_dot_confoiguration.ChainCreators
    # Creates SignedConfigItems
    # Policy
    And the ordererBootstrapAdmin creates a chain creators policy "chainCreatePolicy1" (network name) for peer orgs who wish to form a network using orderer system chain "OrdererSystemChainId":
      | Organization  |
      |  peerOrg0     |
      |  peerOrg1     |
      |  peerOrg2     |

    # Creates SignedConfigItems
    And the ordererBoostrapAdmin creates the chain creation policy names "chainCreationPolicyNames" signedConfigurationItem for orderer system chain "OrdererSystemChainId" with policies:
      |PolicyName                  |
      |chainCreatePolicy1          |


    # Creates SignedConfigItems
    And the ordererBoostrapAdmin creates MSP Configuration Items "mspConfigItems1" for orderer system chain "OrdererSystemChainId" for every MSP referenced by the policies:
      |PolicyName                  |
      |chainCreatePolicy1          |


    # Order info includes orderer admin/orderer information and address (host:port) from previous steps
    # Only the peer organizations can vary.
    And the ordererBootstrapAdmin creates the genesis block for chain "OrdererSystemChainId" for network config policy "<PolicyType>" and consensus "<ConsensusType>" using chain creators policies:
      |  SignedConfigItemsName       |
      |  chainCreatePolicy1          |
      |  chainCreationPolicyNames    |
      |  mspConfigItems1             |


    And the orderer admins inspect and approve the genesis block for chain "OrdererSystemChainId"

    # to be used for setting the orderer genesis block path parameter in composition
    And the orderer admins use the genesis block for chain "OrdererSystemChainId" to configure orderers

    # We now have an orderer network with NO peers.  Now need to configure and start the peer network
    # This can be currently automated through folder creation of the proper form and placing PEMs.
    And user requests role for peer by creating a key and csr for peer and acquires signed certificate from organization:
        | User            | Peer     | Organization  |
        | peer0Signer     | peer0    | peerOrg0      |
        | peer1Signer     | peer1    | peerOrg0      |
        | peer2Signer     | peer2    | peerOrg1      |
        | peer3Signer     | peer3    | peerOrg1      |

    And we compose "<ComposeFile>"

    #   This implicitly incorporates the orderer genesis block info
    And the ordererBootstrapAdmin runs the channel template tool to create the orderer configuration template "template1" for application developers using orderer "orderer0"
    And the ordererBootstrapAdmin distributes orderer configuration template "template1" and chain creation policy name "chainCreatePolicy1"

    And the following application developers are defined for peer organizations
      | Developer       | ChainCreationPolicyName     | Organization  |
      | dev0Org0        | chainCreatePolicy1          |  peerOrg0     |

    # Need Consortium MSP info and
    # need to add the ChannelWriters ConfigItem (using ChannelWriters ref name),
    # ChannelReaders ConfigItem (using ChannelReaders ref name)AnchorPeers ConfigItem
    # and the ChaincodeLifecyclePolicy Config Item
    # NOTE: Template1 will simply hold refs to peer orgs that can create in this channel at the moment
    And the user "dev0Org0" creates a peer template "template1" with chaincode deployment policy using chain creation policy name "chainCreatePolicy1" and peer organizations:
      | Organization  |
      |  peerOrg0     |
      |  peerOrg1     |

    And the user "dev0Org0" creates an peer anchor set "anchors1" for channel "com.acme.blockchain.jdoe.Channel1" for orgs:
      | User            | Peer     | Organization  |
      | peer0Signer     | peer0    | peerOrg0      |
      | peer2Signer     | peer2    | peerOrg1      |

    # TODO: grab the peer orgs from template1 and put into Murali's MSP info SCIs.
    And the user "dev0Org0" creates a signedConfigEnvelope "createChannelSignedConfigEnvelope1"
        | ChannelID                          | Template     | Chain Creation Policy Name  | Anchors  |
        | com.acme.blockchain.jdoe.Channel1  | template1    | chainCreatePolicy1          | anchors1 |

    And the user "dev0Org0" collects signatures for signedConfigEnvelope "createChannelSignedConfigEnvelope1" from peer orgs:
      | Organization  |
      |  peerOrg0     |
      |  peerOrg1     |

    And the user "dev0Org0" creates config Tx "configTx1" using signedConfigEnvelope "createChannelSignedConfigEnvelope1"

    And the user "dev0Org0" broadcasts config Tx "configTx1" to orderer "orderer0" to create channel "com.acme.blockchain.jdoe.Channel1"

    # Sleep as the deliver takes a bit to have the first block ready
    And I wait "2" seconds

    When user "dev0Org0" connects to deliver function on orderer "orderer0"
    And user "dev0Org0" sends deliver a seek request on orderer "orderer0" with properties:
      | ChainId                               | Start |  End    |
      | com.acme.blockchain.jdoe.Channel1     |   0   |  0      |

    Then user "dev0Org0" should get a delivery "genesisBlockForMyNewChannel" from "orderer0" of "1" blocks with "1" messages within "1" seconds

    When user "dev0Org0" request to join channel using genesis block "genesisBlockForMyNewChannel" on peer "peer0" with result "joinChannelResult"
      | Developer       | ChainCreationPolicyName     | Organization  |
      | dev0Org0        | chainCreatePolicy1          |  peerOrg0     |


    Then user "dev0Org0" expects result code for "joinChannelResult" of "200"

    # TODO: Add the channel name!!
    When user "binhn" creates a chaincode spec "cc_spec" of type "GOLANG" for chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with args
      | funcName | arg1 |  arg2 | arg3 | arg4 |
      |   init   |  a   |  100  |  b   |  200 |
    And user "binhn" creates a deployment spec "cc_deploy_spec" using chaincode spec "cc_spec" and devops on peer "vp0"
    And user "binhn" creates a deployment proposal "proposal1" using chaincode deployment spec "cc_deploy_spec"
    And user "binhn" sends proposal "proposal1" to endorsers with timeout of "20" seconds:
      | peer0  |
    And user "binhn" stores their last result as "proposal1Responses"
    Then user "binhn" expects proposal responses "proposal1Responses" with status "200" from endorsers:
      | peer0  |


    Examples: Orderer Options
      |          ComposeFile                 |    Waittime   | PolicyType    |   ConsensusType |
      |   docker-compose-next-4.yml          |       60      | unanimous     |       solo      |
