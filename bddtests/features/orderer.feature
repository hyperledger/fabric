
#
# Copyright IBM Corp. 2016 All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

# Tags that can be used and will affect test internals:
#  @doNotDecompose will NOT decompose the named compose_yaml after scenario ends.
#  Useful for setting up environment and reviewing after scenario.

@orderer
Feature: Orderer
    As a Fabric developer
    I want to run and validate a orderer service


#    @doNotDecompose
    Scenario Outline: Basic orderer function

        Given we compose "<ComposeFile>"
        And I wait "<BootTime>" seconds
        And user "binhn" is an authorized user of the ordering service
        When user "binhn" broadcasts "<NumMsgsToBroadcast>" unique messages on "orderer0"
        And user "binhn" waits "<WaitTime>" seconds
        And user "binhn" connects to deliver function on "orderer0"
        And user "binhn" sends deliver a seek request on "orderer0" with properties:
            | Start |  End    |
            |   1   |  Newest |
        Then user "binhn" should get a delivery from "orderer0" of "<ExpectedBlocks>" blocks with "<NumMsgsToBroadcast>" messages within "<BatchTimeout>" seconds

    Examples: Solo Orderer
        |          ComposeFile                 |   NumMsgsToBroadcast  |  ExpectedBlocks    |  BatchTimeout  |  BootTime  |  WaitTime  |
        |   docker-compose-orderer-solo.yml    |       20              |         2          |       10       |     .5     |     .5     |
        |   docker-compose-orderer-solo.yml    |       40              |         4          |       10       |     .5     |     .5     |
        |   docker-compose-orderer-solo.yml    |       60              |         6          |       10       |     .5     |     .5     |

    Examples: 1 Kafka Orderer and 1 Kafka Broker
        |          ComposeFile                 |   NumMsgsToBroadcast  |  ExpectedBlocks    |  BatchTimeout  |  BootTime  |  WaitTime  |
        |   environments/orderer-1-kafka-1     |       20              |         2          |       10       |      5     |      1     |
        |   environments/orderer-1-kafka-1     |       40              |         4          |       10       |      5     |      1     |
        |   environments/orderer-1-kafka-1     |       60              |         6          |       10       |      5     |      1     |

    Examples: 1 Kafka Orderer and 3 Kafka Brokers
        |          ComposeFile                 |   NumMsgsToBroadcast  |  ExpectedBlocks    |  BatchTimeout  |  BootTime  |  WaitTime  |
        |   environments/orderer-1-kafka-3     |       20              |         2          |       10       |      5     |      1     |
        |   environments/orderer-1-kafka-3     |       40              |         4          |       10       |      5     |      1     |
        |   environments/orderer-1-kafka-3     |       60              |         6          |       10       |      5     |      1     |

#    @doNotDecompose
    Scenario Outline: Basic seek orderer function (Utilizing properties for atomic broadcast)

        Given we compose "<ComposeFile>"
        And I wait "<BootTime>" seconds
        And user "binhn" is an authorized user of the ordering service
        When user "binhn" broadcasts "<NumMsgsToBroadcast>" unique messages on "orderer0"
        And user "binhn" waits "<WaitTime>" seconds
        And user "binhn" connects to deliver function on "orderer0"
        And user "binhn" sends deliver a seek request on "orderer0" with properties:
            | Start |  End    |
            |   1   |  Newest |
        Then user "binhn" should get a delivery from "orderer0" of "<ExpectedBlocks>" blocks with "<NumMsgsToBroadcast>" messages within "<BatchTimeout>" seconds
        When user "binhn" sends deliver a seek request on "orderer0" with properties:
            | Start |  End    |
            |   1   |  Newest |
        Then user "binhn" should get a delivery from "orderer0" of "<ExpectedBlocks>" blocks with "<NumMsgsToBroadcast>" messages within "1" seconds

    Examples: Solo Orderer
        |          ComposeFile                 |   NumMsgsToBroadcast  |  ExpectedBlocks    |  BatchTimeout  |  BootTime  |  WaitTime  |
        |   docker-compose-orderer-solo.yml    |       20              |         2          |       10       |     .5     |     .5     |
        |   docker-compose-orderer-solo.yml    |       40              |         4          |       10       |     .5     |     .5     |
        |   docker-compose-orderer-solo.yml    |       60              |         6          |       10       |     .5     |     .5     |

    Examples: 1 Kafka Orderer and 1 Kafka Broker
        |          ComposeFile                 |   NumMsgsToBroadcast  |  ExpectedBlocks    |  BatchTimeout  |  BootTime  |  WaitTime  |
        |   environments/orderer-1-kafka-1     |       20              |         2          |       10       |      5     |      1     |
        |   environments/orderer-1-kafka-1     |       40              |         4          |       10       |      5     |      1     |
        |   environments/orderer-1-kafka-1     |       60              |         6          |       10       |      5     |      1     |

    Examples: 1 Kafka Orderer and 3 Kafka Brokers
        |          ComposeFile                 |   NumMsgsToBroadcast  |  ExpectedBlocks    |  BatchTimeout  |  BootTime  |  WaitTime  |
        |   environments/orderer-1-kafka-3     |       20              |         2          |       10       |      5     |      1     |
        |   environments/orderer-1-kafka-3     |       40              |         4          |       10       |      5     |      1     |
        |   environments/orderer-1-kafka-3     |       60              |         6          |       10       |      5     |      1     |


#    @doNotDecompose
    Scenario Outline: Basic orderer function using oldest seek target

        Given we compose "<ComposeFile>"
        And I wait "<BootTime>" seconds
        And user "binhn" is an authorized user of the ordering service
        When user "binhn" broadcasts "<NumMsgsToBroadcast>" unique messages on "orderer0"
        And user "binhn" waits "<WaitTime>" seconds
        And user "binhn" connects to deliver function on "orderer0"
        And user "binhn" sends deliver a seek request on "orderer0" with properties:
            | Start  |  End    |
            | Oldest |  2      |
        Then user "binhn" should get a delivery from "orderer0" of "<ExpectedBlocks>" blocks with "<NumMsgsToBroadcast>" messages within "<BatchTimeout>" seconds

    Examples: Solo Orderer
        |          ComposeFile                 |   NumMsgsToBroadcast  |  ExpectedBlocks    |  BatchTimeout  |  BootTime  |  WaitTime  |
        |   docker-compose-orderer-solo.yml    |       20              |         3          |       10       |     .5     |     .5     |

    Examples: 1 Kafka Orderer and 1 Kafka Broker
        |          ComposeFile                 |   NumMsgsToBroadcast  |  ExpectedBlocks    |  BatchTimeout  |  BootTime  |  WaitTime  |
        |   environments/orderer-1-kafka-1     |       20              |         3          |       10       |      5     |      1     |

    Examples: 1 Kafka Orderer and 3 Kafka Brokers
        |          ComposeFile                 |   NumMsgsToBroadcast  |  ExpectedBlocks    |  BatchTimeout  |  BootTime  |  WaitTime  |
        |   environments/orderer-1-kafka-3     |       20              |         3          |       10       |      5     |      1     |
