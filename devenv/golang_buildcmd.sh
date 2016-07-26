#!/bin/sh
set -e
for arch in 8 6; do
        for cmd in a c g l; do
                go tool dist install -v cmd/$arch$cmd
        done
done
exit 0
