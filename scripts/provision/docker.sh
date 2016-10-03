#!/bin/bash

# ---------------------------------------------------------------------------
# Install the hyperledger/fabric-baseimage docker environment
# ---------------------------------------------------------------------------
#
# There are some interesting things to note here:
#
# 1) Note that we take the slightly unorthodox route of _not_ publishing
#    a "latest" tag to dockerhub.  Rather, we only publish specifically
#    versioned images and we build the notion of "latest" here locally
#    during provisioning.  This is because the notion of always
#    pulling the latest/greatest from the net doesn't really apply to us;
#    we always want a coupling between the fabric and the docker environment.
#    At the same time, requiring each and every Dockerfile to pull a specific
#    version adds overhead to the Dockerfile generation logic.  Therefore,
#    we employ a hybrid solution that capitalizes on how docker treats the
#    "latest" tag.  That is, untagged references implicitly assume the tag
#    "latest" (good for simple Dockerfiles), but will satisfy the tag from
#    the local cache before going to the net (good for helping us control
#    what "latest" means locally)
#
#    A good blog entry covering the mechanism being exploited may be found here:
#
#          http://container-solutions.com/docker-latest-confusion
#
# 2) A benefit of (1) is that we now have a convenient vehicle for performing
#    JIT customizations of our docker image during provisioning just like we
#    do for vagrant.  For example, we can install new packages in docker within
#    this script.  We will capitalize on this in future patches.
#
# 3) Note that we do some funky processing of the environment (see "printenv"
#    and "ENV" components below).  Whats happening is we are providing a vehicle
#    for allowing the baseimage to include environmental definitions using
#    standard linux mechanisms (e.g. /etc/profile.d).  The problem is that
#    docker-run by default runs a non-login/non-interactive /bin/dash shell
#    which omits any normal /etc/profile or ~/.bashrc type processing, including
#    environment variable definitions.  So what we do is we force the execution
#    of an interactive shell and extract the defined environment variables
#    (via "printenv") and then re-inject them (using Dockerfile::ENV) in a
#    manner that will make them visible to a non-interactive DASH shell.
#
#    This helps for things like defining things such as the GOPATH.
#
#    An alternative would be to bake any Dockerfile::ENV items in during
#    baseimage creation, but packer lacks the capability to do so, so this
#    is a compromise.
# ---------------------------------------------------------------------------

NAME=hyperledger/fabric-baseimage
RELEASE=`uname -m`-$1
DOCKERHUB_NAME=$NAME:$RELEASE

CURDIR=`dirname $0`

docker inspect $DOCKERHUB_NAME 2>&1 > /dev/null
if [ "$?" == "0" ]; then
    echo "BUILD-CACHE: exists!"
    BASENAME=$DOCKERHUB_NAME
else
    echo "BUILD-CACHE: Pulling \"$DOCKERHUB_NAME\" from dockerhub.."
    docker pull $DOCKERHUB_NAME
    docker inspect $DOCKERHUB_NAME 2>&1 > /dev/null
    if [ "$?" == "0" ]; then
	echo "BUILD-CACHE: Success!"
	BASENAME=$DOCKERHUB_NAME
    else
	echo "BUILD-CACHE: WARNING - Build-cache unavailable, attempting local build"
	(cd $CURDIR/../../images/base && make docker DOCKER_TAG=localbuild)
	if [ "$?" != "0" ]; then
            echo "ERROR: Build-cache could not be compiled locally"
            exit -1
	fi
	BASENAME=$NAME:localbuild
    fi
fi

# Ensure that we have the baseimage we are expecting
docker inspect $BASENAME 2>&1 > /dev/null
if [ "$?" != "0" ]; then
   echo "ERROR: Unable to obtain a baseimage"
   exit -1
fi

# any further errors should be fatal
set -e

TMP=`mktemp -d`
DOCKERFILE=$TMP/Dockerfile

LOCALSCRIPTS=$TMP/scripts
REMOTESCRIPTS=/hyperledger/scripts/provision

mkdir -p $LOCALSCRIPTS
cp -R $CURDIR/* $LOCALSCRIPTS

# extract the FQN environment and run our common.sh to create the :latest tag
cat <<EOF > $DOCKERFILE
FROM $BASENAME
`for i in \`docker run -i $BASENAME /bin/bash -l -c printenv\`;
do
   echo ENV $i
done`
COPY scripts $REMOTESCRIPTS
RUN $REMOTESCRIPTS/common.sh
RUN chmod a+rw -R /opt/gopath

EOF

[ ! -z "$http_proxy" ] && DOCKER_ARGS_PROXY="$DOCKER_ARGS_PROXY --build-arg http_proxy=$http_proxy"
[ ! -z "$https_proxy" ] && DOCKER_ARGS_PROXY="$DOCKER_ARGS_PROXY --build-arg https_proxy=$https_proxy"
[ ! -z "$HTTP_PROXY" ] && DOCKER_ARGS_PROXY="$DOCKER_ARGS_PROXY --build-arg HTTP_PROXY=$HTTP_PROXY"
[ ! -z "$HTTPS_PROXY" ] && DOCKER_ARGS_PROXY="$DOCKER_ARGS_PROXY --build-arg HTTPS_PROXY=$HTTPS_PROXY"
[ ! -z "$no_proxy" ] && DOCKER_ARGS_PROXY="$DOCKER_ARGS_PROXY --build-arg no_proxy=$no_proxy"
[ ! -z "$NO_PROXY" ] && DOCKER_ARGS_PROXY="$DOCKER_ARGS_PROXY --build-arg NO_PROXY=$NO_PROXY"
docker build $DOCKER_ARGS_PROXY -t $NAME:latest $TMP

rm -rf $TMP
