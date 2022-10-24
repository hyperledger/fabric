## Contributing

We welcome contributions to the Hyperledger Fabric Project in many forms, and
there's always plenty to do!

Please visit the
[contributors guide](http://hyperledger-fabric.readthedocs.io/en/latest/CONTRIBUTING.html) in the
docs to learn how to make contributions to this exciting project.

## Running Unit tests

An example of using the script as used in the CI pipeline to run Unit Tests 

```
TEST_PKGS=github.com/hyperledger/fabric/core/chaincode/... ./scripts/run-unit-tests.sh
```

## Creating the mocks for unit tests

A number of mock implementations of interfaces are used within the unit tests. For historical reasons there are two tools
with the repo to generate these mocks. [mockery](https://github.com/vektra/mockery) and [counterfeiter](https://github.com/maxbrunsfeld/counterfeiter)

- look in the already created mock - the first line will indicate which tool it was created with
- for counterfieter, run `go generate ./<dir>/...` in the <dir> where you want the mocks directory to be created

- for mockery the command is `mockery --name ApplicationCapabilities --dir ~/github.com/hyperledger/fabric/common/channelconfig --filename application_capabilities.go`
    - this will create the mock with the filename given in the mocks directory based of the cwd
    - the `--name` and `--dir` indicate where the 'source' interface is to be mocked

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
s
