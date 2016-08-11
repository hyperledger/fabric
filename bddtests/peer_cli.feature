#
# Test Command Line Features of a Peer
#

Feature: Peer Command Line Interface
    As a User of the Fabric
    I want the command line interface to work correctly

    Scenario: List Peers when none are up
    Given we compose "docker-compose-1-empty.yml"
    When I execute "peer network list" in container empty
    Then the command should not complete successfully

    Scenario: List Peers when one is up
    Given we compose "docker-compose-1.yml"
    When I execute "peer network list" in container vp0
    Then the command should complete successfully
    And stdout should contain JSON
    And I should get result with "{"Peers":[]}"

    Scenario: List Peers when two are up
    Given we compose "docker-compose-2.yml"
    When I execute "peer network list" in container vp0
    Then the command should complete successfully
    And stdout should contain JSON with "Peers" array of length 1
