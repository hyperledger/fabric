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

See `IT Daily Results <https://jenkins.hyperledger.org/view/Daily/>`__

See `IT Weekly Results <https://jenkins.hyperledger.org/view/Weekly/>`__

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

See `SVT Daily Results <https://jenkins.hyperledger.org/view/fabric-test/job/fabric-test-daily-results-x86_64/test_results_analyzer/>`__

See `SVT Weekly Results <https://jenkins.hyperledger.org/view/fabric-test/job/fabric-test-weekly-results-x86_64/test_results_analyzer/>`__

See `SVT Performance Test Results Graphs With Testviewer <https://github.com/hyperledger/fabric-test/blob/master/tools/Testviewer/README.md>`__


.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
