Getting Started - Install
=========================

.. toctree::
   :maxdepth: 1
   :hidden:

   prereqs
   install
   sdk_chaincode


The Fabric application stack has five layers:

.. image:: ./getting_started_image2.png
   :width: 300px
   :align: right
   :height: 200px
   :alt: Fabric Application Stack

* :doc:`Prerequisite software <prereqs>`: the base layer needed to run the software, for example, Docker.
* :doc:`Fabric and Fabric samples <install>`: the Fabric executables to run a Fabric network along with sample code.
* :doc:`Contract APIs <sdk_chaincode>`: to develop smart contracts executed on a Fabric Network.
* :doc:`Application APIs <sdk_chaincode>`: to develop your blockchain application.
* The Application: your blockchain application will utilize the Application SDKs to call smart contracts running on a Fabric network.

For a quick local setup, start with :doc:`Fabric and Fabric samples <install>`, which includes the
`install-fabric.sh` script for downloading Fabric binaries, container images, and the samples repository.
If you want to modify Fabric itself, continue afterwards with the
:doc:`development environment <dev-setup/devenv>`.


.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
