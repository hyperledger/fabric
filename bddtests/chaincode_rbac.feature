#
# Test Hyperledger Chaincodes using various RBAC mechanisms
#
# Tags that can be used and will affect test internals:
#
#  @doNotDecompose will NOT decompose the named compose_yaml after scenario ends.  Useful for setting up environment and reviewing after scenario.
#

Feature: Role Based Access Control (RBAC)
    As a HyperLedger developer
    I want various mechanisms available for implementing RBAC within Chaincode

  #@doNotDecompose
  @issue_1207
  Scenario Outline: test a chaincode showing how to implement role-based access control using TCerts with no attributes

      Given we compose "<ComposeFile>"
      And I wait "5" seconds
      And I register with CA supplying username "binhn" and secret "7avZQLwcUe9q" on peers:
         | vp0  |
      And I register with CA supplying username "alice" and secret "CMS10pEQlB16" on peers:
         | vp0  |
      And I use the following credentials for querying peers:
         | peer |   username  |    secret    |
         | vp0  |  test_user0 | MS9qrN8hFjlE |
         | vp1  |  test_user1 | jGlNl6ImkuDo |
         | vp2  |  test_user2 | zMflqOKezFiA |
         | vp3  |  test_user3 | vWdLCE00vJy0 |

      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "1"

      #Acquire a Application-TCert for binh (NOTE: application TCert is NEVER used for signing fabric TXs) 
      When user "binhn" requests a new application TCert
      # Binhn is assuming the application ROLE of Admin by using his own TCert
      And user "binhn" stores their last result as "TCERT_APP_ADMIN"

      # Deploy, in this case Binh is assinging himself as the Admin for the RBAC chaincode.
      When user "binhn" sets metadata to their stored value "TCERT_APP_ADMIN"    
      And user "binhn" deploys chaincode "github.com/hyperledger/fabric/examples/chaincode/go/rbac_tcerts_no_attrs" aliased as "rbac_tcerts_no_attrs" with ctor "init" and args
            ||
            ||
      Then I should have received a chaincode name
      Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
                 | vp0  | vp1 | vp2 | vp3 |

      # When we refer to any party, we actually mean a specific application TCert for that party

      #Acquire a Application-TCert for alice, and supplies to binhn (NOTE: application TCert is NEVER used for signing fabric TXs) 
      When user "alice" requests a new application TCert
      And user "alice" stores their last result as "TCERT_APP_ALICE_1"
      # Alice gives binhn her application TCert (usually OUT-OF-BAND)
      And user "alice" gives stored value "TCERT_APP_ALICE_1" to "binhn"

      
      # binhn is assigning the role of 'writer' to alice
      When "binhn" uses application TCert "TCERT_APP_ADMIN" to assign role "writer" to application TCert "TCERT_APP_ALICE_1"
      Then I should have received a transactionID
      Then I wait up to "60" seconds for transaction to be committed to peers:
         | vp0  | vp1 | vp2 | vp3 |
      And "binhn"'s last transaction should have succeeded


      # Alice attempts to assign a role to binh, but this will fail as she does NOT have permission.
      When "alice" uses application TCert "TCERT_APP_ALICE_1" to assign role "writer" to application TCert "TCERT_APP_ALICE_1"
      Then I wait up to "60" seconds for transaction to be committed to peers:
         | vp0  | vp1 | vp2 | vp3 |
      And "alice"'s last transaction should have failed with message that contains "The invoker does not have the required roles"

      #When "Alice" writes value "Alice's value"
      #Then invoke should succeed

      #When "Bob" reads value
      #Then the result should be "Alice's value"

      #When "Alice" reads value
      #Then should fail with failure "Permissed denied"


      # TODO:  Must manage TCert expiration for all parties involved.

    Examples: Consensus Options
        |          ComposeFile                   |   WaitTime    |
        |   docker-compose-4-consensus-batch.yml |      120      |


  #@doNotDecompose
  @issue_RBAC_TCERT_With_Attributes
  Scenario Outline: test a chaincode showing how to implement role-based access control using TCerts with attributes

      Given we compose "<ComposeFile>"
      And I wait "5" seconds
      And I register with CA supplying username "binhn" and secret "7avZQLwcUe9q" on peers:
         | vp0  |
      And I register with CA supplying username "alice" and secret "8Y7WIrLX0A8G" on peers:
         | vp0  |
      And I use the following credentials for querying peers:
         | peer |   username  |    secret    |
         | vp0  |  test_user0 | MS9qrN8hFjlE |
         | vp1  |  test_user1 | jGlNl6ImkuDo |
         | vp2  |  test_user2 | zMflqOKezFiA |
         | vp3  |  test_user3 | vWdLCE00vJy0 |

      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "1"

      #Acquire a batch of TCerts for binh and store the TCertOwner key material associated with the batch. PreK0 which is per TCert. 
      When user "binhn" requests a batch of TCerts of size "1" with attributes:
            | role  |
            | admin |
      And user "binhn" stores their last result as "BATCH_OF_TCERTS"
      # 

      # Deploy, in this case Binh is assinging himself as the Admin for the RBAC chaincode.
      And user "binhn" deploys chaincode "github.com/hyperledger/fabric/examples/chaincode/go/rbac_tcerts_with_attrs" with ctor "init" to "vp0"
            ||
            ||
      Then I should have received a chaincode name
      Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
                 | vp0  | vp1 | vp2 | vp3 |

      # When we refer to any party, we actually mean a specific application TCert for that party

      #Acquire a Application-TCert for alice, and supplies to binhn (NOTE: application TCert is NEVER used for signing fabric TXs) 
      When user "alice" requests a new application TCert
      And user "alice" stores their last result as "TCERT_APP_ALICE_1"
      # Alice gives binhn her application TCert (usually OUT-OF-BAND)
      And user "alice" gives stored value "TCERT_APP_ALICE_1" to "binhn"

      
      # binhn is assigning the role of 'writer' to alice
      When "binhn" uses application TCert "TCERT_APP_ADMIN" to assign role "writer" to application TCert "TCERT_APP_ALICE_1"
      Then I should have received a transactionID
      Then I wait up to "60" seconds for transaction to be committed to peers:
         | vp0  | vp1 | vp2 | vp3 |


      # Alice attempts to assign a role to binh, but this will fail as she does NOT have permission.  The check is currently
      # to make sure the TX does NOT appear on the chain.
      When "alice" uses application TCert "TCERT_APP_ALICE_1" to assign role "writer" to application TCert "TCERT_APP_ALICE_1"
      Then I wait up to "60" seconds for transaction to be committed to peers:
         | vp0  | vp1 | vp2 | vp3 |
      And transaction should have failed with message "Permission denied"


      #When "Administrator" assigns role "reader" to "Bob"
      #Then invoke should succeed

      #When "Bob" assigns role "reader" to "Bob"
      #Then invoke should fail with failure "Permissed denied"

      #When "Alice" writes value "Alice's value"
      #Then invoke should succeed

      #When "Bob" reads value
      #Then the result should be "Alice's value"

      #When "Alice" reads value
      #Then should fail with failure "Permissed denied"


      # TODO:  Must manage TCert expiration for all parties involved.

#      When I invoke chaincode "rbac_tcerts_no_attrs" function name "addRole" on "vp0"
#            |arg1|arg2|arg3|
#            | a  | b  | 20 |
#      Then I should have received a transactionID
#      Then I wait up to "10" seconds for transaction to be committed to peers:
#            | vp0  | vp1 | vp2 | vp3 |


    Examples: Consensus Options
        |          ComposeFile                   |   WaitTime   |
        |   docker-compose-4-consensus-batch.yml |      60      |


  #@doNotDecompose
  @issue_1565
  Scenario Outline: test chaincode to chaincode invocation

      Given we compose "<ComposeFile>"
      And I wait "5" seconds
      And I register with CA supplying username "binhn" and secret "7avZQLwcUe9q" on peers:
         | vp0  |
      And I register with CA supplying username "alice" and secret "CMS10pEQlB16" on peers:
         | vp0  |
      And I use the following credentials for querying peers:
         | peer |   username  |    secret    |
         | vp0  |  test_user0 | MS9qrN8hFjlE |
         | vp1  |  test_user1 | jGlNl6ImkuDo |
         | vp2  |  test_user2 | zMflqOKezFiA |
         | vp3  |  test_user3 | vWdLCE00vJy0 |

      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "1"

      # Deploy the first chaincode
      When user "binhn" deploys chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" aliased as "chaincode_example02" with ctor "init" and args
                    | arg1 |  arg2 | arg3 | arg4 |
                    |  a   |  100  |  b   |  200 |
      Then I should have received a chaincode name
      Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
                 | vp0  | vp1 | vp2 | vp3 |
      And "binhn"'s last transaction should have succeeded

      # Deploy the second chaincode
      When user "binhn" deploys chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example05" aliased as "chaincode_example05" with ctor "init" and args
                    | arg1  |  arg2 |
                    | sum   |   0   |
      Then I should have received a chaincode name
      Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
                 | vp0  | vp1 | vp2 | vp3 |
      And "binhn"'s last transaction should have succeeded


      # Invoke chaincode_example05 which in turn will invoke chaincode_example02.  NOTE: Binhn must pass a reference to the first chaincode
      Given user "binhn" stores a reference to chaincode "chaincode_example02" as "cc1"
      When user "binhn" invokes chaincode "chaincode_example05" function name "invoke" with args
              |  arg1   |  arg2  |
              | {cc1}   |  sum   |
      Then I should have received a transactionID
      Then I wait up to "60" seconds for transaction to be committed to peers:
         | vp0  | vp1 | vp2 | vp3 |
      And "binhn"'s last transaction should have succeeded


    Examples: Consensus Options
        |          ComposeFile                   |   WaitTime   |
        |   docker-compose-4-consensus-batch.yml |      60      |
