#!/bin/bash

# Development on Z is done on the native OS, not in Vagrant. This script can be 
# used to set things up in RHEL on Z, similar to devenv/setup.sh which does the 
# same for Vagrant. 
# See https://github.com/hyperledger/fabric/blob/master/docs/dev-setup/install.md
#
# To get started:
#       sudo su
#       yum install git
#       mkdir -p $HOME/git/src/github.com/hyperledger
#       cd $HOME/git/src/github.com/hyperledger
#       git clone http://gerrit.hyperledger.org/r/fabric
#       source fabric/devenv/setupRHELonZ.sh
#       make peer unit-test behave

if [ xroot != x$(whoami) ]
then
   echo "You must run as root (Hint: sudo su)"
   exit
fi

if [ -n -d $HOME/git/src/github.com/hyperledger/fabric ]
then
    echo "Script fabric code is under $HOME/git/src/github.com/hyperledger/fabric "
    exit
fi

#TODO: should really just open a few ports..
iptables -I INPUT 1 -j ACCEPT
sysctl vm.overcommit_memory=1

##################
# Install Docker
cd /tmp
wget ftp://ftp.unicamp.br/pub/linuxpatch/s390x/redhat/rhel7.2/docker-1.10.1-rhel7.2-20160408.tar.gz
tar -xvzf docker-1.10.1-rhel7.2-20160408.tar.gz
cp docker-1.10.1-rhel7.2-20160408/docker /bin
rm -rf docker docker-1.10.1-rhel7.2-20160408.tar.gz

#TODO: Install on boot
nohup docker daemon -g /data/docker -H tcp://0.0.0.0:2375 -H unix:///var/run/docker.sock&

###################################
# Crosscompile and install GOLANG
cd $HOME
git clone http://github.com/linux-on-ibm-z/go.git
cd go
git checkout release-branch.go1.6

cat > crosscompile.sh <<HEREDOC
cd /tmp/home/go/src	
yum install -y git wget tar gcc bzip2
export GOROOT_BOOTSTRAP=/usr/local/go
GOOS=linux GOARCH=s390x ./bootstrap.bash
HEREDOC

docker run --privileged --rm -ti -v $HOME:/tmp/home brunswickheads/openchain-peer /bin/bash /tmp/home/go/crosscompile.sh

export GOROOT_BOOTSTRAP=$HOME/go-linux-s390x-bootstrap
cd $HOME/go/src
./all.bash
export PATH=$HOME/go/bin:$PATH

rm -rf $HOME/go-linux-s390x-bootstrap 

################
#ROCKSDB BUILD

cd /tmp
yum install -y gcc-c++ snappy snappy-devel zlib zlib-devel bzip2 bzip2-devel
git clone https://github.com/facebook/rocksdb.git
cd  rocksdb
git checkout tags/v4.1
echo There were some bugs in 4.1 for x/p, dev stream has the fix, living dangereously, fixing in place
sed -i -e "s/-march=native/-march=zEC12/" build_tools/build_detect_platform
sed -i -e "s/-momit-leaf-frame-pointer/-DDUMBDUMMY/" Makefile
make shared_lib && INSTALL_PATH=/usr make install-shared && ldconfig
cd /tmp
rm -rf /tmp/rocksdb

################
# PIP
yum install python-setuptools
curl "https://bootstrap.pypa.io/get-pip.py" -o "get-pip.py"
python get-pip.py
pip install --upgrade pip
pip install behave nose docker-compose

################
#grpcio package

git clone https://github.com/grpc/grpc.git
cd grpc
pip install -rrequirements.txt
git checkout tags/release-0_13_1
sed -i -e "s/boringssl.googlesource.com/github.com\/linux-on-ibm-z/" .gitmodules
git submodule sync
git submodule update --init
cd third_party/boringssl
git checkout s390x-big-endian
cd ../..
GRPC_PYTHON_BUILD_WITH_CYTHON=1 pip install .

# updater-server, update-engine, and update-service-common dependencies (for running locally)
pip install -I flask==0.10.1 python-dateutil==2.2 pytz==2014.3 pyyaml==3.10 couchdb==1.0 flask-cors==2.0.1 requests==2.4.3
cat >> ~/.bashrc <<HEREDOC
      export PATH=$HOME/go/bin:$PATH
      export GOROOT=$HOME/go
      export GOPATH=$HOME/git
HEREDOC

source ~/.bashrc

# Build the actual hyperledger peer
cd $GOPATH/src/github.com/hyperledger/fabric
make clean peer
