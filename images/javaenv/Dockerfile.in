# Copyright Greg Haskins All Rights Reserved
#
# SPDX-License-Identifier: Apache-2.0
#
FROM _BASE_NS_/fabric-baseimage:_BASE_TAG_
RUN curl -sSL https://services.gradle.org/distributions/gradle-2.12-bin.zip > /tmp/gradle-2.12-bin.zip
RUN unzip -qo /tmp/gradle-2.12-bin.zip -d /opt && rm /tmp/gradle-2.12-bin.zip
RUN ln -s /opt/gradle-2.12/bin/gradle /usr/bin
ENV MAVEN_VERSION=3.3.9
ENV USER_HOME_DIR="/root"
RUN mkdir -p /usr/share/maven /usr/share/maven/ref \
  && curl -fsSL http://apache.osuosl.org/maven/maven-3/$MAVEN_VERSION/binaries/apache-maven-$MAVEN_VERSION-bin.tar.gz \
    | tar -xzC /usr/share/maven --strip-components=1 \
  && ln -s /usr/share/maven/bin/mvn /usr/bin/mvn
ENV MAVEN_HOME /usr/share/maven
ENV MAVEN_CONFIG "$USER_HOME_DIR/.m2"
ADD payload/javashim.tar.bz2 /root
ADD payload/protos.tar.bz2 /root
ADD payload/settings.gradle /root
WORKDIR /root
# Build java shim after copying proto files from fabric/proto
RUN core/chaincode/shim/java/javabuild.sh
