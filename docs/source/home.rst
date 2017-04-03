Hyperledger Fabric
==================

The Hyperledger fabric is an implementation of blockchain technology,
that has been collaboratively developed under the Linux Foundation's
`Hyperledger Project <http://hyperledger.org>`__. It leverages familiar
and proven technologies, and offers a modular architecture that allows
pluggable implementations of various functions including membership
services, consensus, and smart contracts (chaincode) execution. It
features powerful container technology to host any mainstream language
for smart contracts development.

|Build Status| |Go Report Card| |GoDoc| |Documentation Status|

Status
------

This project is an archived Hyperledger project. It was proposed
to the community and documented `here <https://goo.gl/RYQZ5N>`__.
Information on this project's history can be found in the
`Hyperledger Project Lifecycle document <https://goo.gl/4edNRc>`__.

Releases
--------

The fabric releases are documented :doc:`here <releases>`. We have just
released our second release under the governance of the Hyperledger
Project - v0.6-preview.

Fabric Starter Kit
------------------

If you'd like to dive right in and get an operational experience on your
local server or laptop to begin development, we have just the thing for
you. We have created a standalone Docker-based :doc:`starter
kit <starter/fabric-starter-kit>` that leverages the latest
published Docker images that you can run on your laptop and be up and
running in no time. That should get you going with a sample application
and some simple chaincode. From there, you can go deeper by exploring
our :ref:`developer-guides`.

Contributing to the project
---------------------------

We welcome contributions to the Hyperledger Project in many forms.
There's always plenty to do! Full details of how to contribute to this
project are documented in the :ref:`fabric-developer-guide` below.

Maintainers
-----------

The project's :doc:`maintainers <MAINTAINERS>` are responsible for
reviewing and merging all patches submitted for review and they guide
the over-all technical direction of the project within the guidelines
established by the Hyperledger Project's Technical Steering Committee
(TSC).

Communication
--------------

We use `Rocket Chat <https://slack.hyperledger.org/>`__ for
communication and Google Hangouts™ for screen sharing between
developers. Our development planning and prioritization is done in
`JIRA <https://jira.hyperledger.org>`__, and we take longer running
discussions/decisions to the `mailing
list <http://lists.hyperledger.org/mailman/listinfo/hyperledger-fabric>`__.

Still Have Questions?
---------------------

We try to maintain a comprehensive set of documentation (see below) for
various audiences. However, we realize that often there are questions
that remain unanswered. For any technical questions relating to the
Hyperledger Fabric project not answered in this documentation, please
use
`StackOverflow <http://stackoverflow.com/questions/tagged/hyperledger>`__.
If you need help finding things, please don't hesitate to send a note to
the `mailing
list <http://lists.hyperledger.org/mailman/listinfo/hyperledger-fabric>`__,
or ask on `Rocket Chat <(https://slack.hyperledger.org/)>`__.

Table of Contents
-----------------

Below, you'll find the following sections:

-  :ref:`read-all-about-it`
-  :ref:`developer-guides`
-  :ref:`chaincode-developer-guide`
-  :ref:`application-developer-guide`
-  :ref:`fabric-developer-guide`
-  :ref:`operations-guide`

.. _read-all-about-it:

Read all about it
-----------------

If you are new to the project, you can begin by reviewing the following
links. If you'd prefer to dive right in, see the
:doc:`Quickstart <#quickstart-documentation>` section, below.

-  `Whitepaper
   WG <https://github.com/hyperledger/hyperledger/wiki/Whitepaper-WG>`__:
   where the community is developing a whitepaper to explain the
   motivation and goals for the project.
-  `Requirements
   WG <https://github.com/hyperledger/hyperledger/wiki/Requirements-WG>`__:
   where the community is developing use cases and requirements.
-  :doc:`Canonical use cases <biz/usecases>`
-  :doc:`Glossary <glossary>`: to understand the terminology that we use
   throughout the Fabric project's documentation.
-  `Fabric
   FAQs <https://github.com/hyperledger/fabric/tree/master/docs/FAQ>`__

.. _developer-guides:

Developer guides
================

.. _chaincode-developer-guide:

Chaincode developer guide
-------------------------

-  :doc:`Setting up the development environment <dev-setup/devenv>`
   when developing and testing Chaincode, or an application that
   leverages the fabric API or SDK, you'll probably want to run the
   fabric locally on your laptop to test. You can use the same setup
   that Fabric developers use.
-  :doc:`Setting Up a Network For Development <Setup/Network-setup>`
   alternately, you can follow these instructions for setting up a local
   network for Chaincode development without the entire fabric
   development environment setup.
-  :doc:`Writing, Building, and Running Chaincode in a Development
   Environment <Setup/Chaincode-setup>` a step-by-step guide to
   writing and testing Chaincode.
-  :doc:`Chaincode FAQ <FAQ/chaincode_FAQ>` a FAQ for all of your
   burning questions relating to Chaincode.

.. _application-developer-guide:

Application developer guide
---------------------------

-  :doc:`APIs - CLI, REST, and Node.js <API/CoreAPI>`
-  :doc:`CLI <API/CoreAPI>` - working with the command-line
   interface.
-  :doc:`REST <API/CoreAPI>` - working with the REST API
   (*deprecated*).
-  :doc:`Node.js SDK <nodeSDK/node-sdk-guide>` - working with the Node.js
   SDK.

.. _fabric-developer-guide:

Fabric developer guide
----------------------

-  First, you'll want to familiarize yourself with the project's
   :doc:`code contribution guidelines <CONTRIBUTING>`
-  After that, you will want to
   :doc:`set up the development environment <dev-setup/devenv>`
-  try :doc:`building the fabric core <dev-setup/build>` in your local
   development environment to ensure that everything is set up correctly.
-  For the *adventurous*, you might try :doc:`building outside of the
   standard Vagrant development environment <dev-setup/build>`
-  :doc:`Logging control <Setup/logging-control>` describes how to
   tweak the logging levels of various components within the fabric.
-  Every source file must include this
   :doc:`license header <dev-setup/headers>` modified to include a copyright
   statement for the principle author(s).


.. _operations-guide:

Operations guide
================

-  :doc:`Setting Up a Network <Setup/Network-setup>` instructions for
   setting up a network of fabric peers.
-  :doc:`Certificate Authority (CA) Setup <Setup/ca-setup>` setting up
   a CA to support identity, security (authentication/authorization),
   privacy and confidentiality.
-  :doc:`Application ACL <tech/application-ACL>` working with access
   control lists.

License
========

The Hyperledger Project uses the :doc:`Apache License Version
2.0 <LICENSE>` software license.

.. |Build Status| image:: https://jenkins.hyperledger.org/buildStatus/icon?job=fabric-merge-x86_64
   :target: https://jenkins.hyperledger.org/view/fabric/job/fabric-merge-x86_64/
.. |Go Report Card| image:: https://goreportcard.com/badge/github.com/hyperledger/fabric
   :target: https://goreportcard.com/report/github.com/hyperledger/fabric
.. |GoDoc| image:: https://godoc.org/github.com/hyperledger/fabric?status.svg
   :target: https://godoc.org/github.com/hyperledger/fabric
.. |Documentation Status| image:: https://readthedocs.org/projects/hyperledger-fabric/badge/?version=latest
   :target: http://hyperledger-fabric.readthedocs.io/en/latest/?badge=latest
