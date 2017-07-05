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

pattern='(https?://)?((([^:\/]+)(:([^\/]*))?@)?([^:\/?]+)(:([0-9]+))?)'

[ -n "$http_proxy" ] && HTTPPROXY=$http_proxy
[ -n "$HTTP_PROXY" ] && HTTPPROXY=$HTTP_PROXY
[ -n "$https_proxy" ] && HTTPSPROXY=$https_proxy
[ -n "$HTTPS_PROXY" ] && HTTPSPROXY=$HTTPS_PROXY

if [ -n "$HTTPPROXY" ]; then
	if [[ "$HTTPPROXY" =~ $pattern ]]; then
		[ -n "${BASH_REMATCH[4]}" ] && JAVA_OPTS="$JAVA_OPTS -Dhttp.proxyUser=${BASH_REMATCH[4]}"
		[ -n "${BASH_REMATCH[6]}" ] && JAVA_OPTS="$JAVA_OPTS -Dhttp.proxyPassword=${BASH_REMATCH[6]}"
		[ -n "${BASH_REMATCH[7]}" ] && JAVA_OPTS="$JAVA_OPTS -Dhttp.proxyHost=${BASH_REMATCH[7]}"
		[ -n "${BASH_REMATCH[9]}" ] && JAVA_OPTS="$JAVA_OPTS -Dhttp.proxyPort=${BASH_REMATCH[9]}"
	fi
fi
if [ -n "$HTTPSPROXY" ]; then
	if [[ "$HTTPSPROXY" =~ $pattern ]]; then
		[ -n "${BASH_REMATCH[4]}" ] && JAVA_OPTS="$JAVA_OPTS -Dhttps.proxyUser=${BASH_REMATCH[4]}"
		[ -n "${BASH_REMATCH[6]}" ] && JAVA_OPTS="$JAVA_OPTS -Dhttps.proxyPassword=${BASH_REMATCH[6]}"
		[ -n "${BASH_REMATCH[7]}" ] && JAVA_OPTS="$JAVA_OPTS -Dhttps.proxyHost=${BASH_REMATCH[7]}"
		[ -n "${BASH_REMATCH[9]}" ] && JAVA_OPTS="$JAVA_OPTS -Dhttps.proxyPort=${BASH_REMATCH[9]}"
	fi
fi

export JAVA_OPTS

if [ x$ARCH == xx86_64 ]
then
    gradle -q -b ${PARENTDIR}/core/chaincode/shim/java/build.gradle clean
    gradle -q -b ${PARENTDIR}/core/chaincode/shim/java/build.gradle build
    cp -r ${PARENTDIR}/core/chaincode/shim/java/build/libs /root/
else
    echo "FIXME: Java Shim code needs work on ppc64le and s390x."
    echo "Commenting it for now."
fi
