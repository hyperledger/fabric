#!/bin/bash -e
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

############################################
# Pull "1.2.1-stable" docker images from nexus3
# Tag it as $ARCH-$BASE_VERSION (1.2.1)
# Push tagged images to hyperledger dockerhub
#############################################

export ORG_NAME=hyperledger/fabric
export NEXUS_URL=nexus3.hyperledger.org:10001
export STABLE_VERSION=${STABLE_VERSION:-1.2.1-stable}
export BASE_VERSION=${BASE_VERSION:-1.2.1}
export IMAGES_LIST=(peer orderer ccenv tools)
export CA_IMAGES_LIST=(ca)
export THIRDPARTY_IMAGES_LIST=(kafka couchdb zookeeper)

ARCH=$(go env GOARCH)
if [ "$ARCH" = "amd64" ]; then
	ARCH=amd64
else
    ARCH=$(uname -m)
fi

printHelp() {
  echo "Usage: STABLE_VERSION=1.2.1-stable BASE_VERSION=1.2.1 ./scripts/pull_Build_Artifacts.sh --pull_Fabric_Images"
  echo
  echo "pull_All - pull fabric, fabric-ca and thirdparty images"
  echo "pull_Fabric_Images_All_Platforms - pull fabric images amd64, s390x"
  echo "cleanup - delete unused docker images"
  echo "pull_Fabric_Images - pull fabric docker images on current arch"
  echo "pull_Fabric_Binaries - pull fabric binaries on current arch"
  echo "pull_Thirdparty_Images - pull fabric thirdparty docker images on current arch"
  echo "pull_Ca_Images - pull fabric ca docker images on current arch"
  echo
  # STABLE_VERSION - Image will be pulled from Nexus3
  # BASE_VERSION - Tag it as BASE_VERSION in Makefile
  echo "e.g. STABLE_VERSION=1.2.1-stable BASE_VERSION=1.2.1 ./scripts/pull_build_artifacts.sh --pull_Fabric_Images"
  echo "e.g. STABLE_VERSION=1.2.1-stable BASE_VERSION=1.2.1 ./scripts/pull_build_artifacts.sh --pull_Ca_Images"

}

cleanup() {
    # Cleanup docker images
    make clean || true
    docker images -q | xargs docker rmi -f || true
}

# pull fabric, fabric-ca and thirdparty images, and binaries
pull_All() {

    echo "-------> pull thirdparty docker images"
    pull_Thirdparty_Images
    echo "-------> pull binaries"
    pull_Fabric_Binaries
    echo "-------> pull fabric docker images"
    pull_Fabric_Images
    echo "-------> pull fabric ca docker images"
    pull_Ca_Images
}

# pull fabric docker images
pull_Fabric_Images() {
    for IMAGES in ${IMAGES_LIST[*]}; do
         docker pull $NEXUS_URL/$ORG_NAME-$IMAGES:$ARCH-$STABLE_VERSION
         docker tag $NEXUS_URL/$ORG_NAME-$IMAGES:$ARCH-$STABLE_VERSION $ORG_NAME-$IMAGES:$ARCH-$BASE_VERSION
         docker tag $NEXUS_URL/$ORG_NAME-$IMAGES:$ARCH-$STABLE_VERSION $ORG_NAME-$IMAGES
         docker rmi -f $NEXUS_URL/$ORG_NAME-$IMAGES:$ARCH-$STABLE_VERSION
    done
}

# pull fabric binaries
pull_Fabric_Binaries() {
    export MARCH=$(echo "$(uname -s|tr '[:upper:]' '[:lower:]'|sed 's/mingw64_nt.*/windows/')-$(uname -m | sed 's/x86_64/amd64/g')" | awk '{print tolower($0)}')
    echo "-------> MARCH:" $MARCH
    echo "-------> pull stable binaries for all platforms (x and z)"
    MVN_METADATA=$(echo "https://nexus.hyperledger.org/content/repositories/releases/org/hyperledger/fabric/hyperledger-fabric-$STABLE_VERSION/maven-metadata.xml")
    curl -L "$MVN_METADATA" > maven-metadata.xml
    RELEASE_TAG=$(cat maven-metadata.xml | grep release)
    COMMIT=$(echo $RELEASE_TAG | awk -F - '{ print $4 }' | cut -d "<" -f1)
    echo "-------> COMMIT:" $COMMIT
    curl https://nexus.hyperledger.org/content/repositories/releases/org/hyperledger/fabric/hyperledger-fabric-$STABLE_VERSION/$MARCH.$STABLE_VERSION-$COMMIT/hyperledger-fabric-$STABLE_VERSION-$MARCH.$STABLE_VERSION-$COMMIT.tar.gz | tar xz
    if [ $? != 0 ]; then
	echo "-------> FAILED to pull fabric binaries"
        exit 1
    fi
}

# pull fabric docker images from amd64 and s390x platforms
pull_Fabric_Images_All_Platforms() {

    # pull stable images from nexus and tag to hyperledger
    echo "-------> pull docker images for all platforms (x, z)"
    for arch in amd64 s390x; do
        for IMAGES in ${IMAGES_LIST[*]}; do
            docker pull $NEXUS_URL/$ORG_NAME-$IMAGES:$arch-$STABLE_VERSION
            docker tag $NEXUS_URL/$ORG_NAME-$IMAGES:$arch-$STABLE_VERSION $ORG_NAME-$IMAGES:$arch-$BASE_VERSION
            docker rmi -f $NEXUS_URL/$ORG_NAME-$IMAGES:$arch-$STABLE_VERSION
        done
    done
}

# pull thirdparty docker images from nexus
pull_Thirdparty_Images() {
    echo "------> pull thirdparty docker images from nexus"
    BASEIMAGE_VERSION=$(curl --silent  https://raw.githubusercontent.com/hyperledger/fabric/master/Makefile 2>&1 | tee Makefile | grep "BASEIMAGE_RELEASE=" | cut -d "=" -f2)
    for IMAGES in ${THIRDPARTY_IMAGES_LIST[*]}; do
          docker pull $NEXUS_URL/$ORG_NAME-$IMAGES:$ARCH-$BASEIMAGE_VERSION
          docker tag $NEXUS_URL/$ORG_NAME-$IMAGES:$ARCH-$BASEIMAGE_VERSION $ORG_NAME-$IMAGES
          docker tag $NEXUS_URL/$ORG_NAME-$IMAGES:$ARCH-$BASEIMAGE_VERSION $ORG_NAME-$IMAGES:$ARCH-$BASEIMAGE_VERSION
          docker rmi -f $NEXUS_URL/$ORG_NAME-$IMAGES:$ARCH-$BASEIMAGE_VERSION
    done
}

# pull fabric-ca docker images
pull_Ca_Images() {
        for IMAGES in ${CA_IMAGES_LIST[*]}; do
            docker pull $NEXUS_URL/$ORG_NAME-$IMAGES:$ARCH-$STABLE_VERSION
            docker tag $NEXUS_URL/$ORG_NAME-$IMAGES:$ARCH-$STABLE_VERSION $ORG_NAME-$IMAGES:$ARCH-$BASE_VERSION
            docker tag $NEXUS_URL/$ORG_NAME-$IMAGES:$ARCH-$STABLE_VERSION $ORG_NAME-$IMAGES
            docker rmi -f $NEXUS_URL/$ORG_NAME-$IMAGES:$ARCH-$STABLE_VERSION
        done
}

Parse_Arguments() {
    while [ $# -gt 0 ]; do
        case $1 in
            --cleanup)
                cleanup
                ;;
            --pull_All)
                pull_All
                ;;
            --pull_Thirdparty_Images)
                pull_Thirdparty_Images
                ;;
            --pull_Fabric_Binaries)
                pull_Fabric_Binaries
                ;;
            --pull_Fabric_Images)
                pull_Fabric_Images
                ;;
            --pull_Fabric_Images_All_Platforms)
                pull_Fabric_Images_All_Platforms
                ;;
            --pull_Ca_Images)
                pull_Ca_Images
                ;;
	    --printHelp)
		printHelp
		;;
        esac
        shift
    done
}
Parse_Arguments $@
