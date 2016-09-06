#!/bin/bash

# Development on Power (ppc64le) systems is done outside of vagrant, on the
# native OS. This script helps setup the dev env on ppc64le Ubuntu.
#
# See https://github.com/hyperledger/fabric/blob/master/docs/dev-setup/install.md#building-outside-of-vagrant-
#
# NOTE: This script assumes that
#	- OS specific apt-sources / repositories are already set appropriately.
#	- Host's GOPATH environment variable is set.
#
# To get started on a fresh Ubuntu install:
#	mkdir -p $GOPATH/src/github.com/hyperledger
#	cd $GOPATH/src/github.com/hyperledger
#	git clone http://gerrit.hyperledger.org/r/fabric
#	sudo ./fabric/devenv/setupUbuntuOnPPC64el.sh
#	cd $GOPATH/src/github.com/hyperledger/fabric
#	make dist-clean all

if [ xroot != x$(whoami) ]
then
   echo "You must run as root (Hint: Try prefix 'sudo' while executing the script)"
   exit
fi

if [ ! -d "$GOPATH/src/github.com/hyperledger/fabric" ]
then
    echo "Ensure fabric code is under $GOPATH/src/github.com/hyperledger/fabric"
    exit
fi

#####################################
# Install pre-requisite OS packages #
#####################################
apt-get update
apt-get -y install software-properties-common curl git sudo wget

#####################################
# Install and setup Docker services #
#####################################
# Along with docker.io, aufs-tools also needs to be installed as 'auplink' which is part of aufs-tools package gets invoked during behave tests.
apt-get -y install docker.io aufs-tools

# Set DOCKER_OPTS and restart Docker daemon.
sed  -i '/#DOCKER_OPTS=/a DOCKER_OPTS="-H tcp://0.0.0.0:2375 -H unix:///var/run/docker.sock"' /etc/default/docker
systemctl restart docker

####################################
# Install Go and set env variable  #
####################################
# Golang binaries for ppc64le are publicly available from Unicamp and is recommended as it includes certain platform specific tuning/optimization.
# Alternativley package part of Ubuntu disto repo can also be used.
wget ftp://ftp.unicamp.br/pub/linuxpatch/toolchain/at/ubuntu/dists/trusty/at9.0/binary-ppc64el/advance-toolchain-at9.0-golang_9.0-3_ppc64el.deb
dpkg -i advance-toolchain-at9.0-golang_9.0-3_ppc64el.deb
rm -f advance-toolchain-at9.0-golang_9.0-3_ppc64el.deb

# Create links under /usr/bin using update-alternatives
update-alternatives --install /usr/bin/go go /usr/local/go/bin/go 9
update-alternatives --install /usr/bin/gofmt gofmt /usr/local/go/bin/gofmt 9

# Set the GOROOT env variable
export GOROOT="/usr/local/go"

####################################
# Build and install RocksDB        #
####################################

apt-get -y install libsnappy-dev zlib1g-dev libbz2-dev "build-essential"

cd /tmp
git clone https://github.com/facebook/rocksdb.git
cd rocksdb
git checkout tags/v4.1
echo There were some bugs in 4.1. This was fixed in dev stream and newer releases like 4.5.1.
sed -ibak 's/ifneq ($(MACHINE),ppc64)/ifeq (,$(findstring ppc64,$(MACHINE)))/g' Makefile
PORTABLE=1 make shared_lib
INSTALL_PATH=/usr/local make install-shared
ldconfig
cd - ; rm -rf /tmp/rocksdb

################################################
# Install PIP tools, behave and docker-compose #
################################################

apt-get -y install python-pip
pip install --upgrade pip
pip install behave nose docker-compose

pip install -I flask==0.10.1 python-dateutil==2.2 pytz==2014.3 pyyaml==3.10 couchdb==1.0 flask-cors==2.0.1 requests==2.4.3 grpcio==0.13.1
