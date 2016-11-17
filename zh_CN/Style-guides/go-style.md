## Coding guidelines

### Coding Golang <a name="coding-go"></a>

We code in Go&trade; and strictly follow the [best
practices](http://golang.org/doc/effective_go.html) and will not accept any
deviations. You must run the following tools against your Go code and fix all
errors and warnings:
  - [golint](https://github.com/golang/lint)
  - [go vet](https://golang.org/cmd/vet/)
  - [goimports](https://godoc.org/golang.org/x/tools/cmd/goimports)

## Generating gRPC code <a name="gRPC"></a>

If you modify any `.proto` files, run the following command to generate/update
the respective `.pb.go` files.

```
cd $GOPATH/src/github.com/hyperledger/fabric
make protos
```

## Adding or updating Go packages

The Hyperledger Fabric Project uses Go 1.6 vendoring for package management.
This means that all required packages reside in the `vendor` folder within the
fabric project. Go will use packages in this folder instead of the GOPATH when
the `go install` or `go build` commands are executed. To manage the packages in
the `vendor` folder, we use [Govendor](https://github.com/kardianos/govendor),
which is installed in the Vagrant environment. The following commands can be
used for package management:

```
  # Add external packages.
  govendor add +external

  # Add a specific package.
  govendor add github.com/kardianos/osext

  # Update vendor packages.
  govendor update +vendor

  # Revert back to normal GOPATH packages.
  govendor remove +vendor

  # List package.
  govendor list
```
