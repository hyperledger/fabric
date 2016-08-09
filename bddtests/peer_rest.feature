#
# Test REST API Features of Peers
#
# Tags that can be used and will affect test internals:
#  @doNotDecompose will NOT decompose the named compose_yaml after scenario ends.  Useful for setting up environment and reviewing after scenario.
#  @chaincodeImagesUpToDate use this if all scenarios chaincode images are up to date, and do NOT require building.  BE SURE!!!

Feature: Peer REST API
    As a Fabric developer
    I want to verify REST API behavior

    Scenario: 1 peer and 1 membersrvc, query transaction certs with query parameter count
      Given we compose "docker-compose-rest.yml"
      And I use the following credentials for querying peers:
        | peer | username   | secret       |
        | vp0  | test_user0 | MS9qrN8hFjlE |
      And I register with CA supplying username "test_user0" and secret "MS9qrN8hFjlE" on peers:
        | vp0 |

      When I request transaction certs with query parameters on "vp0"
        | key   | value |
        | count | 1     |
      Then I should get a JSON response with "1" different transaction certs