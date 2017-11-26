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

# Install Python, pip, behave
#
# install python-dev and libyaml-dev to get compiled speedups
apt-get install --yes python-dev
apt-get install --yes libyaml-dev

apt-get install --yes python-setuptools
apt-get install --yes python-pip
apt-get install --yes build-essential
# required dependencies for cryptography, which is required by pyOpenSSL
# https://cryptography.io/en/stable/installation/#building-cryptography-on-linux
apt-get install --yes libssl-dev libffi-dev
pip install --upgrade pip

# Pip packages required for behave tests
pip install -r ../devenv/bddtests-requirements.txt

# install ruby and apiaryio
#apt-get install --yes ruby ruby-dev gcc
#gem install apiaryio

# Install Tcl prerequisites for busywork
apt-get install --yes tcl tclx tcllib

# Install NPM for the SDK
apt-get install --yes npm
