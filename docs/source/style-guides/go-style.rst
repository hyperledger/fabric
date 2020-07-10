Coding guidelines
-----------------

Coding in Go
~~~~~~~~~~~~

We code in Go™ and try to follow the best practices and style outlined in
`Effective Go <https://golang.org/doc/effective_go.html>`__ and the
supplemental rules from the `Go Code Review Comments wiki
<https://github.com/golang/go/wiki/CodeReviewComments>`__.

We also recommend new contributors review the following before submitting
pull requests:

  - `Practical Go <https://dave.cheney.net/practical-go/presentations/qcon-china.html>`__
  - `Go Proverbs <https://go-proverbs.github.io/>`__

The following tools are executed against all pull requests. Any errors flagged
by these tools must be addressed before the code will be merged:

  - `gofmt -s <https://golang.org/cmd/gofmt/>`__
  - `goimports <https://godoc.org/golang.org/x/tools/cmd/goimports>`__
  - `go vet <https://golang.org/cmd/vet/>`__

Testing
^^^^^^^

Unit tests are expected to accompany all production code changes. These tests
should be fast, provide very good coverage for new and modified code, and
support parallel execution.

Two matching libraries are commonly used in our tests. When modifying code,
please use the matching library that has already been chosen for the package.

  - `gomega <https://onsi.github.io/gomega/>`__
  - `testify/require <https://godoc.org/github.com/stretchr/testify/require>`__

Any fixtures or data required by tests should generated or placed under version
control. When fixtures are generated, they must be placed in a temporary
directory created by ``ioutil.TempDir`` and cleaned up when the test
terminates. When fixtures are placed under version control, they should be
created inside a ``testdata`` folder; documentation that describes how to
regenerate the fixtures should be provided in the tests or a ``README.txt``.
Sharing fixtures across packages is strongly discouraged.

When fakes or mocks are needed, they must be generated. Bespoke, hand-coded
mocks are a maintenance burden and tend to include simulations that inevitably
diverge from reality. Within Fabric, we use ``go generate`` directives to
manage the generation with the following tools:

  - `counterfeiter <https://github.com/maxbrunsfeld/counterfeiter>`__
  - `mockery <https://github.com/vektra/mockery>`__

API Documentation
^^^^^^^^^^^^^^^^^

The API documentation for Hyperledger Fabric's Go APIs is available
in `GoDoc <https://godoc.org/github.com/hyperledger/fabric>`_.

Adding or updating Go packages
------------------------------

Hyperledger Fabric uses go modules to manage and vendor its dependencies. This
means that all of the external packages required to build our binaries reside
in the ``vendor`` folder at the top of the repository. Go uses the packages in
this folder instead of the module cache when ``go`` commands are executed.

If a code change results in a new or updated dependency, please be sure to run
``go mod tidy`` and ``go mod vendor`` to keep the ``vendor`` folder and
dependency metadata up to date.

See the `Go Modules Wiki <https://github.com/golang/go/wiki/Modules>`__ for
additional information.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
