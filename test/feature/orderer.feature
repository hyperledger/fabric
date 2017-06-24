# Copyright IBM Corp. 2017 All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

Feature: Orderer Service
    As a user I want to be able to have my transactions ordered correctly

@skip
Scenario: FAB-1335: Resilient Kafka Orderer and Brokers
    Given the kafka default replication factor is 3
    And the orderer Batchsize MaxMessageCount is 20
    And the orderer BatchTimeout is 10 minutes
    And a bootstrapped orderer network of type kafka with 3 brokers
    When 10 unique messages are broadcasted
    Then we get 10 successful broadcast responses
    When the topic partition leader is stopped
    And 10 unique messages are broadcasted
    Then we get 10 successful broadcast responses
    And all 20 messages are delivered in 1 block

@skip
Scenario: FAB-1306: Adding a new Kafka Broker
    Given a kafka cluster
    And an orderer connected to the kafka cluster
    When a new organization NewOrg certificate is added
    Then the NewOrg is able to connect to the kafka cluster

@skip
Scenario: FAB-1306: Multiple organizations in a kafka cluster, remove 1
    Given a certificate from Org1 is added to the kafka orderer network
    And a certificate from Org2 is added to the kafka orderer network
    And an orderer connected to the kafka cluster
    When authorization for Org2 is removed from the kafka cluster
    Then the Org2 cannot connect to the kafka cluster

@skip
Scenario: FAB-1306: Multiple organizations in a cluster - remove all, reinstate 1.
    Given a certificate from Org1 is added to the kafka orderer network
    And a certificate from Org2 is added to the kafka orderer network
    And a certificate from Org3 is added to the kafka orderer network
    And an orderer connected to the kafka cluster
    When authorization for Org2 is removed from the kafka cluster
    Then the Org2 cannot connect to the kafka cluster
    And the orderer functions successfully
    When authorization for Org1 is removed from the kafka cluster
    Then the Org1 cannot connect to the kafka cluster
    And the orderer functions successfully
    When authorization for Org3 is removed from the kafka cluster
    Then the Org3 cannot connect to the kafka cluster
    And the zookeeper notifies the orderer of the disconnect
    And the orderer stops sending messages to the cluster
    When authorization for Org1 is added to the kafka cluster
    And I wait "15" seconds
    Then the Org1 is able to connect to the kafka cluster
    And the orderer functions successfully

@skip
Scenario: FAB-3851: Message Payloads Greater than 1MB
    Given I have a bootstrapped fabric network
    When a user deploys chaincode
    Then the chaincode is deployed

@daily
Scenario: FAB-4686: Test taking down all kafka brokers and bringing back last 3
    Given I have a bootstrapped fabric network of type kafka
    And I wait "60" seconds
    When a user deploys chaincode at path "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02" with ["init","a","1000","b","2000"] with name "mycc"
    And I wait "30" seconds
    Then the chaincode is deployed
    When a user invokes on the chaincode named "mycc" with args ["invoke","a","b","10"]
    And a user queries on the chaincode named "mycc" with args ["query","a"]
    Then a user receives expected response of 990

    Given "kafka0" is taken down
    And I wait "5" seconds
    When a user invokes on the chaincode named "mycc" with args ["invoke","a","b","10"]
    When a user queries on the chaincode with args ["query","a"]
    Then a user receives expected response of 980

    Given "kafka1" is taken down
    And "kafka2" is taken down
    And "kafka3" is taken down
    And I wait "5" seconds
    When a user invokes on the chaincode named "mycc" with args ["invoke","a","b","10"]
    And a user queries on the chaincode named "mycc" with args ["query","a"]
    Then a user receives expected response of 980
    And I wait "5" seconds

    Given "kafka3" comes back up
    And "kafka2" comes back up
    And "kafka1" comes back up
    And I wait "240" seconds
    When a user invokes on the chaincode named "mycc" with args ["invoke","a","b","10"]
    When a user queries on the chaincode named "mycc" with args ["query","a"]
    Then a user receives expected response of 970

@skip
#@doNotDecompose
Scenario Outline: FAB-3937: Message Broadcast
  Given a bootstrapped orderer network of type <type>
  When a message is broadcasted
  Then we get a successful broadcast response
  Examples:
    | type  |
    | solo  |
    | kafka |

@skip
Scenario Outline: FAB-3938: Broadcasted message delivered.
  Given a bootstrapped orderer network of type <type>
  When 1 unique messages are broadcasted
  Then all 1 messages are delivered within 10 seconds
Examples:
  | type  |
  | solo  |
  | kafka |
