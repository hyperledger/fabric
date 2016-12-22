#
# Test Consensus in Fabric Peers
#
# Tags that can be used and will affect test internals:
#  @doNotDecompose will NOT decompose the named compose_yaml after scenario ends.  Useful for setting up environment and reviewing after scenario.
#  @chaincodeImagesUpToDate use this if all scenarios chaincode images are up to date, and do NOT require building.  BE SURE!!!

Feature: Consensus between peers
    As a Fabric developer
    I want to run a network of peers

@scat
  Scenario: chaincode example02 with 4 peers and 1 membersrvc, multiple peers stopped
      Given we compose "docker-compose-4-consensus-batch.yml"
      And I register with CA supplying username "binhn" and secret "7avZQLwcUe9q" on peers:
                     | vp0  |
      And I use the following credentials for querying peers:
         | peer |   username  |    secret    |
         | vp0  |  test_user0 | MS9qrN8hFjlE |
         | vp1  |  test_user1 | jGlNl6ImkuDo |
         | vp2  |  test_user2 | zMflqOKezFiA |
         | vp3  |  test_user3 | vWdLCE00vJy0 |

      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "1"

      # Deploy
      When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
                    | arg1 |  arg2 | arg3 | arg4 |
                    |  a   |  200  |  b   |  300 |
      Then I should have received a chaincode name
      Then I wait up to "60" seconds for transaction to be committed to peers:
                     | vp1 | vp2 | vp3 |

      # Build up a sizable blockchain, that vp3 will need to validate at startup
      When I invoke chaincode "example2" function name "invoke" on "vp0" "30" times
                    |arg1|arg2|arg3|
                    | b  | a  | 1  |
      Then I should have received a transactionID
      Then I wait up to "120" seconds for transaction to be committed to peers:
                    | vp1 | vp2 | vp3 |

      When I query chaincode "example2" function name "query" with value "a" on peers:
                    | vp0  | vp1 | vp2 | vp3 |
      Then I should get a JSON response from peers with "result.message" = "230"
                    | vp0  | vp1 | vp2 | vp3 |

      Given I stop peers:
            | vp3  |

      # Invoke a transaction to get vp3 out of sync
      When I invoke chaincode "example2" function name "invoke" on "vp0"
      |arg1|arg2|arg3|
      | a  | b  | 10 |
      Then I should have received a transactionID
      Then I wait up to "120" seconds for transaction to be committed to peers:
            | vp1 | vp2 |

      When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 |
      Then I should get a JSON response from peers with "result.message" = "220"
            | vp0  | vp1 | vp2 |

      Given I stop peers:
            | vp2  |

      # Invoke a transaction to get vp2 out of sync
      When I invoke chaincode "example2" function name "invoke" on "vp0"
      |arg1|arg2|arg3|
      | a  | b  | 10 |
      Then I should have received a transactionID
#      Then I wait up to "120" seconds for transaction to be committed to peers that fail:
#            | vp1 |

      When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 |
      # Should keep the same value as before
      Then I should get a JSON response from peers with "result.message" = "220"
            | vp0  | vp1 |

      # Now start vp2 again
      Given I start peers:
            | vp2  |
      And I wait "15" seconds

      And I wait "60" seconds
      When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 |
      Then I should get a JSON response from peers with "result.message" = "210"
            | vp0  | vp1 | vp2 |

      # Invoke 6 more txs, this will trigger a state transfer, set a target, and execute new outstanding transactions
      When I invoke chaincode "example2" function name "invoke" on "vp0" "10" times
            |arg1|arg2|arg3|
            | a  | b  | 10 |
      Then I should have received a transactionID
      Then I wait up to "60" seconds for transaction to be committed to peers:
            | vp1 | vp2 |
      # wait a bit longer and let state transfer finish
      Then I wait "60" seconds
      When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 |
      Then I should get a JSON response from peers with "result.message" = "110"
            | vp0  | vp1 | vp2 |

      # Now start vp3 again
      Given I start peers:
            | vp3  |
      And I wait "15" seconds

      # Invoke 10 more txs, this will trigger a state transfer, set a target, and execute new outstanding transactions
      When I invoke chaincode "example2" function name "invoke" on "vp0" "10" times
            |arg1|arg2|arg3|
            | a  | b  | 10 |
      Then I should have received a transactionID
      Then I wait "180" seconds
      When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 |
      Then I should get a JSON response from peers with "result.message" = "10"
            | vp0  | vp1 | vp2 |

      Given I stop peers:
            | vp1  |

      # Invoke a transaction to get vp0 out of sync
      When I invoke chaincode "example2" function name "invoke" on "vp0"
            |arg1|arg2|arg3|
            | a  | b  | 10 |
      Then I should have received a transactionID
      Then I wait up to "120" seconds for transaction to be committed to peers:
            | vp2 | vp3 |

      When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp2 | vp3 |
      Then I should get a JSON response from peers with "result.message" = "0"
            | vp0  | vp2 | vp3 |


#@scat
#@doNotDecompose
#  Scenario: chaincode example02 with 4 peers and 1 membersrvc, upgrade peer code with same DB
#    Given we compose "docker-compose-4-consensus-upgrade.yml"
#    And I register with CA supplying username "binhn" and secret "7avZQLwcUe9q" on peers:
#                                             | vp0  |
#    And I use the following credentials for querying peers:
#                                 | peer |   username  |    secret    |
#                                 | vp0  |  test_user0 | MS9qrN8hFjlE |
#                                 | vp1  |  test_user1 | jGlNl6ImkuDo |
#                                 | vp2  |  test_user2 | zMflqOKezFiA |
#                                 | vp3  |  test_user3 | vWdLCE00vJy0 |
#
#    When requesting "/chain" from "vp0"
#    Then I should get a JSON response with "height" = "1"
#
#    # Deploy
#    When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
#                                            | arg1 |  arg2 | arg3 | arg4 |
#                                            |  a   |  200  |  b   |  300 |
#    Then I should have received a chaincode name
#    Then I wait up to "120" seconds for transaction to be committed to peers:
#                                             | vp1 | vp2 | vp3 |
#
#    # Build up a sizable blockchain, that vp3 will need to validate at startup
#    When I invoke chaincode "example2" function name "invoke" on "vp0" "30" times
#                                            |arg1|arg2|arg3|
#                                            | b  | a  | 1  |
#    Then I should have received a transactionID
#    Then I wait up to "120" seconds for transaction to be committed to peers:
#                                            | vp1 | vp2 | vp3 |
#
#    When I query chaincode "example2" function name "query" with value "a" on peers:
#                                            | vp0  | vp1 | vp2 | vp3 |
#    Then I should get a JSON response from peers with "result.message" = "230"
#                                            | vp0  | vp1 | vp2 | vp3 |
#    And I wait "5" seconds
#
#    When requesting "/chain" from "vp0"
#    Then I should "store" the "height" from the JSON response
#
#    Given I build new images
#    And I fallback using the following credentials
#                |  username  |    secret    |
#                |  test_vp0  | MwYpmSRjupbT |
#                |  test_vp1  | 5wgHK9qqYaPy |
#                |  test_vp2  | vQelbRvja7cJ |
#                |  test_vp3  | 9LKqKH5peurL |
#    And I wait "60" seconds
#
#    When requesting "/chain" from "vp0"
#    Then I should get a JSON response with "height" = "previous"
#
#    Given I use the following credentials for querying peers:
#                | peer |   username  |    secret    |
#                | vp0  |  test_user0 | MS9qrN8hFjlE |
#                | vp1  |  test_user1 | jGlNl6ImkuDo |
#                | vp2  |  test_user2 | zMflqOKezFiA |
#                | vp3  |  test_user3 | vWdLCE00vJy0 |
#
#    When I query chaincode "example2" function name "query" with value "a" on peers:
#                                    | vp0  | vp1 | vp2 |
#    Then I should get a JSON response from peers with "result.message" = "230"
#                                    | vp0  | vp1 | vp2 |
#    And I wait "60" seconds
#
#    When I invoke chaincode "example2" function name "invoke" on "vp0" "30" times
#                                    |arg1|arg2|arg3|
#                                    | b  | a  | 1  |
#    Then I should have received a transactionID
#    Then I wait up to "120" seconds for transaction to be committed to peers:
#                                    | vp1 | vp2 | vp3 |
#
#    When I query chaincode "example2" function name "query" with value "a" on peers:
#                                    | vp0  | vp1 | vp2 | vp3 |
#    Then I should get a JSON response from peers with "result.message" = "260"
#    And I wait "5" seconds
#
#    When requesting "/chain" from "vp0"
#    Then I should "store" the "height" from the JSON response
#
#    Given I upgrade using the following credentials
#                |  username  |    secret    |
#                |  test_vp0  | MwYpmSRjupbT |
#                |  test_vp1  | 5wgHK9qqYaPy |
#                |  test_vp2  | vQelbRvja7cJ |
#                |  test_vp3  | 9LKqKH5peurL |
#    And I wait "60" seconds
#
#    When I query chaincode "example2" function name "query" with value "a" on peers:
#                                    | vp0  | vp1 | vp2 |
#    Then I should get a JSON response from peers with "result.message" = "260"
#                                    | vp0  | vp1 | vp2 |
#
#    When requesting "/chain" from "vp0"
#    Then I should get a JSON response with "height" = "previous"
#
#    When I invoke chaincode "example2" function name "invoke" on "vp0" "30" times
#                                            |arg1|arg2|arg3|
#                                            | b  | a  | 1  |
#    Then I should have received a transactionID
#    Then I wait up to "120" seconds for transaction to be committed to peers:
#                                            | vp1 | vp2 | vp3 |
#
#    When I query chaincode "example2" function name "query" with value "a" on peers:
#                                    | vp0  | vp1 | vp2 | vp3 |
#    Then I should get a JSON response from peers with "result.message" = "290"


@scat
    Scenario: chaincode example02 with 4 peers and 1 membersrvc, stop vp1

        Given we compose "docker-compose-4-consensus-batch.yml"
        And I register with CA supplying username "binhn" and secret "7avZQLwcUe9q" on peers:
            | vp0  |
        And I use the following credentials for querying peers:
            | peer |   username  |    secret    |
            | vp0  |  test_user0 | MS9qrN8hFjlE |
            | vp1  |  test_user1 | jGlNl6ImkuDo |
            | vp2  |  test_user2 | zMflqOKezFiA |
            | vp3  |  test_user3 | vWdLCE00vJy0 |

        When requesting "/chain" from "vp0"
        Then I should get a JSON response with "height" = "1"

        # Deploy
        When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
            | arg1 |  arg2 | arg3 | arg4 |
            |  a   |  100  |  b   |  200 |
        Then I should have received a chaincode name
        Then I wait up to "60" seconds for transaction to be committed to peers:
            | vp1 | vp2 | vp3 |

        Given I stop peers:
            | vp1 |

        # Execute one request to get vp1 out of sync
        When I invoke chaincode "example2" function name "invoke" on "vp0"
            |arg1|arg2|arg3|
            | b  | a  | 1  |
        Then I should have received a transactionID
        Then I wait up to "60" seconds for transaction to be committed to peers:
            | vp2 | vp3 |

        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp2 | vp3 |
        Then I should get a JSON response from peers with "result.message" = "101"
            | vp0  | vp2 | vp3 |

        # Now start vp1 again
        Given I start peers:
            | vp1 |

        # Invoke some more txs, this will trigger a state transfer, but it cannot complete
        When I invoke chaincode "example2" function name "invoke" on "vp0" "8" times
            |arg1|arg2|arg3|
            | a  | b  | 10 |
        Then I should have received a transactionID
        Then I wait up to "60" seconds for transaction to be committed to peers:
            | vp2 | vp3 |
        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp2 | vp3 |
        Then I should get a JSON response from peers with "result.message" = "21"

        # Force vp1 to attempt to sync with the rest of the peers
        When I invoke chaincode "example2" function name "invoke" on "vp1"
            |arg1|arg2|arg3|
            | a  | b  | 10 |
        And I unconditionally query chaincode "example2" function name "query" with value "a" on peers:
            | vp1  |
        Then I should get a JSON response from peers with "error.data" = "Error when querying chaincode: Error: state may be inconsistent, cannot query"
            | vp1  |


@scat
  Scenario: Peers catch up only when necessary
    Given we compose "docker-compose-4-consensus-upgrade.yml"
    And I register with CA supplying username "binhn" and secret "7avZQLwcUe9q" on peers:
                                 | vp0  |
    And I use the following credentials for querying peers:
                                 | peer |   username  |    secret    |
                                 | vp0  |  test_user0 | MS9qrN8hFjlE |
                                 | vp1  |  test_user1 | jGlNl6ImkuDo |
                                 | vp2  |  test_user2 | zMflqOKezFiA |
                                 | vp3  |  test_user3 | vWdLCE00vJy0 |

    When requesting "/network/peers" from "vp0"
    Then I should get a JSON response with array "peers" contains "4" elements

    # Deploy
    When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
                                            | arg1 |  arg2 | arg3 | arg4 |
                                            |  a   |  200  |  b   |  300 |
    Then I should have received a chaincode name
    Then I wait up to "240" seconds for transaction to be committed to peers:
                                             | vp0  | vp1 | vp2 | vp3 |

    # Build up a sizable blockchain, that vp3 will need to validate at startup
    When I invoke chaincode "example2" function name "invoke" on "vp0" "30" times
                                            |arg1|arg2|arg3|
                                            | b  | a  | 1  |
    Then I should have received a transactionID
    Then I wait up to "240" seconds for transaction to be committed to peers:
                                            | vp0  | vp1 | vp2 | vp3 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
                                            | vp0  | vp1 | vp2 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "230"
                                            | vp0  | vp1 | vp2 | vp3 |
    And I wait "5" seconds

    Given I stop peers:
            | vp1 |

    # Invoke a transaction to get vp1 out of sync
    When I invoke chaincode "example2" function name "invoke" on "vp0" "1" times
            |arg1|arg2|arg3|
            | a  | b  | 10  |
    Then I should have received a transactionID
    Then I wait up to "240" seconds for transaction to be committed to peers:
            | vp0  | vp2 | vp3 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp2 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "220"
            | vp0  | vp2 | vp3 |

    When requesting "/chain" from "vp0"
    Then I should "store" the "height" from the JSON response
    When requesting "/chain" from "vp2"
    Then I should get a JSON response with "height" = "previous"
    When requesting "/chain" from "vp3"
    Then I should get a JSON response with "height" = "previous"

    Given I start peers:
            | vp1 |
    And I wait "30" seconds

    When I invoke chaincode "example2" function name "invoke" on "vp0" "10" times
            |arg1|arg2|arg3|
            | a  | b  | 1  |
    Then I should have received a transactionID
    Then I wait up to "240" seconds for transaction to be committed to peers:
            | vp0  | vp2 | vp3 |
    And I wait "60" seconds

    When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp2 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "210"
            | vp0  | vp2 | vp3 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp1 |
    Then I should get a JSON response from peers with "result.message" = "220"
            | vp1 |


#@doNotDecompose
  Scenario: 16 peer network - basic consensus
    Given we compose "docker-compose-16-consensus.yml"
    And I register with CA supplying username "binhn" and secret "7avZQLwcUe9q" on peers:
                                 | vp0  |
    And I use the following credentials for querying peers:
                                 | peer  |   username   |    secret    |
                                 | vp0   |  test_user0  | MS9qrN8hFjlE |
                                 | vp1   |  test_user1  | jGlNl6ImkuDo |
                                 | vp2   |  test_user2  | zMflqOKezFiA |
                                 | vp3   |  test_user3  | vWdLCE00vJy0 |
                                 | vp4   |  test_user4  | 4nXSrfoYGFCP |
                                 | vp5   |  test_user5  | yg5DVhm0er1z |
                                 | vp6   |  test_user6  | b7pmSxzKNFiw |
                                 | vp7   |  test_user7  | YsWZD4qQmYxo |
                                 | vp8   |  test_user8  | W8G0usrU7jRk |
                                 | vp9   |  test_user9  | H80SiB5ODKKQ |
                                 | vp10  |  test_user10 | n21Dq435t9S1 |
                                 | vp11  |  test_user11 | 6S0UjokSRHYh |
                                 | vp12  |  test_user12 | dpodq6r2+NPu |
                                 | vp13  |  test_user13 | 9XZFoBjXJ5zM |
                                 | vp14  |  test_user14 | 6lOOiQXW5uXM |
                                 | vp15  |  test_user15 | PTyW9AVbBSjk |

    When requesting "/network/peers" from "vp0"
    Then I should get a JSON response with array "peers" contains "16" elements

    # Deploy
    When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
                                            | arg1 |  arg2 | arg3 | arg4 |
                                            |  a   |  200  |  b   |  300 |
    Then I should have received a chaincode name
    Then I wait up to "240" seconds for transaction to be committed to peers:
           | vp1 | vp2 | vp3 | vp4 | vp5 | vp6 | vp7 | vp8 | vp9 | vp10 | vp11 | vp12 | vp13 | vp14 | vp15 |

    When I invoke chaincode "example2" function name "invoke" on "vp0" "10" times
                       |arg1|arg2|arg3|
                       | b  | a  | 1  |
    Then I should have received a transactionID
    Then I wait up to "240" seconds for transaction to be committed to peers:
           | vp1 | vp2 | vp3 | vp4 | vp5 | vp6 | vp7 | vp8 | vp9 | vp10 | vp11 | vp12 | vp13 | vp14 | vp15 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
           | vp0  | vp1 | vp2 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "210"
           | vp0  | vp1 | vp2 | vp3 |
    And I wait "15" seconds

    When requesting "/chain" from "vp0"
    Then I should "store" the "height" from the JSON response
    Given I stop peers:
            | vp1 | vp2 | vp3 | vp4 | vp5 | vp6 |

    When I invoke chaincode "example2" function name "invoke" on "vp0" "5" times
            |arg1|arg2|arg3|
            | a  | b  | 1 |
    And I wait "5" seconds

    When requesting "/chain" from "vp0"
    Then I should get a JSON response with "height" = "previous"

    When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0 | vp7 | vp8 | vp9 | vp10 | vp11 | vp12 | vp13 | vp14 | vp15 |
    Then I should get a JSON response from peers with "result.message" = "210"
            | vp0 | vp7 | vp8 | vp9 | vp10 | vp11 | vp12 | vp13 | vp14 | vp15 |

    Given I start peers:
            | vp3 |
    And I wait "15" seconds

    When I invoke chaincode "example2" function name "invoke" on "vp0"
            |arg1|arg2|arg3|
            | a  | b  | 10 |
    Then I should have received a transactionID
    Then I wait up to "120" seconds for transaction to be committed to peers:
           | vp0 | vp3 | vp8 | vp9 | vp10 | vp11 | vp12 | vp13 | vp14 | vp15 |
    When I query chaincode "example2" function name "query" with value "a" on peers:
           | vp0  | vp3 | vp8 | vp9 | vp10 | vp11 | vp12 | vp13 | vp14 | vp15 |
    Then I should get a JSON response from peers with "result.message" = "200"
           | vp0  | vp3 | vp8 | vp9 | vp10 | vp11 | vp12 | vp13 | vp14 | vp15 |

    When requesting "/chain" from "vp0"
    Then I should get a JSON response with "height" > "previous"


#@doNotDecompose
  Scenario: Take down the chaincode!
    Given we compose "docker-compose-4-consensus-upgrade.yml"
    And I register with CA supplying username "binhn" and secret "7avZQLwcUe9q" on peers:
                                             | vp0  |
    And I use the following credentials for querying peers:
                                 | peer |   username  |    secret    |
                                 | vp0  |  test_user0 | MS9qrN8hFjlE |
                                 | vp1  |  test_user1 | jGlNl6ImkuDo |
                                 | vp2  |  test_user2 | zMflqOKezFiA |
                                 | vp3  |  test_user3 | vWdLCE00vJy0 |

    When requesting "/chain" from "vp0"
    Then I should get a JSON response with "height" = "1"

    # Deploy
    When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
                                            | arg1 |  arg2 | arg3 | arg4 |
                                            |  a   |  200  |  b   |  300 |
    Then I should have received a chaincode name
    Then I wait up to "120" seconds for transaction to be committed to peers:
                                             | vp0  | vp1 | vp2 | vp3 |

    # Build up a sizable blockchain, that vp3 will need to validate at startup
    When I invoke chaincode "example2" function name "invoke" on "vp0" "30" times
                                            |arg1|arg2|arg3|
                                            | b  | a  | 1  |
    Then I should have received a transactionID
    Then I wait up to "120" seconds for transaction to be committed to peers:
                                            | vp0  | vp1 | vp2 | vp3 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
                                            | vp0  | vp1 | vp2 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "230"
                                            | vp0  | vp1 | vp2 | vp3 |
    And I wait "5" seconds

    Given I stop the chaincode
    And I remove the chaincode images

    When I query chaincode "example2" function name "query" with value "a" on peers:
                                            | vp0  | vp1 | vp2 | vp3 |
    Then I wait "30" seconds
    And I should get a JSON response from peers with "result.message" = "230"


#@doNotDecompose
  Scenario: Take down the chaincode and stop the peers!
    Given we compose "docker-compose-4-consensus-upgrade.yml"
    And I register with CA supplying username "binhn" and secret "7avZQLwcUe9q" on peers:
                                             | vp0  |
    And I use the following credentials for querying peers:
                                 | peer |   username  |    secret    |
                                 | vp0  |  test_user0 | MS9qrN8hFjlE |
                                 | vp1  |  test_user1 | jGlNl6ImkuDo |
                                 | vp2  |  test_user2 | zMflqOKezFiA |
                                 | vp3  |  test_user3 | vWdLCE00vJy0 |

    When requesting "/chain" from "vp0"
    Then I should get a JSON response with "height" = "1"

    # Deploy
    When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
                                            | arg1 |  arg2 | arg3 | arg4 |
                                            |  a   |  200  |  b   |  300 |
    Then I should have received a chaincode name
    Then I wait up to "120" seconds for transaction to be committed to peers:
                                             | vp0  | vp1 | vp2 | vp3 |

    # Build up a sizable blockchain, that vp3 will need to validate at startup
    When I invoke chaincode "example2" function name "invoke" on "vp0" "30" times
                                            |arg1|arg2|arg3|
                                            | b  | a  | 1  |
    Then I should have received a transactionID
    Then I wait up to "120" seconds for transaction to be committed to peers:
                                            | vp0  | vp1 | vp2 | vp3 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
                                            | vp0  | vp1 | vp2 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "230"
                                            | vp0  | vp1 | vp2 | vp3 |
    And I wait "5" seconds

    Given I stop the chaincode
    And I remove the chaincode images
    And I stop peers:
            | vp0 | vp1 | vp2 | vp3 |
    And I wait "15" seconds
    And I start peers:
            | vp0 | vp1 | vp2 | vp3 |
    And I wait "30" seconds

    When I query chaincode "example2" function name "query" with value "a" on peers:
                                            | vp0  | vp1 | vp2 | vp3 |
    Then I wait "30" seconds
    And I should get a JSON response from peers with "result.message" = "230"
                                            | vp0  | vp1 | vp2 | vp3 |


#@doNotDecompose
  Scenario: Take down the chaincode and the peers!
    Given we compose "docker-compose-4-consensus-upgrade.yml"
    And I register with CA supplying username "binhn" and secret "7avZQLwcUe9q" on peers:
                                             | vp0  |
    And I use the following credentials for querying peers:
                                 | peer |   username  |    secret    |
                                 | vp0  |  test_user0 | MS9qrN8hFjlE |
                                 | vp1  |  test_user1 | jGlNl6ImkuDo |
                                 | vp2  |  test_user2 | zMflqOKezFiA |
                                 | vp3  |  test_user3 | vWdLCE00vJy0 |

    When requesting "/chain" from "vp0"
    Then I should get a JSON response with "height" = "1"

    # Deploy
    When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
                                            | arg1 |  arg2 | arg3 | arg4 |
                                            |  a   |  200  |  b   |  300 |
    Then I should have received a chaincode name
    Then I wait up to "120" seconds for transaction to be committed to peers:
                                             | vp0  | vp1 | vp2 | vp3 |

    # Build up a sizable blockchain, that vp3 will need to validate at startup
    When I invoke chaincode "example2" function name "invoke" on "vp0" "30" times
                                            |arg1|arg2|arg3|
                                            | b  | a  | 1  |
    Then I should have received a transactionID
    Then I wait up to "120" seconds for transaction to be committed to peers:
                                            | vp0  | vp1 | vp2 | vp3 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
                                            | vp0  | vp1 | vp2 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "230"
                                            | vp0  | vp1 | vp2 | vp3 |
    And I wait "5" seconds

    When requesting "/chain" from "vp0"
    Then I should "store" the "height" from the JSON response

    Given I stop the chaincode
    And I remove the chaincode images
    And I stop peers:
            | vp0 | vp1 | vp2 | vp3 |
    And I wait "15" seconds
    And we compose "docker-compose-4-consensus-newpeers_w_upgrade.yml"
    And I wait "30" seconds

    When requesting "/chain" from "vp0"
    Then I should get a JSON response with "height" = "previous"

    Given I use the following credentials for querying peers:
                                 | peer |   username  |    secret    |
                                 | vp0  |  test_user0 | MS9qrN8hFjlE |
                                 | vp1  |  test_user1 | jGlNl6ImkuDo |
                                 | vp2  |  test_user2 | zMflqOKezFiA |
                                 | vp3  |  test_user3 | vWdLCE00vJy0 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
                                            | vp0  | vp1 | vp2 | vp3 |
    Then I wait "30" seconds
    And I should get a JSON response from peers with "result.message" = "230"
                                            | vp0  | vp1 | vp2 | vp3 |
