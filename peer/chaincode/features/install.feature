Feature: Install chaincode via CLI

Scenario Outline: Install first version chaincode via CLI

  Given a fabric peer and orderer
  When a <lang> chaincode is installed via the CLI
  Then the chaincode is installed on the peer

  Examples:
  | lang |
  | go   |
  | java |

Scenario Outline: Install chaincode with new version

  Only the first version of a chaincode (identified by chaincode name) can be
  installed. Subsequent versions must be upgraded instead.

  Given a fabric peer and orderer
  When version one of a <lang> chaincode is installed via the CLI
  Then installing version two of the same chaincode via the CLI will fail

  Examples:
  | lang |
  | go   |
  | java |
