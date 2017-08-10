#!/bin/bash

./node_modules/.bin/uber-licence --version || npm i uber-licence@latest
./node_modules/.bin/uber-licence --dry --file "*.go"
