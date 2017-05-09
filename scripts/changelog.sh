#!/bin/sh

echo "## $2 $(date)" >> CHANGELOG.new
echo "" >> CHANGELOG.new
git log $1..$2  --oneline | grep -v Merge | sed -e "s/\[\(FAB-[0-9]*\)\]/\[\1\](https:\/\/jira.hyperledger.org\/browse\/\1\)/" -e "s/ \(FAB-[0-9]*\)/ \[\1\](https:\/\/jira.hyperledger.org\/browse\/\1\)/" -e "s/\([0-9|a-z]*\)/* \[\1\](https:\/\/github.com\/hyperledger\/fabric\/commit\/\1)/" >> CHANGELOG.new
echo "" >> CHANGELOG.new
cat CHANGELOG.md >> CHANGELOG.new
mv -f CHANGELOG.new CHANGELOG.md
