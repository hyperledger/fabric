#
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

def getAttributeFromJSON(attribute, json):
    foundJson = getHierarchyAttributesFromJSON(attribute.split("."), json)
    assert foundJson is not None, "Unable to locate {} in JSON".format(attribute)

    return foundJson

def getHierarchyAttributesFromJSON(attributes, json):
    foundJson = None

    currentAttribute = attributes[0]
    if currentAttribute in json:
        foundJson = json[currentAttribute]

        attributesToGo = attributes[1:]
        if len(attributesToGo) > 0:
            foundJson = getHierarchyAttributesFromJSON(attributesToGo, foundJson)

    return foundJson