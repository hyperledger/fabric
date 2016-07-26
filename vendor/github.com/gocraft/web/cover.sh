#!/usr/bin/env bash

go test -covermode=count -coverprofile=count.out .
go tool cover -html=count.out
go tool cover -func=count.out