Feature: test dump_db_stat utility
  As a user
  I want to invoke dump_db_stat utility

  Scenario: no flag
    When I execute utility with no flag
    Then I should get a process exit code "3"

  Scenario: wrong flag
    When I execute utility with flag "-dbDir1" and path "nonExistingDir"
    Then I should get a process exit code "2"

  Scenario: dbDirNotExists
    When I execute utility with flag "-dbDir" and path "nonExistingDir"
    Then I should get a process exit code "4"

  Scenario: wrongDBDir
    Given I create a dir "testDir"
    When I execute utility with flag "-dbDir" and path "testDir"
    Then I should get a process exit code "5"
    Then I should delete dir "testDir"
