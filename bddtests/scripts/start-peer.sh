#!/bin/sh

SCRIPT_DIR=$(dirname $0)
MEMBERSHIP_IP=$(cat /etc/hosts | grep membersrvc | head -n 1 | cut -f1)
TIMEOUT=20

if [ -n "$MEMBERSHIP_IP" ]; then
    echo "membersrvc detected, waiting for it before starting with a $TIMEOUT second timout"
    "$SCRIPT_DIR"/wait-for-it.sh -t "$TIMEOUT" "$MEMBERSHIP_IP":7054

    if [ $? -ne 0 ]; then
        echo "Failed to contact membersrvc within $TIMEOUT seconds"
        exit 1
    fi
else
    echo "No membersrvc to wait for, starting immediately"
fi

exec peer node start

