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


System test
~~~~~~~~~~~

[WIP] ...coming soon

This topic is intended to contain recommended test scenarios, as well as
current performance numbers against a variety of configurations.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/

