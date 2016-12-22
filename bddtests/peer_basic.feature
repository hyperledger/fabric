#
# Test Fabric Peers
#
# Tags that can be used and will affect test internals:
#  @doNotDecompose will NOT decompose the named compose_yaml after scenario ends.  Useful for setting up environment and reviewing after scenario.
#  @chaincodeImagesUpToDate use this if all scenarios chaincode images are up to date, and do NOT require building.  BE SURE!!!

#@chaincodeImagesUpToDate
Feature: Network of Peers
    As a Fabric developer
    I want to run a network of peers

#    @wip
@smoke
  Scenario: Peers list test, single peer issue #827
    Given we compose "docker-compose-1.yml"
      When requesting "/network/peers" from "vp0"
      Then I should get a JSON response with array "peers" contains "1" elements

#    @wip
  Scenario: Peers list test,3 peers issue #827
    Given we compose "docker-compose-3.yml"
      When requesting "/network/peers" from "vp0"
      Then I should get a JSON response with array "peers" contains "3" elements

#    @doNotDecompose
    @wip
   @issue_767
  Scenario: Range query test, single peer, issue #767
    Given we compose "docker-compose-1.yml"
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "1"
      When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/map" with ctor "init" to "vp0"
      ||
      ||

      Then I should have received a chaincode name
      Then I wait up to "60" seconds for transaction to be committed to all peers

      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "2"

      When I invoke chaincode "map" function name "put" on "vp0"
        | arg1 | arg2 |
        | key1  | value1  |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers

      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "3"

      When I query chaincode "map" function name "get" on "vp0":
        | arg1|
        | key1 |
      Then I should get a JSON response with "result.message" = "value1"

      When I invoke chaincode "map" function name "put" on "vp0"
        | arg1 | arg2 |
        | key2  | value2  |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers

      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "4"

      When I query chaincode "map" function name "keys" on "vp0":
        ||
        ||
      Then I should get a JSON response with "result.message" = "["key1","key2"]"

      When I invoke chaincode "map" function name "remove" on "vp0"
        | arg1 | |
        | key1  | |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers

      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "5"

      When I query chaincode "map" function name "keys" on "vp0":
        ||
        ||
      Then I should get a JSON response with "result.message" = "["key2"]"

# @doNotDecompose
  @wip
  @issue_477
  Scenario: chaincode shim table API, issue 477
    Given we compose "docker-compose-1.yml"
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "1"
      When I deploy chaincode "github.com/hyperledger/fabric/bddtests/chaincode/go/table" with ctor "init" to "vp0"
      ||
      ||
      Then I should have received a chaincode name
      Then I wait up to "60" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "2"

      When I invoke chaincode "table_test" function name "insertRowTableOne" on "vp0"
        | arg1 | arg2 | arg3 |
        | test1| 10   | 20   |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "3"

      When I invoke chaincode "table_test" function name "insertRowTableOne" on "vp0"
        | arg1 | arg2 | arg3 |
        | test2| 10   | 20   |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "4"

      When I query chaincode "table_test" function name "getRowTableOne" on "vp0":
        | arg1 |
        | test1|
      Then I should get a JSON response with "result.message" = "{[string:"test1"  int32:10  int32:20 ]}"

      When I invoke chaincode "table_test" function name "insertRowTableTwo" on "vp0"
        | arg1 | arg2 | arg3 | arg3 |
        | foo2 | 34   | 65   | bar8 |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "5"

      When I query chaincode "table_test" function name "getRowTableTwo" on "vp0":
        | arg1 | arg2 | arg3 |
        | foo2 | 65   | bar8 |
      Then I should get a JSON response with "result.message" = "{[string:"foo2"  int32:34  int32:65  string:"bar8" ]}"

      When I invoke chaincode "table_test" function name "replaceRowTableOne" on "vp0"
        | arg1 | arg2 | arg3 |
        | test1| 30   | 40   |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "6"

      When I query chaincode "table_test" function name "getRowTableOne" on "vp0":
        | arg1 |
        | test1|
      Then I should get a JSON response with "result.message" = "{[string:"test1"  int32:30  int32:40 ]}"

      When I invoke chaincode "table_test" function name "deleteRowTableOne" on "vp0"
        | arg1 |
        | test1|
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "7"

      When I query chaincode "table_test" function name "getRowTableOne" on "vp0":
        | arg1 |
        | test1|
      Then I should get a JSON response with "result.message" = "{[]}"

      When I query chaincode "table_test" function name "getRowTableOne" on "vp0":
        | arg1 |
        | test2|
      Then I should get a JSON response with "result.message" = "{[string:"test2"  int32:10  int32:20 ]}"

      When I invoke chaincode "table_test" function name "insertRowTableOne" on "vp0"
        | arg1 | arg2 | arg3 |
        | test3| 10   | 20   |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "8"

      When I invoke chaincode "table_test" function name "insertRowTableOne" on "vp0"
        | arg1 | arg2 | arg3 |
        | test4| 10   | 20   |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "9"

      When I invoke chaincode "table_test" function name "insertRowTableOne" on "vp0"
        | arg1 | arg2 | arg3 |
        | test5| 10   | 20   |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "10"

      When I query chaincode "table_test" function name "getRowTableOne" on "vp0":
        | arg1 |
        | test3|
      Then I should get a JSON response with "result.message" = "{[string:"test3"  int32:10  int32:20 ]}"

      When I query chaincode "table_test" function name "getRowTableOne" on "vp0":
        | arg1 |
        | test4|
      Then I should get a JSON response with "result.message" = "{[string:"test4"  int32:10  int32:20 ]}"

      When I query chaincode "table_test" function name "getRowTableOne" on "vp0":
        | arg1 |
        | test5|
      Then I should get a JSON response with "result.message" = "{[string:"test5"  int32:10  int32:20 ]}"

      When I invoke chaincode "table_test" function name "insertRowTableTwo" on "vp0"
        | arg1 | arg2 | arg3 | arg3 |
        | foo2 | 35   | 65   | bar10 |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "11"

      When I invoke chaincode "table_test" function name "insertRowTableTwo" on "vp0"
        | arg1 | arg2 | arg3 | arg3 |
        | foo2 | 36   | 65   | bar11 |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "12"

      When I invoke chaincode "table_test" function name "insertRowTableTwo" on "vp0"
        | arg1 | arg2 | arg3 | arg3 |
        | foo2 | 37   | 65   | bar12 |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "13"

      When I invoke chaincode "table_test" function name "insertRowTableTwo" on "vp0"
        | arg1 | arg2 | arg3 | arg3 |
        | foo2 | 38   | 66   | bar10 |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "14"

      When I query chaincode "table_test" function name "getRowsTableTwo" on "vp0":
        | arg1 | arg2 |
        | foo2 | 65   |
      Then I should get a JSON response with "result.message" = "[{"columns":[{"Value":{"String_":"foo2"}},{"Value":{"Int32":37}},{"Value":{"Int32":65}},{"Value":{"String_":"bar12"}}]},{"columns":[{"Value":{"String_":"foo2"}},{"Value":{"Int32":34}},{"Value":{"Int32":65}},{"Value":{"String_":"bar8"}}]},{"columns":[{"Value":{"String_":"foo2"}},{"Value":{"Int32":36}},{"Value":{"Int32":65}},{"Value":{"String_":"bar11"}}]},{"columns":[{"Value":{"String_":"foo2"}},{"Value":{"Int32":35}},{"Value":{"Int32":65}},{"Value":{"String_":"bar10"}}]}]"

      When I query chaincode "table_test" function name "getRowsTableTwo" on "vp0":
        | arg1 | arg2 |
        | foo2 | 66   |
      Then I should get a JSON response with "result.message" = "[{"columns":[{"Value":{"String_":"foo2"}},{"Value":{"Int32":38}},{"Value":{"Int32":66}},{"Value":{"String_":"bar10"}}]}]"

      When I query chaincode "table_test" function name "getRowsTableTwo" on "vp0":
        | arg1 |
        | foo2 |
      Then I should get a JSON response with "result.message" = "[{"columns":[{"Value":{"String_":"foo2"}},{"Value":{"Int32":37}},{"Value":{"Int32":65}},{"Value":{"String_":"bar12"}}]},{"columns":[{"Value":{"String_":"foo2"}},{"Value":{"Int32":34}},{"Value":{"Int32":65}},{"Value":{"String_":"bar8"}}]},{"columns":[{"Value":{"String_":"foo2"}},{"Value":{"Int32":36}},{"Value":{"Int32":65}},{"Value":{"String_":"bar11"}}]},{"columns":[{"Value":{"String_":"foo2"}},{"Value":{"Int32":38}},{"Value":{"Int32":66}},{"Value":{"String_":"bar10"}}]},{"columns":[{"Value":{"String_":"foo2"}},{"Value":{"Int32":35}},{"Value":{"Int32":65}},{"Value":{"String_":"bar10"}}]}]"

      When I invoke chaincode "table_test" function name "deleteAndRecreateTableOne" on "vp0"
        ||
        ||
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "15"

      When I query chaincode "table_test" function name "getRowTableOne" on "vp0":
        | arg1 |
        | test3|
      Then I should get a JSON response with "result.message" = "{[]}"

      When I query chaincode "table_test" function name "getRowTableOne" on "vp0":
        | arg1 |
        | test4|
      Then I should get a JSON response with "result.message" = "{[]}"

      When I query chaincode "table_test" function name "getRowTableOne" on "vp0":
        | arg1 |
        | test5|
      Then I should get a JSON response with "result.message" = "{[]}"

      When I query chaincode "table_test" function name "getRowTableOne" on "vp0":
        | arg1 |
        | test2|
      Then I should get a JSON response with "result.message" = "{[]}"

      When I query chaincode "table_test" function name "getRowTableTwo" on "vp0":
        | arg1 | arg2 | arg3 |
        | foo2 | 65   | bar8 |
      Then I should get a JSON response with "result.message" = "{[string:"foo2"  int32:34  int32:65  string:"bar8" ]}"

      When I invoke chaincode "table_test" function name "insertRowTableThree" on "vp0"
        | arg1 | arg2 | arg3 | arg4 | arg5 | arg6 | arg7 |
        | foo2 | -38  | -66  | 77   | 88   | hello| true |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "16"

      When I query chaincode "table_test" function name "getRowTableThree" on "vp0":
        | arg1 |
        | foo2 |
      Then I should get a JSON response with "result.message" = "{[string:"foo2"  int32:-38  int64:-66  uint32:77  uint64:88  bytes:"hello"  bool:true ]}"

      When I invoke chaincode "table_test" function name "insertRowTableFour" on "vp0"
        | arg1   |
        | foobar |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "17"

      When I query chaincode "table_test" function name "getRowTableFour" on "vp0":
        | arg1   |
        | foobar |
      Then I should get a JSON response with "result.message" = "{[string:"foobar" ]}"

      When I query chaincode "table_test" function name "getRowsTableFour" on "vp0":
        | arg1   |
        | foobar |
      Then I should get a JSON response with "result.message" = "[{"columns":[{"Value":{"String_":"foobar"}}]}]"

@doNotDecompose
#    @wip
	Scenario: chaincode example 01 single peer erroneous TX
	    Given we compose "docker-compose-1.yml"
	    When requesting "/chain" from "vp0"
	    Then I should get a JSON response with "height" = "1"
	    When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01" with ctor "init" to "vp0"
		     | arg1 |  arg2 | arg3 | arg4 |
		     |  a   |  100  |  b   |  200 |
	    Then I should have received a chaincode name
	    Then I wait up to "60" seconds for transaction to be committed to all peers

	    When requesting "/chain" from "vp0"
	    Then I should get a JSON response with "height" = "2"

        When I invoke chaincode "example1" function name "invoke" on "vp0"
			|arg1|
			| 1  |
	    Then I should have received a transactionID
	    Then I wait up to "25" seconds for transaction to be committed to all peers

	    When requesting "/chain" from "vp0"
	    Then I should get a JSON response with "height" = "3"
	    When requesting "/chain/blocks/2" from "vp0"
	    Then I should get a JSON response containing "transactions" attribute

        When I invoke chaincode "example1" function name "invoke" on "vp0"
			|arg1|
			| a  |
	    Then I should have received a transactionID
	    Then I wait "10" seconds
        When requesting "/chain" from "vp0"
	    Then I should get a JSON response with "height" = "4"
        When requesting "/chain/blocks/3" from "vp0"
	    Then I should get a JSON response containing no "transactions" attribute

#    @doNotDecompose
#    @wip
  @devops
	Scenario: chaincode map single peer content generated ID
	    Given we compose "docker-compose-1.yml"
	    When requesting "/chain" from "vp0"
	    Then I should get a JSON response with "height" = "1"
	    When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/map" with ctor "init" to "vp0"
	      ||
              ||
	    Then I should have received a chaincode name
	    Then I wait up to "60" seconds for transaction to be committed to all peers

        When I invoke chaincode "map" function name "put" on "vp0" with "sha256"
	    | arg1  |arg2|
            |   a   | 10 |
	    Then I should have received a transactionID
	    Then I wait up to "25" seconds for transaction to be committed to all peers
	    Then I check the transaction ID if it is "73b88d92d86502eb66288f84fafae848fed0d21790f2ef1475850f4b635c47f0"

    Scenario: chaincode example 01 single peer rejection message
	    Given we compose "docker-compose-1-exp.yml"
	    Given I start a listener
	    Then I wait "5" seconds

	    When requesting "/chain" from "vp0"
	    Then I should get a JSON response with "height" = "1"
	    When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01" with ctor "init" to "vp0"
		     | arg1 |  arg2 | arg3 | arg4 |
		     |  a   |  100  |  b   |  200 |
	    Then I should have received a chaincode name
	    Then I wait up to "60" seconds for transaction to be committed to all peers

        When I invoke chaincode "example1" function name "invoke" on "vp0"
			|arg1|
			| a  |
	    Then I should have received a transactionID
	    Then I wait "10" seconds

	Then I should get a rejection message in the listener after stopping it

#    @doNotDecompose
#    @wip
@smoke
	Scenario: chaincode example 02 single peer
	    Given we compose "docker-compose-1.yml"
	    When requesting "/chain" from "vp0"
	    Then I should get a JSON response with "height" = "1"
	    When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
		     | arg1 |  arg2 | arg3 | arg4 |
		     |  a   |  100  |  b   |  200 |
	    Then I should have received a chaincode name
	    Then I wait up to "60" seconds for transaction to be committed to all peers

	    When requesting "/chain" from "vp0"
	    Then I should get a JSON response with "height" = "2"

        When I query chaincode "example2" function name "query" on "vp0":
            |arg1|
            |  a |
	    Then I should get a JSON response with "result.message" = "100"


        When I invoke chaincode "example2" function name "invoke" on "vp0"
			|arg1|arg2|arg3|
			| a  | b  | 10 |
	    Then I should have received a transactionID
	    Then I wait up to "25" seconds for transaction to be committed to all peers

	    When requesting "/chain" from "vp0"
	    Then I should get a JSON response with "height" = "3"

        When I query chaincode "example2" function name "query" on "vp0":
            |arg1|
            |  a |
	    Then I should get a JSON response with "result.message" = "90"

        When I query chaincode "example2" function name "query" on "vp0":
            |arg1|
            |  b |
	    Then I should get a JSON response with "result.message" = "210"

#    @doNotDecompose
#    @wip
	Scenario: chaincode example02 with 5 peers, issue #520
	    Given we compose "docker-compose-5.yml"
	    When requesting "/chain" from "vp0"
	    Then I should get a JSON response with "height" = "1"

	    When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
		     | arg1 |  arg2 | arg3 | arg4 |
		     |  a   |  100  |  b   |  200 |
	    Then I should have received a chaincode name
	    Then I wait up to "60" seconds for transaction to be committed to all peers

        When I query chaincode "example2" function name "query" on all peers:
            |arg1|
            |  a |
	    Then I should get a JSON response from all peers with "result.message" = "100"

        When I invoke chaincode "example2" function name "invoke" on "vp0"
			|arg1|arg2|arg3|
			| a  | b  | 20 |
	    Then I should have received a transactionID
	    Then I wait up to "20" seconds for transaction to be committed to all peers

        When I query chaincode "example2" function name "query" on all peers:
            |arg1|
            |  a |
	    Then I should get a JSON response from all peers with "result.message" = "80"


#    @doNotDecompose
#    @wip
@scat
@smoke
    @issue_567
    Scenario Outline: chaincode example02 with 4 peers and 1 membersrvc, issue #567

        Given we compose "<ComposeFile>"
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
        When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
            | arg1 |  arg2 | arg3 | arg4 |
            |  a   |  100  |  b   |  200 |
        Then I should have received a chaincode name
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 | vp3 |

        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 | vp3 |
        Then I should get a JSON response from peers with "result.message" = "100"
            | vp0  | vp1 | vp2 | vp3 |

        When I invoke chaincode "example2" function name "invoke" on "vp0"
            |arg1|arg2|arg3|
            | a  | b  | 20 |
        Then I should have received a transactionID
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 | vp3 |

        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 | vp3 |
        Then I should get a JSON response from peers with "result.message" = "80"
            | vp0  | vp1 | vp2 | vp3 |

    Examples: Consensus Options
        |          ComposeFile                     |   WaitTime   |
        |   docker-compose-4-consensus-noops.yml   |      60      |
        |   docker-compose-4-consensus-batch.yml   |      60      |


@scat
    @issue_680
    @fab380
    Scenario Outline: chaincode example02 with 4 peers and 1 membersrvc, issue #680 (State transfer)

        Given we compose "<ComposeFile>"
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
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 | vp3 |

        # Build up a sizable blockchain, that vp3 will need to validate at startup
        When I invoke chaincode "example2" function name "invoke" on "vp0" "30" times
            |arg1|arg2|arg3|
            | b  | a  | 1  |
        Then I should have received a transactionID
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 | vp3 |

        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 | vp3 |
        Then I should get a JSON response from peers with "result.message" = "130"
            | vp0  | vp1 | vp2 | vp3 |

        Given I stop peers:
            | vp3 |

        # Invoke a transaction to get vp3 out of sync
        When I invoke chaincode "example2" function name "invoke" on "vp0"
            |arg1|arg2|arg3|
            | a  | b  | 10 |
        Then I should have received a transactionID
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 |

        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0 | vp1 | vp2 |
        Then I should get a JSON response from peers with "result.message" = "120"
            | vp0 | vp1 | vp2 |

        # Now start vp3 again
        Given I start peers, waiting up to "15" seconds for them to be ready:
            | vp3 |

        # Invoke 10 more txs, this will trigger a state transfer, set a target, and execute new outstanding transactions
        When I invoke chaincode "example2" function name "invoke" on "vp0" "10" times
            |arg1|arg2|arg3|
            | a  | b  | 10 |
        Then I should have received a transactionID
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 | vp3 |
        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 | vp3 |
        Then I should get a JSON response from peers with "result.message" = "20"
            | vp0  | vp1 | vp2 | vp3 |


    Examples: Consensus Options
        |          ComposeFile                                                                            |   WaitTime   |
        |   docker-compose-4-consensus-batch.yml                                                          |      60      |
        |   docker-compose-4-consensus-batch.yml docker-compose-4-consensus-batch-nosnapshotbuffer.yml    |      60      |


#    @doNotDecompose
@scat
    @issue_724
    Scenario Outline: chaincode example02 with 4 peers and 1 membersrvc, issue #724

        Given we compose "<ComposeFile>"
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
        When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
            | arg1 |  arg2 | arg3 | arg4 |
            |  a   |  100  |  b   |  200 |
        Then I should have received a chaincode name
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 | vp3 |


        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 | vp3 |
        Then I should get a JSON response from peers with "result.message" = "100"
            | vp0  | vp1 | vp2 | vp3 |

        Given I stop peers:
            | vp0  |  vp1   | vp2  | vp3  |

        Given I start peers, waiting up to "15" seconds for them to be ready:
            | vp0  |  vp1   | vp2  | vp3  |

        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp3  |
        Then I should get a JSON response from peers with "result.message" = "100"
            | vp3  |

    Examples: Consensus Options
        |          ComposeFile                     |   WaitTime   |
        |   docker-compose-4-consensus-noops.yml   |      60      |


    Scenario: basic startup of 3 validating peers
        Given we compose "docker-compose-3.yml"
        When requesting "/chain" from "vp0"
        Then I should get a JSON response with "height" = "1"

    @TLS
    Scenario: basic startup of 2 validating peers using TLS
        Given we compose "docker-compose-2-tls-basic.yml"
        When requesting "/chain" from "vp0"
        Then I should get a JSON response with "height" = "1"


@scat
@smoke
    Scenario Outline: 4 peers and 1 membersrvc, consensus still works if one backup replica fails

        Given we compose "<ComposeFile>"
        And I use the following credentials for querying peers:
            | peer |   username  |    secret    |
            | vp0  |  test_user0 | MS9qrN8hFjlE |
            | vp1  |  test_user1 | jGlNl6ImkuDo |
            | vp2  |  test_user2 | zMflqOKezFiA |
            | vp3  |  test_user3 | vWdLCE00vJy0 |
        And I register with CA supplying username "test_user0" and secret "MS9qrN8hFjlE" on peers:
            | vp0 |

        When requesting "/chain" from "vp0"
        Then I should get a JSON response with "height" = "1"

        # Deploy
         When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
            | arg1 |  arg2 | arg3 | arg4 |
            |  a   |  100  |  b   |  200 |
        Then I should have received a chaincode name
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 | vp3 |

        # get things started. All peers up and executing Txs
        When I invoke chaincode "example2" function name "invoke" on "vp0" "5" times
            |arg1|arg2|arg3|
            | a  | b  | 1  |
        Then I should have received a transactionID
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 | vp3 |

        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 | vp3 |
        Then I should get a JSON response from peers with "result.message" = "95"
            | vp0  | vp1 | vp2 | vp3 |

        # STOP vp2
        Given I stop peers:
            | vp2  |

        # continue invoking Txs
        When I invoke chaincode "example2" function name "invoke" on "vp0" "5" times
            |arg1|arg2|arg3|
            | a  | b  | 1 |
        Then I should have received a transactionID
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp3 |

        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp3 |
        Then I should get a JSON response from peers with "result.message" = "90"
            | vp0 | vp1 | vp3 |

    Examples: Consensus Options
        |          ComposeFile                       |   WaitTime   |
        |   docker-compose-4-consensus-batch.yml     |      60      |

@scat
    Scenario Outline: 4 peers and 1 membersrvc, consensus fails if 2 backup replicas fail

        Given we compose "<ComposeFile>"
        And I use the following credentials for querying peers:
            | peer |   username  |    secret    |
            | vp0  |  test_user0 | MS9qrN8hFjlE |
            | vp1  |  test_user1 | jGlNl6ImkuDo |
            | vp2  |  test_user2 | zMflqOKezFiA |
            | vp3  |  test_user3 | vWdLCE00vJy0 |
        And I register with CA supplying username "test_user0" and secret "MS9qrN8hFjlE" on peers:
            | vp0 |

        When requesting "/chain" from "vp0"
        Then I should get a JSON response with "height" = "1"

        # Deploy
        When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
            | arg1 |  arg2 | arg3 | arg4 |
            |  a   |  100  |  b   |  200 |
        Then I should have received a chaincode name
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 | vp3 |

        # get things started. All peers up and executing Txs
        When I invoke chaincode "example2" function name "invoke" on "vp0" "5" times
            |arg1|arg2|arg3|
            | a  | b  | 1  |
        Then I should have received a transactionID
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 | vp3 |

        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 | vp3 |
        Then I should get a JSON response from peers with "result.message" = "95"
            | vp0  | vp1 | vp2 | vp3 |

        Given I stop peers:
            | vp1  | vp2 |

        # continue invoking Txs
        When I invoke chaincode "example2" function name "invoke" on "vp0" "5" times
            |arg1|arg2|arg3|
            | a  | b  | 1 |
        And I wait "5" seconds

        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0 | vp3 |
        Then I should get a JSON response from peers with "result.message" = "95"
            | vp0 | vp3 |

    Examples: Consensus Options
        |          ComposeFile                       |   WaitTime   |
        |   docker-compose-4-consensus-batch.yml     |      60      |

     #@doNotDecompose
     #@wip
     #@skip
      Scenario Outline: 4 peers and 1 membersrvc, consensus still works if 1 peer (vp3) is byzantine

         Given we compose "<ComposeFile>"
         And I use the following credentials for querying peers:
            | peer |   username  |    secret    |
            | vp0  |  test_user0 | MS9qrN8hFjlE |
            | vp1  |  test_user1 | jGlNl6ImkuDo |
            | vp2  |  test_user2 | zMflqOKezFiA |
            | vp3  |  test_user3 | vWdLCE00vJy0 |
         And I register with CA supplying username "test_user0" and secret "MS9qrN8hFjlE" on peers:
            | vp0 |

         When requesting "/chain" from "vp0"
          Then I should get a JSON response with "height" = "1"

         # Deploy
         When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
            | arg1 |  arg2 | arg3 | arg4 |
            |  a   |  100  |  b   |  200 |
          Then I should have received a chaincode name
          Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 |

         When I invoke chaincode "example2" function name "invoke" on "vp0" "50" times
              |arg1|arg2|arg3|
              | a  | b  | 1  |
          Then I should have received a transactionID
          Then I wait up to "60" seconds for transaction to be committed to peers:
              | vp1 | vp2 |

         When I query chaincode "example2" function name "query" with value "a" on peers:
              | vp0  | vp1 | vp2 |
          Then I should get a JSON response from peers with "result.message" = "50"
              | vp0  | vp1 | vp2 |

      Examples: Consensus Options
          |                                  ComposeFile                                               |   WaitTime   |
          |   docker-compose-4-consensus-batch.yml    docker-compose-4-consensus-vp3-byzantine.yml     |      60      |


  #@doNotDecompose
  @issue_1182
  Scenario Outline: chaincode example02 with 4 peers,1 membersrvc, and 1 non-validating peer.

      Given we compose "<ComposeFile>"
      And I register with CA supplying username "binhn" and secret "7avZQLwcUe9q" on peers:
         | nvp0  |
      And I use the following credentials for querying peers:
         | peer |   username  |    secret    |
         | vp0  |  test_user0 | MS9qrN8hFjlE |
         | vp1  |  test_user1 | jGlNl6ImkuDo |
         | vp2  |  test_user2 | zMflqOKezFiA |
         | vp3  |  test_user3 | vWdLCE00vJy0 |

#      Current issue as blocks NOT synced yet.
#      When requesting "/chain" from "nvp0"
#      Then I should get a JSON response with "height" = "1"


      # Deploy
      When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "nvp0"
                    | arg1 |  arg2 | arg3 | arg4 |
                    |  a   |  100  |  b   |  200 |
      Then I should have received a chaincode name
      Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
                 | vp1 | vp2 |

    Examples: Consensus Options
        |          ComposeFile                                                       |   WaitTime   |
        |   docker-compose-4-consensus-batch.yml docker-compose-4-consensus-nvp0.yml |      60      |

@scat
    @issue_1000
    Scenario Outline: chaincode example02 with 4 peers and 1 membersrvc, test crash fault

        Given we compose "<ComposeFile>"
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
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 | vp3 |

        # Build up a sizable blockchain, to advance the sequence number
        When I invoke chaincode "example2" function name "invoke" on "vp0" "30" times
            |arg1|arg2|arg3|
            | b  | a  | 1  |
        Then I should have received a transactionID
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 | vp3 |

        When I query chaincode "example2" function name "query" with value "a" on peers:
|
        Then I should get a JSON response from peers with "result.message" = "130"
            | vp0  | vp1 | vp2 | vp3 |

        Given I stop peers:
            | vp1 | vp2 | vp3 |

        # Now start vp1, vp2 again, hopefully retaining pbft state
        Given I start peers, waiting up to "15" seconds for them to be ready:
            | vp1 | vp2 |

        # Invoke 1 more tx, if the crash recovery worked, it will commit, otherwise, it will not
        When I invoke chaincode "example2" function name "invoke" on "vp0"
            |arg1|arg2|arg3|
            | a  | b  | 10 |
        Then I should have received a transactionID
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 |
        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 |
	    Then I should get a JSON response from peers with "result.message" = "120"
            | vp0  | vp1 | vp2 |


    Examples: Consensus Options
        |          ComposeFile                       |   WaitTime   |
        |   docker-compose-4-consensus-batch.yml     |      60      |



@scat
    @issue_1091
    @doNotDecompose
    Scenario Outline: chaincode example02 with 4 peers and 1 membersrvc, issue #1091 (out of date peer)

        Given we compose "<ComposeFile>"
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
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 |

        Given I stop peers:
            | vp3  |

        # Execute one request to get vp3 out of sync
        When I invoke chaincode "example2" function name "invoke" on "vp0"
            |arg1|arg2|arg3|
            | b  | a  | 1  |
        Then I should have received a transactionID
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 |

        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 |
        Then I should get a JSON response from peers with "result.message" = "101"
            | vp0  | vp1 | vp2 |

        Given I start peers, waiting up to "15" seconds for them to be ready:
            | vp3  |

        # Invoke some more txs, this will trigger a state transfer, but it cannot complete
        When I invoke chaincode "example2" function name "invoke" on "vp0" "8" times
            |arg1|arg2|arg3|
            | a  | b  | 10 |
        Then I should have received a transactionID
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 |
        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 |
        Then I should get a JSON response from peers with "result.message" = "21"

        # Force VP3 to attempt to sync with the rest of the peers
        When I invoke chaincode "example2" function name "invoke" on "vp3"
            |arg1|arg2|arg3|
            | a  | b  | 10 |
        And I unconditionally query chaincode "example2" function name "query" with value "a" on peers:
            | vp3  |
        Then I should get a JSON response from peers with "error.data" = "Error when querying chaincode: Error: state may be inconsistent, cannot query"
            | vp3  |


    Examples: Consensus Options
        |          ComposeFile                       |   WaitTime   |
        |   docker-compose-4-consensus-batch.yml     |      60      |

    Scenario: chaincode example02 with 4 peers, one paused, issue #1056
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

        Given I pause peers:
            | vp3  |

        When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
            | arg1 |  arg2 | arg3 | arg4 |
            |  a   |  100  |  b   |  200 |
        Then I should have received a chaincode name
        Then I wait up to "60" seconds for transaction to be committed to peers:
            | vp1 | vp2 |

        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 |
        Then I should get a JSON response from peers with "result.message" = "100"
            | vp0  | vp1 | vp2 |

        When I invoke chaincode "example2" function name "invoke" on "vp0" "20" times
            |arg1|arg2|arg3|
            | a  | b  |  1 |
        Then I should have received a transactionID
        Then I wait up to "20" seconds for transaction to be committed to peers:
            | vp1 | vp2 |

        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp0  | vp1 | vp2 |
        Then I should get a JSON response from peers with "result.message" = "80"
            | vp0  | vp1 | vp2 |

    @issue_1873
    Scenario Outline: 4 peers and 1 membersrvc, consensus works if vp0 is stopped TTT3
        Given we compose "<ComposeFile>"
        And I use the following credentials for querying peers:
            | peer |   username  |    secret    |
            | vp0  |  test_user0 | MS9qrN8hFjlE |
            | vp1  |  test_user1 | jGlNl6ImkuDo |
            | vp2  |  test_user2 | zMflqOKezFiA |
            | vp3  |  test_user3 | vWdLCE00vJy0 |
        And I register with CA supplying username "test_user0" and secret "MS9qrN8hFjlE" on peers:
            | vp0 |

        When requesting "/chain" from "vp0"
        Then I should get a JSON response with "height" = "1"

        # Deploy
        When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
            | arg1 |  arg2 | arg3 | arg4 |
            |  a   |  100  |  b   |  200 |
        Then I should have received a chaincode name
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp1 | vp2 | vp3 |

        When requesting "/chain" from "vp0"
        Then I should get a JSON response with "height" = "2"

        Given I stop peers:
            | vp0  |

        And I register with CA supplying username "test_user1" and secret "jGlNl6ImkuDo" on peers:
            | vp1 |

        When I invoke chaincode "example2" function name "invoke" on "vp1" "5" times
            |arg1|arg2|arg3|
            | a  | b  | 1 |
        Then I should have received a transactionID
        Then I wait up to "<WaitTime>" seconds for transaction to be committed to peers:
            | vp2 | vp3 |
        When I query chaincode "example2" function name "query" with value "a" on peers:
            | vp1 | vp2 | vp3 |
        Then I should get a JSON response from peers with "result.message" = "95"
            | vp1 | vp2 | vp3 |
    Examples: Consensus Options
        |          ComposeFile                       |   WaitTime   |
        |   docker-compose-4-consensus-batch.yml     |      60      |

    @issue_1851
    Scenario Outline: verify reconnect of disconnected peer, issue #1851
        Given we compose "<ComposeFile>"

        When requesting "/network/peers" from "vp0"
        Then I should get a JSON response with array "peers" contains "2" elements

        Given I stop peers:
            | vp0  |

        When requesting "/network/peers" from "vp1"
        Then I should get a JSON response with array "peers" contains "1" elements

        Given I start peers, waiting up to "15" seconds for them to be ready:
            | vp0  |

        When requesting "/network/peers" from "vp1"
        Then I should get a JSON response with array "peers" contains "2" elements

    Examples: Composition options
        |          ComposeFile     |
        |   docker-compose-2.yml   |


@scat
@issue_1942
# @doNotDecompose
Scenario: chaincode example02 with 4 peers, stop and start alternates, reverse
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

    When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
                          | arg1 |  arg2 | arg3 | arg4 |
                          |  a   |  1000 |  b   |   0  |
    Then I should have received a chaincode name
    Then I wait up to "60" seconds for transaction to be committed to peers:
                          | vp1 | vp2 | vp3 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
                          | vp0 | vp1  | vp2 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "1000"
                          | vp0 |  vp1 | vp2 | vp3 |

    Given I stop peers:
                          | vp2 |
    And I register with CA supplying username "test_user3" and secret "vWdLCE00vJy0" on peers:
                          | vp3  |

    When I invoke chaincode "example2" function name "invoke" on "vp3" "3" times
                          |arg1|arg2|arg3|
                          | a  | b  | 1  |
    Then I should have received a transactionID
    Then I wait up to "180" seconds for transaction to be committed to peers:
                          | vp1 | vp3 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
                          | vp0  | vp1 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "997"
                          | vp0  | vp1 | vp3 |

    Given I start peers, waiting up to "15" seconds for them to be ready:
                          | vp2  |

    Given I stop peers:
                          | vp1  |
    When I invoke chaincode "example2" function name "invoke" on "vp3" "20" times
                          |arg1|arg2|arg3|
                          | a  | b  | 1  |
    Then I wait up to "180" seconds for transactions to be committed to peers:
                          | vp2 | vp3 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
                          | vp0  | vp2 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "977"
                          | vp0  | vp2 | vp3 |

@scat
@issue_1874a
#@doNotDecompose
Scenario: chaincode example02 with 4 peers, two stopped
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

    When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
        | arg1 |  arg2 | arg3 | arg4 |
        |  a   |  100  |  b   |  200 |
    Then I should have received a chaincode name
    Then I wait up to "60" seconds for transaction to be committed to peers:
        | vp1 | vp2 | vp3 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
        | vp0 | vp1 | vp2 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "100"
        | vp0 | vp1 | vp2 | vp3 |

    Given I stop peers:
        | vp2 | vp3 |

    When I invoke chaincode "example2" function name "invoke" on "vp0"
        |arg1|arg2|arg3|
        | a  | b  | 10 |
    Then I should have received a transactionID

    Given I start peers, waiting up to "15" seconds for them to be ready:
        | vp3 |

    # Make sure vp3 catches up first
    Then I wait up to "60" seconds for transaction to be committed to peers:
        #| vp0 | vp1 | vp3 |
        | vp1 | vp3 |
    When I query chaincode "example2" function name "query" with value "a" on peers:
        | vp0 | vp1 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "90"
        | vp0 | vp1 | vp3 |

    When I invoke chaincode "example2" function name "invoke" on "vp0" "9" times
        |arg1|arg2|arg3|
        | a  | b  | 10 |
    Then I should have received a transactionID
    Then I wait up to "60" seconds for transaction to be committed to peers:
        #| vp0 | vp1 | vp3 |
        | vp1 | vp3 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
        | vp0 | vp1 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "0"
        | vp0 | vp1 | vp3 |

@scat
@issue_1874b
#@doNotDecompose
Scenario: chaincode example02 with 4 peers, two stopped, bring back vp0
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

    When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
        | arg1 |  arg2 | arg3 | arg4 |
        |  a   |  100  |  b   |  200 |
    Then I should have received a chaincode name
    Then I wait up to "60" seconds for transaction to be committed to peers:
        | vp1 | vp2 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
        | vp0  | vp1 | vp2 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "100"
        | vp0  | vp1 | vp2 | vp3 |

    Given I stop peers:
        | vp0 |

    And I register with CA supplying username "test_user1" and secret "jGlNl6ImkuDo" on peers:
        | vp1  |

    When I invoke chaincode "example2" function name "invoke" on "vp1"
        |arg1|arg2|arg3|
        | a  | b  | 10 |
    Then I should have received a transactionID

    Given I stop peers:
        | vp3  |

    When I invoke chaincode "example2" function name "invoke" on "vp1"
        |arg1|arg2|arg3|
        | a  | b  | 10 |
    Then I should have received a transactionID

    Given I start peers, waiting up to "15" seconds for them to be ready:
        | vp0  |

    When I invoke chaincode "example2" function name "invoke" on "vp1" "8" times
        |arg1|arg2|arg3|
        | a  | b  | 10 |
    Then I should have received a transactionID
    Then I wait up to "60" seconds for transaction to be committed to peers:
        | vp1 | vp2 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
        | vp0  | vp1 | vp2 |
    Then I should get a JSON response from peers with "result.message" = "0"
        | vp0  | vp1 | vp2 |

@scat
@issue_1874c
Scenario: chaincode example02 with 4 peers, two stopped, bring back both
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

    When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ctor "init" to "vp0"
        | arg1 |  arg2 | arg3 | arg4 |
        |  a   |  100  |  b   |  200 |
    Then I should have received a chaincode name
    Then I wait up to "60" seconds for transaction to be committed to peers:
        | vp1 | vp2 |

    When I query chaincode "example2" function name "query" with value "a" on peers:
        | vp0  | vp1 | vp2 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "100"
        | vp0  | vp1 | vp2 | vp3 |

    Given I stop peers:
        | vp1 | vp2 |

    When I invoke chaincode "example2" function name "invoke" on "vp0" "1" times
        |arg1|arg2|arg3|
        | a  | b  | 10 |
    Then I should have received a transactionID

    Given I start peers, waiting up to "15" seconds for them to be ready:
        | vp1 | vp2 |

    When I invoke chaincode "example2" function name "invoke" on "vp0" "8" times
        |arg1|arg2|arg3|
        | a  | b  | 10 |
    Then I should have received a transactionID
    Then I wait up to "60" seconds for transaction to be committed to peers:
        | vp1 | vp2 | vp3 |
    Then I wait "30" seconds

    When I query chaincode "example2" function name "query" with value "a" on peers:
        | vp0  | vp1 | vp2 | vp3 |
    Then I should get a JSON response from peers with "result.message" = "10"
        | vp0  | vp1 | vp2 | vp3 |

@scat
    @issue_2116
    #@doNotDecompose
    Scenario Outline: chaincode authorizable_counter with 4 peers, two stopped, bring back both
        Given we compose "<ComposeFile>"
        And I register with CA supplying username "diego" and secret "DRJ23pEQl16a" on peers:
                | vp0  |
        And I use the following credentials for querying peers:
                | peer |   username  |    secret    |
                | vp0  |  test_user0 | MS9qrN8hFjlE |
                | vp1  |  test_user1 | jGlNl6ImkuDo |
                | vp2  |  test_user2 | zMflqOKezFiA |
                | vp3  |  test_user3 | vWdLCE00vJy0 |

        When requesting "/chain" from "vp0"
        Then I should get a JSON response with "height" = "1"

        When I deploy chaincode "github.com/hyperledger/fabric/examples/chaincode/go/authorizable_counter" with ctor "init" to "vp0"
                | arg1 |
                | 0    |
        Then I should have received a chaincode name
        Then I wait up to "60" seconds for transaction to be committed to peers:
                | vp1 | vp2 |

        When I query chaincode "authorizable_counter" function name "read" on "vp0":
                |arg1|
                | a  |
        Then I should get a JSON response with "result.message" = "0"

        When I invoke chaincode "authorizable_counter" function name "increment" with attributes "position" on "vp0"
                |arg1|
                | a  |
        Then I should have received a transactionID

        When I invoke chaincode "authorizable_counter" function name "increment" on "vp0" "8" times
                |arg1|arg2|arg3|
                | a  | b  | 10 |
        Then I should have received a transactionID
        Then I wait up to "30" seconds for transaction to be committed to peers:
                | vp1 | vp2 | vp3 |

        When I query chaincode "authorizable_counter" function name "read" with value "a" on peers:
                | vp0  | vp1 | vp2 | vp3 |

        Then I should get a JSON response from peers with "result.message" = "1"
                | vp0  | vp1 | vp2 | vp3 |

        When I invoke chaincode "authorizable_counter" function name "increment" with attributes "company" on "vp0"
                |arg1|
                | a  |

        When I invoke chaincode "authorizable_counter" function name "increment" with attributes "company, position, age" on "vp0"
                |arg1|
                | a  |

        Then I wait up to "15" seconds for transaction to be committed to peers:
                | vp1 | vp2 | vp3 |

        When I query chaincode "authorizable_counter" function name "read" with value "a" on peers:
                | vp0  | vp1 | vp2 | vp3 |

        Then I should get a JSON response from peers with "result.message" = "2"
                | vp0  | vp1 | vp2 | vp3 |

        Examples: Consensus Options
        |          ComposeFile                   |   WaitTime    |
        |   docker-compose-4-consensus-batch.yml docker-membersrvc-attributes-enabled.yml |      120      |
        |   docker-compose-4-consensus-batch.yml docker-membersrvc-attributes-enabled.yml docker-membersrvc-attributes-encryption-enabled.yml |      120      |

#    noop
#    @doNotDecompose
  Scenario: noop chaincode test
    Given we compose "docker-compose-1.yml"
      When I invoke master chaincode "noop" function name "execute" on "vp0"
        |arg1|
        | aa |
      Then I should have received a transactionID
