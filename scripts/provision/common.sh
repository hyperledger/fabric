#!/bin/bash

# Add any logic that is common to both the peer and docker environments here

# Use the mirror protocol for apt so that geographically close mirrors are automatically chosen
source /etc/lsb-release

cat <<EOF > /etc/apt/sources.list
deb mirror://mirrors.ubuntu.com/mirrors.txt $DISTRIB_CODENAME main restricted universe multiverse
deb mirror://mirrors.ubuntu.com/mirrors.txt $DISTRIB_CODENAME-updates main restricted universe multiverse
deb mirror://mirrors.ubuntu.com/mirrors.txt $DISTRIB_CODENAME-backports main restricted universe multiverse
deb mirror://mirrors.ubuntu.com/mirrors.txt $DISTRIB_CODENAME-security main restricted universe multiverse
EOF

apt-get update -qq

# Used by CHAINTOOL
apt-get install -y default-jre
