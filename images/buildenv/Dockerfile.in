# Copyright Greg Haskins All Rights Reserved
#
# SPDX-License-Identifier: Apache-2.0

FROM _BASE_NS_/fabric-baseimage:_BASE_TAG_
COPY payload/protoc-gen-go /usr/local/bin/
ADD payload/gotools.tar.bz2 /usr/local/bin/

# override GOCACHE=off from fabric-baseimage
ENV GOCACHE "/tmp"
