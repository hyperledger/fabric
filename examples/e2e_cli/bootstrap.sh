#!/bin/bash

export ARCH=$(uname -s|tr '[:upper:]' '[:lower:]'|sed 's/mingw64_nt.*/windows/')-$(go env GOARCH)

curl https://nexus.hyperledger.org/content/repositories/logs/sandbox/fabric-binary/${ARCH}-1.0.0-alpha2.tar.gz | tar xz
cd release/${ARCH}
sh download-dockerimages.sh -c $(uname -m)-1.0.0-alpha2 -f $(uname -m)-1.0.0-alpha2
docker images


