#!/bin/bash -eu
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

# ----------------------------------------------------------------
# Generate pkcs11 token for fabric tests
# ----------------------------------------------------------------
softhsm2-util --init-token --slot 0 --label "ForFabric" --so-pin 1234 --pin 98765432

cat <<EOF >>/home/vagrant/.bashrc

export PKCS11_LIB="$(find /usr/lib -name libsofthsm2.so | head -1)"
export PKCS11_PIN=98765432
export PKCS11_LABEL="ForFabric"
EOF

cat <<EOF >>/home/vagrant/.bashrc

export GOPATH=\$HOME/go
export PATH=\$PATH:\$HOME/go/bin
cd \$HOME/fabric
EOF
