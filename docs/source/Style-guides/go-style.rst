Coding guidelines
-----------------

Coding Golang
~~~~~~~~~~~~~~

We code in Go™ and strictly follow the `best
practices <http://golang.org/doc/effective_go.html>`__ and will not
accept any deviations. You must run the following tools against your Go
code and fix all errors and warnings: -
`golint <https://github.com/golang/lint>`__ - `go
vet <https://golang.org/cmd/vet/>`__ -
`goimports <https://godoc.org/golang.org/x/tools/cmd/goimports>`__

Generating gRPC code
---------------------

If you modify any ``.proto`` files, run the following command to
generate/update the respective ``.pb.go`` files.

::

    cd $GOPATH/src/github.com/hyperledger/fabric
    make protos

Adding or updating Go packages
------------------------------

Hyperledger Fabric uses Govendor for package
management. This means that all required packages reside in the
``$GOPATH/src/github.com/hyperledger/fabric/vendor`` folder. Go will use
packages in this folder instead of the GOPATH when the ``go install`` or
``go build`` commands are executed. To manage the packages in the
``vendor`` folder, we use
`Govendor <https://github.com/kardianos/govendor>`__, which is installed
in the Vagrant environment. The following commands can be used for
package management:

::

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

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/

