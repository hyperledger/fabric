Feature: Instantiate chaincode via CLI

Scenario Outline: Install & instantiate a first version chaincode via CLI

  Given a fabric peer and orderer
  When a <lang> chaincode is installed via the CLI
  Then the chaincode can be instantiated via the CLI

  Examples:
  | lang |
  | go   |
  | java |
