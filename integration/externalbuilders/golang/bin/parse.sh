#!/bin/bash

# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

while (( "$#" )); do
  case "$1" in
    --artifacts)
      ARTIFACTS=$2
      shift 2
      ;;
    "--package-id")
      PACKAGE_ID=$2
      shift 2
      ;;
    "--path")
      GO_PACKAGE_PATH=$2
      shift 2
      ;;
    "--type")
      TYPE=$2
      shift 2
      ;;
    "--source")
      SOURCE=$2
      shift 2
      ;;
    "--output")
      OUTPUT=$2
      shift 2
      ;;
    --) # end argument parsing
      shift
      break
      ;;
    -*|--*=) # unsupported flags
      echo "Error: Unsupported flag $1" >&2
      exit 1
      ;;
  esac
done
