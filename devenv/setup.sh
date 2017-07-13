#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


set -e
set -x

DEVENV_REVISION=`(cd /hyperledger/devenv; git rev-parse --short HEAD)`

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

# Storage backend logic
case "${DOCKER_STORAGE_BACKEND}" in
  aufs|AUFS|"")
    DOCKER_STORAGE_BACKEND_STRING="aufs" ;;
  btrfs|BTRFS)
    # mkfs
    apt-get install -y btrfs-tools
    mkfs.btrfs -f /dev/sdb
    rm -Rf /var/lib/docker
    mkdir -p /var/lib/docker
    . <(sudo blkid -o udev /dev/sdb)
    echo "UUID=${ID_FS_UUID} /var/lib/docker btrfs defaults 0 0" >> /etc/fstab
    mount /var/lib/docker

    DOCKER_STORAGE_BACKEND_STRING="btrfs" ;;
  *) echo "Unknown storage backend ${DOCKER_STORAGE_BACKEND}"
     exit 1;;
esac

# Update system
apt-get update -qq

# Prep apt-get for docker install
apt-get install -y apt-transport-https ca-certificates
apt-key adv --keyserver hkp://p80.pool.sks-keyservers.net:80 --recv-keys 58118E89F3A912897C070ADBF76221572C52609D

# Add docker repository
echo deb https://apt.dockerproject.org/repo ubuntu-xenial main > /etc/apt/sources.list.d/docker.list

# Update system
apt-get update -qq

# Install docker
apt-get install -y linux-image-extra-$(uname -r) apparmor docker-engine

# Install docker-compose
curl -L https://github.com/docker/compose/releases/download/1.8.1/docker-compose-`uname -s`-`uname -m` > /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose

# Configure docker
DOCKER_OPTS="-s=${DOCKER_STORAGE_BACKEND_STRING} -r=true --api-cors-header='*' -H tcp://0.0.0.0:2375 -H unix:///var/run/docker.sock ${DOCKER_OPTS}"
sed -i.bak '/^DOCKER_OPTS=/{h;s|=.*|=\"'"${DOCKER_OPTS}"'\"|};${x;/^$/{s||DOCKER_OPTS=\"'"${DOCKER_OPTS}"'\"|;H};x}' /etc/default/docker

service docker restart
usermod -a -G docker ubuntu # Add ubuntu user to the docker group

# Test docker
docker run --rm busybox echo All good

# ----------------------------------------------------------------
# Install Golang
# ----------------------------------------------------------------
GO_VER=1.7.5
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
# Install NodeJS
# ----------------------------------------------------------------
NODE_VER=6.9.5
NODE_URL=https://nodejs.org/dist/v$NODE_VER/node-v$NODE_VER-linux-x64.tar.gz

curl -sL $NODE_URL | (cd /usr/local && tar --strip-components 1 -xz )

# ----------------------------------------------------------------
# Install Behave
# ----------------------------------------------------------------
/hyperledger/scripts/install_behave.sh

# ----------------------------------------------------------------
# Install Java
# ----------------------------------------------------------------
apt-get install -y openjdk-8-jdk maven

wget https://services.gradle.org/distributions/gradle-2.12-bin.zip -P /tmp --quiet
unzip -q /tmp/gradle-2.12-bin.zip -d /opt && rm /tmp/gradle-2.12-bin.zip
ln -s /opt/gradle-2.12/bin/gradle /usr/bin

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
sudo cp /hyperledger/devenv/limits.conf /etc/security/limits.conf

# Configure vagrant specific environment
cat <<EOF >/etc/profile.d/vagrant-devenv.sh
# Expose the devenv/tools in the $PATH
export PATH=\$PATH:/hyperledger/devenv/tools:/hyperledger/build/bin
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
