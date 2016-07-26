#!/bin/bash

# Add any logic that is common to both the peer and docker environments here

apt-get update -qq

# Used by CHAINTOOL
apt-get install -y default-jre
