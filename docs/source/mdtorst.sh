#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

FILES=*.md
for f in $FILES
do
  # extension="${f##*.}"
  filename="${f%.*}"
  echo "Converting $f to $filename.rst"
  `pandoc $f -t rst -o $filename.rst`
  # uncomment this line to delete the source file.
  # rm $f
done
