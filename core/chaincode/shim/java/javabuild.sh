#!/bin/bash

#
#Copyright DTCC 2016 All Rights Reserved.
#
#Licensed under the Apache License, Version 2.0 (the "License");
#you may not use this file except in compliance with the License.
#You may obtain a copy of the License at
#
#         http://www.apache.org/licenses/LICENSE-2.0
#
#Unless required by applicable law or agreed to in writing, software
#distributed under the License is distributed on an "AS IS" BASIS,
#WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#See the License for the specific language governing permissions and
#limitations under the License.
#
#
set -e
PARENTDIR=$(pwd)
ARCH=`uname -m`

if [ x$ARCH != xx86_64 ]
then
    apt-get update && apt-get install openjdk-8-jdk -y
    echo "FIXME: Java Shim code needs work on ppc64le. Commenting it for now."
else
    add-apt-repository ppa:openjdk-r/ppa -y
    apt-get update && apt-get install openjdk-8-jdk -y
    update-java-alternatives -s java-1.8.0-openjdk-amd64
    gradle -q -b ${PARENTDIR}/core/chaincode/shim/java/build.gradle clean
    gradle -q -b ${PARENTDIR}/core/chaincode/shim/java/build.gradle build
    cp -r ${PARENTDIR}/core/chaincode/shim/java/build/libs /root/
fi
