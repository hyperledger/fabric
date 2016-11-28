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

#    @doNotDecompose
  Scenario Outline: Bootstrap a development network with 1 peer and 1 orderer, each having a single independent root of trust (No COP)
    #creates 1 self-signed key/cert pair per orderer organization with an associated admin
    Given the orderer network has organizations:
      | Organization  |
      | ordererOrg0   |
    And user requests role of orderer admin by creating a key and csr for orderer and acquires signed certificate from organization:
      | User           | Orderer  | Organization  |
      | orderer0Admin  | orderer0 | ordererOrg0   |
      | orderer0Signer | orderer0 | ordererOrg0   |
#    And user requests role of orderer signer by creating a key and csr for orderer and acquires signed certificate from organization:
#      | User           | Orderer  | Organization  |

    And the peer network has organizations:
      |  peerOrg0  |

    And a ordererBootstrapAdmin is identified and given access to all public certificates and orderer node info
    # Order info includes orderer admin/orderer information and address (host:port) from previous steps
    # Only the peer organizations can vary.
    And the ordererBootstrapAdmin creates the genesis block for chain "<OrdererSystemChainId>" for network config policy "<PolicyType>" and consensus "<ConsensusType>" and peer organizations:
      | peerOrg0  |

    And the orderer admins inspect and approve the genesis block for chain "<OrdererSystemChainId>"

    # to be used for setting the orderer genesis block path parameter in composition
    And the orderer admins use the genesis block for chain "<OrdererSystemChainId>" to configure orderers
    And we compose "<ComposeFile>"



    # We now have an orderer network with NO peers.  Now need to configure and start the peer network
    And user requests role of peer admin by creating a key and csr for peer and acquires signed certificate from organization:
      | peer0Admin | peer0 | peerOrg0  |

    # We now have an orderer network with NO peers.  Now need to configure and start the peer network
    And user requests role of peer signer by creating a key and csr for peer and acquires signed certificate from organization:
      | peer0Signer | peer0 | peerOrg0  |


    And peer admins get the genesis block for chain 'chain1' from chainBoostrapAdmin
    And the peer admins inspect and approve the genesis block for chain 'chain1'

    And the peer admins use the genesis block for chain 'chain1' to configure peers

    And I wait "1" seconds
    And user "binhn" registers with peer organization "peerOrg0"

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
      |          ComposeFile                 |    Waittime   |       OrdererSystemChainId       |PolicyType    |   ConsensusType |
      |   docker-compose-next-4.yml          |       60      |   **TEST_CHAINID**               |unanimous    |       solo      |
