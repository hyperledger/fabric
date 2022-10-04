#!/usr/bin/env sh

#
# SPDX-License-Identifier: Apache-2.0
#

SUCCESS="✅"
WARN="⚠️ "
EXIT=0

# Check git
if ! command -v git > /tmp/fabcheckcmd
then
    echo "${WARN} Please install Git"
    EXIT=1
else
    echo "${SUCCESS} $(git --version) found: $(cat /tmp/fabcheckcmd)"
fi

# Check curl
if ! command -v curl > /tmp/fabcheckcmd
then
    echo "${WARN} Please install cURL"
    EXIT=1
else
    echo "${SUCCESS} $(curl --version | head -n 1 | cut -d ' ' -f 1,2,3) found: $(cat /tmp/fabcheckcmd)"
fi

# Check docker
if ! command -v docker > /tmp/fabcheckcmd
then
    echo "${WARN} Please install Docker"
    EXIT=1
else
    echo "${SUCCESS} $(docker --version) found: $(cat /tmp/fabcheckcmd)"
fi

# Check go
if ! command -v go > /tmp/fabcheckcmd
then
    echo "${WARN} Please install Go"
    EXIT=1
else
    echo "${SUCCESS} $(go version) found: $(cat /tmp/fabcheckcmd)"
fi

# Check jq
if ! command -v jq > /tmp/fabcheckcmd
then
  echo "${WARN} Please install jq"
  EXIT=1
else
  echo "${SUCCESS} jq $(jq --version) found: $(cat /tmp/fabcheckcmd)"
fi

rm /tmp/fabcheckcmd > /dev/null

exit $EXIT
