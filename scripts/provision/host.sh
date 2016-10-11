#!/bin/bash

CURDIR=`dirname $0`

$CURDIR/common.sh

# Install docker-compose
curl -L https://github.com/docker/compose/releases/download/1.5.2/docker-compose-`uname -s`-`uname -m` > /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose

# Install Python, pip, behave, nose
#
# install python-dev and libyaml-dev to get compiled speedups
apt-get install --yes python-dev
apt-get install --yes libyaml-dev

apt-get install --yes python-setuptools
apt-get install --yes python-pip
pip install --upgrade pip
pip install behave
pip install nose

# updater-server, update-engine, and update-service-common dependencies (for running locally)
pip install -I flask==0.10.1 python-dateutil==2.2 pytz==2014.3 pyyaml==3.10 couchdb==1.0 flask-cors==2.0.1 requests==2.4.3

# Python grpc package for behave tests
# Required to update six for grpcio
pip install --ignore-installed six
pip install --upgrade 'grpcio==0.13.1'

# install ruby and apiaryio
#apt-get install --yes ruby ruby-dev gcc
#gem install apiaryio

# Install Tcl prerequisites for busywork
apt-get install --yes tcl tclx tcllib

# Install NPM for the SDK
apt-get install --yes npm

# Install JDK 1.8 for Java chaincode development
ARCH=`uname -m`
if [ x$ARCH == xx86_64 ]
then
add-apt-repository ppa:openjdk-r/ppa -y
fi
apt-get update && apt-get install openjdk-8-jdk -y

# Download Gradle and create sym link
wget https://services.gradle.org/distributions/gradle-2.12-bin.zip -P /tmp --quiet
unzip -q /tmp/gradle-2.12-bin.zip -d /opt && rm /tmp/gradle-2.12-bin.zip
ln -s /opt/gradle-2.12/bin/gradle /usr/bin

# Download maven for supporting maven build in java chaincode
MAVEN_VERSION=3.3.9
mkdir -p /usr/share/maven /usr/share/maven/ref
curl -fsSL http://apache.osuosl.org/maven/maven-3/$MAVEN_VERSION/binaries/apache-maven-$MAVEN_VERSION-bin.tar.gz \
    | tar -xzC /usr/share/maven --strip-components=1 \
  && ln -s /usr/share/maven/bin/mvn /usr/bin/mvn

if [ x$ARCH == xx86_64 ]
then
# Set the default JDK to 1.8
update-java-alternatives -s java-1.8.0-openjdk-amd64
fi
