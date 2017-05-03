Feature: Endorser Peer
    As a user I want to run and validate an endorsing peer

Scenario Outline: Test basic chaincode deploy
  Given I have a bootstrapped fabric network of type <type>
  When a user deploys chaincode
  Then the chaincode is deployed
  When a user queries on the chaincode
  Then a user receives expected response
  Examples:
    | type  |
    | solo  |
    | kafka |
