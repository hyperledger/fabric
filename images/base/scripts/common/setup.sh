#!/bin/bash

# ALERT: if you encounter an error like:
# error: [Errno 1] Operation not permitted: 'cf_update.egg-info/requires.txt'
# The proper fix is to remove any "root" owned directories under your update-cli directory
# as source mount-points only work for directories owned by the user running vagrant

# Stop on first error
set -e
set -x

# Update the entire system to the latest releases
apt-get update -qq
apt-get dist-upgrade -qqy

# install git
apt-get install --yes git

# Set Go environment variables needed by other scripts
export GOPATH="/opt/gopath"

#install golang
#apt-get install --yes golang
mkdir -p $GOPATH
MACHINE=`uname -m`
if [ x$MACHINE = xs390x ]
then
   apt-get install --yes golang
   export GOROOT="/usr/lib/go-1.6"
elif [ x$MACHINE = xppc64 ]
then
   echo "TODO: Add PPC support"
   exit
elif [ x$MACHINE = xx86_64 ]
then
   export GOROOT="/opt/go"

   #ARCH=`uname -m | sed 's|i686|386|' | sed 's|x86_64|amd64|'`
   ARCH=amd64
   GO_VER=1.6

   cd /tmp
   wget --quiet --no-check-certificate https://storage.googleapis.com/golang/go$GO_VER.linux-${ARCH}.tar.gz
   tar -xvf go$GO_VER.linux-${ARCH}.tar.gz
   mv go $GOROOT
   chmod 775 $GOROOT
   rm go$GO_VER.linux-${ARCH}.tar.gz
fi

PATH=$GOROOT/bin:$GOPATH/bin:$PATH

cat <<EOF >/etc/profile.d/goroot.sh
export GOROOT=$GOROOT
export GOPATH=$GOPATH
export PATH=\$PATH:$GOROOT/bin:$GOPATH/bin
EOF


# Install NodeJS

if [ x$MACHINE = xs390x ]
then
    apt-get install --yes nodejs
else
    NODE_VER=0.12.7
    NODE_PACKAGE=node-v$NODE_VER-linux-x64.tar.gz
    TEMP_DIR=/tmp
    SRC_PATH=$TEMP_DIR/$NODE_PACKAGE

    # First remove any prior packages downloaded in case of failure
    cd $TEMP_DIR
    rm -f node*.tar.gz
    wget --quiet https://nodejs.org/dist/v$NODE_VER/$NODE_PACKAGE
    cd /usr/local && sudo tar --strip-components 1 -xzf $SRC_PATH
fi

# Install GRPC

# ----------------------------------------------------------------
# NOTE: For instructions, see https://github.com/google/protobuf
#
# ----------------------------------------------------------------

# First install protoc
cd /tmp
git clone https://github.com/google/protobuf.git
cd protobuf
git checkout v3.0.0-beta-3
#unzip needed for ./autogen.sh
apt-get install -y unzip
apt-get install -y autoconf
apt-get install -y build-essential libtool
./autogen.sh
# NOTE: By default, the package will be installed to /usr/local. However, on many platforms, /usr/local/lib is not part of LD_LIBRARY_PATH.
# You can add it, but it may be easier to just install to /usr instead.
#
# To do this, invoke configure as follows:
#
# ./configure --prefix=/usr
#
#./configure
./configure --prefix=/usr

if [ x$MACHINE = xs390x ]
then
    echo FIXME: protobufs wont compile on 390, missing atomic call
else
    make
    make check
    make install
fi
export LD_LIBRARY_PATH=/usr/local/lib:$LD_LIBRARY_PATH
cd ~/

# Install rocksdb
apt-get install -y libsnappy-dev zlib1g-dev libbz2-dev
cd /tmp
git clone https://github.com/facebook/rocksdb.git
cd rocksdb
git checkout tags/v4.1
if [ x$MACHINE = xs390x ]
then
    echo There were some bugs in 4.1 for x/p, dev stream has the fix, living dangereously, fixing in place
    sed -i -e "s/-march=native/-march=z196/" build_tools/build_detect_platform
    sed -i -e "s/-momit-leaf-frame-pointer/-DDUMBDUMMY/" Makefile
fi

PORTABLE=1 make shared_lib
INSTALL_PATH=/usr/local make install-shared
ldconfig
cd ~/

# Make our versioning persistent
echo $BASEIMAGE_RELEASE > /etc/hyperledger-baseimage-release

# clean up our environment
apt-get -y autoremove
apt-get clean
rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
