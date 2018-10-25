#!/bin/bash
# Copyright Hitachi, Ltd. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

COMMIT_FILES=`git diff --name-only --diff-filter=ACMRTUXB HEAD | grep -Ev '(^|/)vendor/'`

echo "Checking trailing spaces ..."
for filename in `echo $COMMIT_FILES`; do
  if [[ `file $filename` == *"ASCII text"* ]];
  then
    if [ ! -z "`egrep -l " +$" $filename`" ];
    then
      FOUND_TRAILING='yes'
      echo "Error: Trailing spaces found in file:$filename, lines:"
      egrep -n " +$" $filename
    fi
  fi
done

if [ ! -z ${FOUND_TRAILING+x} ];
then
  echo "Please omit trailing spaces and make again."
  exit 1
fi
