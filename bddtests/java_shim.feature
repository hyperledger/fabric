# Test Hyperledger Peers
#
# Tags that can be used and will affect test internals:
#
#  @doNotDecompose will NOT decompose the named compose_yaml after scenario ends.  Useful for setting up environment and reviewing after scenario.
#
#  @chaincodeImagesUpToDate use this if all scenarios chaincode images are up to date, and do NOT require building.  BE SURE!!!

#
#Copyright DTCC 2016 All Rights Reserved.
#
#Licensed under the Apache License, Version 2.0 (the "License");
#you may not use this file except in compliance with the License.
#You may obtain a copy of the License at
#
#         http://www.apache.org/licenses/LICENSE-2.0
#
#Unless required by applicable law or agreed to in writing, software
#distributed under the License is distributed on an "AS IS" BASIS,
#WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#See the License for the specific language governing permissions and
#limitations under the License.
#
#


#@chaincodeImagesUpToDate
Feature: Java chaincode example

  Scenario: java SimpleSample chaincode example single peer
      Given we compose "docker-compose-1.yml"
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "1"
            When I deploy lang chaincode "examples/chaincode/java/SimpleSample" of "JAVA" with ctor "init" to "vp0"
               | arg1 |  arg2 | arg3 | arg4 |
               |  a   |  100  |  b   |  200 |
            Then I should have received a chaincode name
            Then I wait up to "60" seconds for transaction to be committed to all peers

            When requesting "/chain" from "vp0"
            Then I should get a JSON response with "height" = "2"

              When I query chaincode "SimpleSample" function name "query" on "vp0":
                  |arg1|
                  |  a |
            Then I should get a JSON response with "result.message" = "{'Name':'a','Amount':'100'}"

            When I invoke chaincode "SimpleSample" function name "transfer" on "vp0"
            |arg1|arg2|arg3|
            | a  | b  | 10 |
            Then I should have received a transactionID
            Then I wait up to "25" seconds for transaction to be committed to all peers

            When requesting "/chain" from "vp0"
            Then I should get a JSON response with "height" = "3"

              When I query chaincode "SimpleSample" function name "query" on "vp0":
                  |arg1|
                  |  a |
            Then I should get a JSON response with "result.message" = "{'Name':'a','Amount':'90'}"

              When I query chaincode "SimpleSample" function name "query" on "vp0":
                  |arg1|
                  |  b |
            Then I should get a JSON response with "result.message" = "{'Name':'b','Amount':'210'}"

Scenario: java RangeExample chaincode single peer
      Given we compose "docker-compose-1.yml"
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "1"
            When I deploy lang chaincode "examples/chaincode/java/RangeExample" of "JAVA" with ctor "init" to "vp0"
            ||
            ||
            Then I should have received a chaincode name
            Then I wait up to "60" seconds for transaction to be committed to all peers

            When requesting "/chain" from "vp0"
            Then I should get a JSON response with "height" = "2"

            When I invoke chaincode "RangeExample" function name "put" on "vp0"
            |arg1|arg2|
            | a  | alice |
            Then I should have received a transactionID
            Then I wait up to "25" seconds for transaction to be committed to all peers

            When requesting "/chain" from "vp0"
            Then I should get a JSON response with "height" = "3"

            When I invoke chaincode "RangeExample" function name "put" on "vp0"
            |arg1|arg2|
            | b  | bob |
            Then I should have received a transactionID
            Then I wait up to "25" seconds for transaction to be committed to all peers


            When I query chaincode "RangeExample" function name "get" on "vp0":
                  |arg1|
                  |  a |
            Then I should get a JSON response with "result.message" = "alice"

            When I query chaincode "RangeExample" function name "get" on "vp0":
                  |arg1|
                  |  b |
            Then I should get a JSON response with "result.message" = "bob"


            When I query chaincode "RangeExample" function name "keys" on "vp0":
            ||
            ||
            Then I should get a JSON response with "result.message" = "[a, b]"
            When I invoke chaincode "RangeExample" function name "del" on "vp0"
            |arg1|
            | b  |
            Then I should have received a transactionID
            Then I wait up to "25" seconds for transaction to be committed to all peers
            When I query chaincode "RangeExample" function name "keys" on "vp0":
            ||
            ||
            Then I should get a JSON response with "result.message" = "[a]"

  Scenario: Java TableExample chaincode single peer
      Given we compose "docker-compose-1.yml"
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "1"
            When I deploy lang chaincode "examples/chaincode/java/TableExample" of "JAVA" with ctor "init" to "vp0"
                       ||
                       ||
                Then I should have received a chaincode name
                Then I wait up to "240" seconds for transaction to be committed to all peers

            When requesting "/chain" from "vp0"
                Then I should get a JSON response with "height" = "2"
            When I invoke chaincode "TableExample" function name "insert" on "vp0"
                        |arg1|arg2|
                        | 0  | Alice  |
                Then I should have received a transactionID
                Then I wait up to "25" seconds for transaction to be committed to all peers
            When I invoke chaincode "TableExample" function name "insert" on "vp0"
                        |arg1|arg2|
                        | 1  | Bob  |
                Then I should have received a transactionID
                Then I wait up to "25" seconds for transaction to be committed to all peers
            When I invoke chaincode "TableExample" function name "insert" on "vp0"
                        |arg1|arg2|
                        | 2  | Charlie  |
                Then I should have received a transactionID
                Then I wait up to "25" seconds for transaction to be committed to all peers

              When I query chaincode "TableExample" function name "get" on "vp0":
                  |arg1|
                  |  0 |
                Then I should get a JSON response with "result.message" = "Alice"

              When I query chaincode "TableExample" function name "get" on "vp0":
                  |arg1|
                  |  2 |
                Then I should get a JSON response with "result.message" = "Charlie"
              When I invoke chaincode "TableExample" function name "update" on "vp0"
                        |arg1|arg2|
                        | 2  | Chaitra  |
                Then I should have received a transactionID
                Then I wait up to "25" seconds for transaction to be committed to all peers
             When I query chaincode "TableExample" function name "get" on "vp0":
                  |arg1|
                  |  2 |
                Then I should get a JSON response with "result.message" = "Chaitra"
              When I invoke chaincode "TableExample" function name "delete" on "vp0"
                  |arg1|
                  |  2 |
                Then I should have received a transactionID
                Then I wait up to "25" seconds for transaction to be committed to all peers
             When I query chaincode "TableExample" function name "get" on "vp0":
                  |arg1|
                  |  2 |
                Then I should get a JSON response with "result.message" = "No record found !"
  Scenario: Java chaincode example from remote git repository
      Given we compose "docker-compose-1.yml"
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "1"
      # TODO Needs to be replaced with an official test repo in the future.
            When I deploy lang chaincode "http://github.com/xspeedcruiser/javachaincodemvn" of "JAVA" with ctor "init" to "vp0"
               | arg1 |  arg2 | arg3 | arg4 |
               |  a   |  100  |  b   |  200 |
            Then I should have received a chaincode name
            Then I wait up to "300" seconds for transaction to be committed to all peers

            When requesting "/chain" from "vp0"
            Then I should get a JSON response with "height" = "2"

              When I query chaincode "SimpleSample" function name "query" on "vp0":
                  |arg1|
                  |  a |
            Then I should get a JSON response with "result.message" = "{'Name':'a','Amount':'100'}"

            When I invoke chaincode "SimpleSample" function name "transfer" on "vp0"
            |arg1|arg2|arg3|
            | a  | b  | 10 |
            Then I should have received a transactionID
            Then I wait up to "25" seconds for transaction to be committed to all peers

            When requesting "/chain" from "vp0"
            Then I should get a JSON response with "height" = "3"

              When I query chaincode "SimpleSample" function name "query" on "vp0":
                  |arg1|
                  |  a |
            Then I should get a JSON response with "result.message" = "{'Name':'a','Amount':'90'}"

              When I query chaincode "SimpleSample" function name "query" on "vp0":
                  |arg1|
                  |  b |
            Then I should get a JSON response with "result.message" = "{'Name':'b','Amount':'210'}"
