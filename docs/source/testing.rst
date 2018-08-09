Testing
=======

Unit test
~~~~~~~~~
See :doc:`building Hyperledger Fabric <dev-setup/build>` for unit testing instructions.

See `Unit test coverage reports <https://jenkins.hyperledger.org/view/fabric/job/fabric-merge-x86_64/>`__

To see coverage for a package and all sub-packages, execute the test with the ``-cover`` switch:

::

    go test ./... -cover

To see exactly which lines are not covered for a package, generate an html report with source
code annotated by coverage:

::

    go test -coverprofile=coverage.out
    go tool cover -html=coverage.out -o coverage.html


Integration test
~~~~~~~~~~~~~~~~
See `Hyperledger Fabric Integration tests <https://github.com/hyperledger/fabric/blob/master/integration/README.rst>`__
for Hyperledger Fabric integration test details.

General integration tests cover testing between components within a repository, e.g., tests between
peers and an orderer, and are used to show that a feature is working as expected across different interfaces.
A person should feel free to mock components as necessary.

Interoperability integration tests are integration tests that are across repositories, e.g., tests
between fabric and fabric-sdk-node.

Integration tests consist of more than 1 component. These tests would be used to show that
a feature is working as expected across different interfaces.

For more details on how to execute the integration tests, see the integration README.


System test
~~~~~~~~~~~
See `System Testing Hyperledger Fabric <https://github.com/hyperledger/fabric-test/blob/master/README.md>`__
for details on system testing.

System testing includes the following categories:
* Scalability

* Performance and Concurrency

* Upgrades and Fallbacks

* Long Running

* Compatibility

* Chaos and Recovery

* Functional (only for regression purposes)

See `Performance test results <http://169.46.120.39:31000/#/>`__

See `Daily test results <https://jenkins.hyperledger.org/view/Daily/>`__

See `Weekly test results <https://jenkins.hyperledger.org/view/Weekly/>`__


.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
