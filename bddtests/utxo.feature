#
# Test openchain Peers
#
# Tags that can be used and will affect test internals:
#
#  @doNotDecompose will NOT decompose the named compose_yaml after scenario ends.  Useful for setting up environment and reviewing after scenario.
#
#  @chaincodeImagesUpToDate use this if all scenarios chaincode images are up to date, and do NOT require building.  BE SURE!!!

#@chaincodeImagesUpToDate
Feature: utxo
    As an openchain developer
    I want to be able to launch a 3 peers

  #@doNotDecompose
  @wip
  @issueUtxo
  Scenario: UTXO chaincode test
    Given we compose "docker-compose-1.yml"
      And I wait "1" seconds
      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "1"
      When I deploy chaincode "github.com/openblockchain/obc-peer/examples/chaincode/go/utxo" with ctor "init" to "vp0"
      ||
      ||

      Then I should have received a chaincode name
      Then I wait up to "60" seconds for transaction to be committed to all peers

      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "2"

      When I invoke chaincode "map" function name "execute" on "vp0"
        | arg1 |
        | AQAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAP////9NBP//AB0BBEVUaGUgVGltZXMgMDMvSmFuLzIwMDkgQ2hhbmNlbGxvciBvbiBicmluayBvZiBzZWNvbmQgYmFpbG91dCBmb3IgYmFua3P/////AQDyBSoBAAAAQ0EEZ4r9sP5VSCcZZ/GmcTC3EFzWqCjgOQmmeWLg6h9h3rZJ9rw/TO84xPNVBOUewRLeXDhN97oLjVeKTHAra/EdX6wAAAAA |
      Then I should have received a transactionID
      Then I wait up to "25" seconds for transaction to be committed to all peers

      When requesting "/chain" from "vp0"
      Then I should get a JSON response with "height" = "3"

    #  When I query chaincode "map" function name "get" on "vp0":
    #    | arg1|
    #    | key1 |
    #  Then I should get a JSON response with "OK" = "value1"
