#
# Test the Hyperledger SDK
#
# Tags that can be used and will affect test internals:
#
#  @doNotDecompose will NOT decompose the named compose_yaml after scenario ends.  Useful for setting up environment and reviewing after scenario.
#

Feature: Node SDK
    As a HyperLedger developer
    I want to have a single test driver for the various Hyperledger SDKs


  @doNotDecompose
  @sdk
  Scenario Outline: test initial sdk setup

      Given we compose "<ComposeFile>"
      And I wait "5" seconds
      And I register thru the sample SDK app supplying username "WebAppAdmin" and secret "DJY27pEnl16d" on "sampleApp0"
      Then I should get a JSON response with "foo" = "bar"


    Examples: Consensus Options
        |          ComposeFile                                               |   WaitTime   |
        |   docker-compose-4-consensus-batch.yml docker-compose-sdk-node.yml |      60      |
        #|   docker-compose-4-consensus-batch.yml docker-compose-sdk-java.yml |      60      |
