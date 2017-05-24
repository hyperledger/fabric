#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


#
# This script is used on Debian based linux distros.
# (i.e., linux that supports the apt packaging manager.)
#

# Update system
apt-get update -qq

# Install Python, pip, behave, nose
#
# install python-dev and libyaml-dev to get compiled speedups
apt-get install --yes python-dev
apt-get install --yes libyaml-dev

apt-get install --yes python-setuptools
apt-get install --yes python-pip
apt-get install --yes build-essential
pip install --upgrade pip
pip install behave
pip install nose

# updater-server, update-engine, and update-service-common dependencies (for running locally)
pip install -I flask==0.10.1 python-dateutil==2.2 pytz==2014.3 pyyaml==3.10 couchdb==1.0 flask-cors==2.0.1 requests==2.4.3 pyOpenSSL==16.2.0 pysha3==1.0b1

# Python grpc package for behave tests
# Required to update six for grpcio
pip install --ignore-installed six
pip install --upgrade 'grpcio==0.13.1'

# Pip packages required for some behave tests
pip install ecdsa python-slugify b3j0f.aop
pip install google
pip install protobuf
pip install pyyaml
pip install pykafka

# install ruby and apiaryio
#apt-get install --yes ruby ruby-dev gcc
#gem install apiaryio

# Install Tcl prerequisites for busywork
apt-get install --yes tcl tclx tcllib

# Install NPM for the SDK
apt-get install --yes npm
