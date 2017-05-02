# Copyright IBM Corp. 2016 All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

Feature: Orderer Service
    As a user I want to validate an orderer service runs as expected

#@doNotDecompose
Scenario Outline: Message Broadcast
  Given a bootstrapped orderer network of type <type>
  When a message is broadcasted
  Then we get a successful broadcast response
  Examples:
    | type  |
    | solo  |
    | kafka |

Scenario Outline: Broadcasted message delivered.
  Given a bootstrapped orderer network of type <type>
  When 1 unique messages are broadcasted
  Then all 1 messages are delivered within 10 seconds
Examples:
  | type  |
  | solo  |
  | kafka |
