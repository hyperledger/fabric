#!/bin/bash -e

# Copyright IBM Corp All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

go mod verify
# check vendor by restoring it from module definition and checking if
# there are any differences from the code which is committed
go mod vendor
if [ -n "$(git diff HEAD --stat -- vendor/)" ] ;
then
    echo "vendor modified"
    git diff -- vendor/
    exit 1
fi

