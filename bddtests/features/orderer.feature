#
# Test Orderer
#
# Tags that can be used and will affect test internals:
#  @doNotDecompose will NOT decompose the named compose_yaml after scenario ends.  Useful for setting up environment and reviewing after scenario.
#  @chaincodeImagesUpToDate use this if all scenarios chaincode images are up to date, and do NOT require building.  BE SURE!!!

#@chaincodeImagesUpToDate
@orderer
Feature: Orderer
    As a Fabric developer
    I want to run and validate a orderer service




#    @doNotDecompose
	Scenario Outline: Basic orderer function

	    Given we compose "<ComposeFile>"
	    And I wait ".5" seconds
	    And user "binhn" is an authorized user of the ordering service
	    When user "binhn" broadcasts "<NumMsgsToBroadcast>" unique messages on "orderer0"
	    And user "binhn" connects to deliver function on "orderer0" with Ack of "<SendAck>" and properties:
            |  Start    | SpecifiedNumber |  WindowSize    |
            | SPECIFIED |        1         |       10        |
	    Then user "binhn" should get a delivery from "orderer0" of "<ExpectedBlocks>" blocks with "<NumMsgsToBroadcast>" messages within "<BatchTimeout>" seconds

	Examples: Orderer Options
        |          ComposeFile                |    SendAck   |    NumMsgsToBroadcast  |  ExpectedBlocks    |  BatchTimeout  |
        |   docker-compose-orderer-solo.yml   |     true     |        20              |         2          |       10       |
        |   docker-compose-orderer-solo.yml   |     true     |        40              |         4          |       10       |
        |   docker-compose-orderer-solo.yml   |     true     |        60              |         6          |       10       |



#    @doNotDecompose
	Scenario Outline: Basic seek orderer function (Utilizing properties for atomic broadcast)

	    Given we compose "<ComposeFile>"
	    And I wait ".5" seconds
	    And user "binhn" is an authorized user of the ordering service
	    When user "binhn" broadcasts "<NumMsgsToBroadcast>" unique messages on "orderer0"
	    And user "binhn" connects to deliver function on "orderer0" with Ack of "<SendAck>" and properties:
            |  Start    | SpecifiedNumber |  WindowSize    |
            | SPECIFIED |        1         |       10        |
	    Then user "binhn" should get a delivery from "orderer0" of "<ExpectedBlocks>" blocks with "<NumMsgsToBroadcast>" messages within "<BatchTimeout>" seconds
	    When user "binhn" seeks to block "1" on deliver function on "orderer0"
	    Then user "binhn" should get a delivery from "orderer0" of "<ExpectedBlocks>" blocks with "<NumMsgsToBroadcast>" messages within "1" seconds


    Examples: Orderer Options
        |          ComposeFile                |    SendAck   |    NumMsgsToBroadcast  |  ExpectedBlocks    |  BatchTimeout  |
        |   docker-compose-orderer-solo.yml   |     true     |        20              |         2          |       10       |
#        |   docker-compose-orderer-solo.yml   |     true     |        40              |         4          |       10       |
#        |   docker-compose-orderer-solo.yml   |     true     |        60              |         6          |       10       |


#    @doNotDecompose
	Scenario Outline: Basic orderer function varying ACK

	    Given we compose "<ComposeFile>"
	    And I wait ".5" seconds
	    And user "binhn" is an authorized user of the ordering service
	    When user "binhn" broadcasts "<NumMsgsToBroadcast>" unique messages on "orderer0"
	    And user "binhn" connects to deliver function on "orderer0" with Ack of "<SendAck>" and properties:
            |  Start    | SpecifiedNumber |  WindowSize    |
            | SPECIFIED |        1         |       1         |
	    Then user "binhn" should get a delivery from "orderer0" of "<ExpectedBlocks>" blocks with "<NumMsgsToBroadcast>" messages within "<BatchTimeout>" seconds


    Examples: Orderer Options
        |          ComposeFile                |    SendAck   |    NumMsgsToBroadcast  |  ExpectedBlocks    |  BatchTimeout  |
        |   docker-compose-orderer-solo.yml   |     false    |        20              |         1          |       10       |
        |   docker-compose-orderer-solo.yml   |     true     |        20              |         2          |       10       |
