#
#
# Copyright IBM Corp. 2016 All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

# Test Endorser function
#
# Tags that can be used and will affect test internals:
#  @doNotDecompose will NOT decompose the named compose_yaml after scenario ends.  Useful for setting up environment and reviewing after scenario.

@endorser
Feature: Endorser
    As a application developer
    I want to get endorsements and submit transactions and receive events

#    @doNotDecompose
    @FAB-184
  	Scenario Outline: Basic deploy endorsement for chaincode through GRPC to multiple endorsers

	    Given we compose "<ComposeFile>"
        And I wait "1" seconds
        And I register with CA supplying username "binhn" and secret "7avZQLwcUe9q" on peers:
          | vp0  |

        When user "binhn" creates a chaincode spec "cc_spec" of type "GOLANG" for chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with args
             | funcName | arg1 |  arg2 | arg3 | arg4 |
             |   init   |  a   |  100  |  b   |  200 |
        And user "binhn" creates a deployment spec "cc_deploy_spec" using chaincode spec "cc_spec" and devops on peer "vp0"
        And user "binhn" creates a deployment proposal "proposal1" using chaincode deployment spec "cc_deploy_spec"
        And user "binhn" sends proposal "proposal1" to endorsers with timeout of "20" seconds:
          | vp0  | vp1 | vp2 |  vp3  |
        And user "binhn" stores their last result as "proposal1Responses"
        Then user "binhn" expects proposal responses "proposal1Responses" with status "200" from endorsers:
          | vp0  | vp1 | vp2 |  vp3  |


    Examples: Orderer Options
        |          ComposeFile                 |    Waittime   |
        |   docker-compose-next-4.yml          |       60      |


#    @doNotDecompose
    @FAB-314
	Scenario Outline: Advanced deploy endorsement with ESCC for chaincode through GRPC to single endorser

	    Given we compose "<ComposeFile>"
        And I register with CA supplying username "binhn" and secret "7avZQLwcUe9q" on peers:
          | vp0  |

        When user "binhn" creates a chaincode spec of type "GOLANG" for chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" aliased as "cc_spec" with args
             | funcName | arg1 |  arg2 | arg3 | arg4 |
             |   init   |  a   |  100  |  b   |  200 |
        And user "binhn" sets ESCC to "my_escc" for chaincode spec "cc_spec"
        And user "binhn" creates a deployment proposal "proposal1" using chaincode spec "cc_spec"
        And user "binhn" sends proposal "proposal1" to endorsers with timeout of "20" seconds:
          | vp0  | vp1 | vp2 |  vp3  |
        And user "binhn" stores their last result as "proposal1Responses"
        Then user "binhn" expects proposal responses "proposal1Responses" with status "200" from endorsers:
          | vp0  | vp1 | vp2 |  vp3  |

    Examples: Orderer Options
        |          ComposeFile           |    Waittime   |
        |   docker-compose-next-4.yml    |       60      |

#    @doNotDecompose
    @FAB-314
	Scenario Outline: Advanced deploy endorsement with VSCC for chaincode through GRPC to single endorser

	    Given we compose "<ComposeFile>"
        And I register with CA supplying username "binhn" and secret "7avZQLwcUe9q" on peers:
          | vp0  |

        When user "binhn" creates a chaincode spec of type "GOLANG" for chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" aliased as "cc_spec" with args
             | funcName | arg1 |  arg2 | arg3 | arg4 |
             |   init   |  a   |  100  |  b   |  200 |
        And user "binhn" sets VSCC to "my_vscc" for chaincode spec "cc_spec"
        And user "binhn" creates a deployment proposal "proposal1" using chaincode spec "cc_spec"
        And user "binhn" sends proposal "proposal1" to endorsers with timeout of "20" seconds:
          | vp0  | vp1 | vp2 |  vp3  |
        And user "binhn" stores their last result as "proposal1Responses"
        Then user "binhn" expects proposal responses "proposal1Responses" with status "200" from endorsers:
          | vp0  | vp1 | vp2 |  vp3  |

    Examples: Orderer Options
        |          ComposeFile           |    Waittime   |
        |   docker-compose-next-4.yml    |       60      |

