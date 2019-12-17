#!/bin/sh
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

cat > CHANGELOG.new << EOF
## "${2}"

$(git log "$1..HEAD"  --oneline | grep -v Merge | sed -e "s/\[\(FAB-[0-9]*\)\]/\[\1\](https:\/\/jira.hyperledger.org\/browse\/\1\)/" -e "s/ \(FAB-[0-9]*\)/ \[\1\](https:\/\/jira.hyperledger.org\/browse\/\1\)/" -e "s/\([0-9|a-z]*\)/* \[\1\](https:\/\/github.com\/hyperledger\/fabric\/commit\/\1)/")

EOF
cat CHANGELOG.md >> CHANGELOG.new
mv -f CHANGELOG.new CHANGELOG.md
