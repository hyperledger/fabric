#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


set -e
set -x

DEVENV_REVISION=`(cd /hyperledger/fabric/devenv; git rev-parse --short HEAD)`

# Install WARNING before we start provisioning so that it
# will remain active.  We will remove the warning after
# success
SCRIPT_DIR="$(readlink -f "$(dirname "$0")")"
cat "$SCRIPT_DIR/failure-motd.in" >> /etc/motd

# Update the entire system to the latest releases
apt-get update
#apt-get dist-upgrade -y

# Install some basic utilities
apt-get install -y build-essential git make curl unzip g++ libtool

# ----------------------------------------------------------------
# Install Docker
# ----------------------------------------------------------------

# Update system
apt-get update -qq

# Prep apt-get for docker install
apt-get install -y apt-transport-https ca-certificates
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -

# Add docker repository
add-apt-repository \
   "deb [arch=amd64] https://download.docker.com/linux/ubuntu \
   $(lsb_release -cs) \
   stable"

# Update system
apt-get update -qq

# Install docker
#apt-get install -y docker-ce=17.06.2~ce~0~ubuntu  # in case we need to set the version
apt-get install -y docker-ce

# Install docker-compose
curl -L https://github.com/docker/compose/releases/download/1.14.0/docker-compose-`uname -s`-`uname -m` > /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose

usermod -a -G docker ubuntu # Add ubuntu user to the docker group

# Test docker
docker run --rm busybox echo All good

# ----------------------------------------------------------------
# Install Golang
# ----------------------------------------------------------------
GO_VER=1.12.12
GO_URL=https://storage.googleapis.com/golang/go${GO_VER}.linux-amd64.tar.gz

# Set Go environment variables needed by other scripts
export GOPATH="/opt/gopath"
export GOROOT="/opt/go"
PATH=$GOROOT/bin:$GOPATH/bin:$PATH

cat <<EOF >/etc/profile.d/goroot.sh
export GOROOT=$GOROOT
export GOPATH=$GOPATH
export PATH=\$PATH:$GOROOT/bin:$GOPATH/bin
EOF

mkdir -p $GOROOT

curl -sL $GO_URL | (cd $GOROOT && tar --strip-components 1 -xz)

# ----------------------------------------------------------------
# Install nvm and Node.js
# ----------------------------------------------------------------
runuser -l ubuntu -c '/hyperledger/fabric/devenv/install_nvm.sh'

# ----------------------------------------------------------------
# Install Java
# ----------------------------------------------------------------
apt-get install -y openjdk-8-jdk maven

wget https://services.gradle.org/distributions/gradle-4.4.1-bin.zip -P /tmp --quiet
unzip -q /tmp/gradle-4.4.1-bin.zip -d /opt && rm /tmp/gradle-4.4.1-bin.zip
ln -s /opt/gradle-4.4.1/bin/gradle /usr/bin

# ----------------------------------------------------------------
# Misc tasks
# ----------------------------------------------------------------

# Create directory for the DB
sudo mkdir -p /var/hyperledger
sudo chown -R ubuntu:ubuntu /var/hyperledger

# clean any previous builds as they may have image/.dummy files without
# the backing docker images (since we are, by definition, rebuilding the
# filesystem) and then ensure we have a fresh set of our go-tools.
# NOTE: This must be done before the chown below
cd $GOPATH/src/github.com/hyperledger/fabric
make clean gotools

# Ensure permissions are set for GOPATH
sudo chown -R ubuntu:ubuntu $GOPATH

# Update limits.conf to increase nofiles for LevelDB and network connections
sudo cp /hyperledger/fabric/devenv/limits.conf /etc/security/limits.conf

# Configure vagrant specific environment
cat <<EOF >/etc/profile.d/vagrant-devenv.sh
# Expose the devenv/tools in the $PATH
export PATH=\$PATH:/hyperledger/fabric/devenv/tools:/hyperledger/fabric/.build/bin
export FABRIC_CFG_PATH=/hyperledger/fabric/sampleconfig/
export VAGRANT=1
export CGO_CFLAGS=" "
EOF

# Set our shell prompt to something less ugly than the default from packer
# Also make it so that it cd's the user to the fabric dir upon logging in
cat <<EOF >> /home/ubuntu/.bashrc
PS1="\u@hyperledger-devenv:$DEVENV_REVISION:\w$ "
cd $GOPATH/src/github.com/hyperledger/fabric/
EOF

# finally, remove our warning so the user knows this was successful
rm /etc/motd
