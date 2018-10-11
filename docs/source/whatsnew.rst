What's new in v1.3
==================

A quick rundown of the new features and documentation in the v1.3 release of
Hyperledger Fabric:

New features
------------

* :doc:`idemix`:
  A way to keep identities anonymous and unlinkable through the use of zero-knowledge
  proofs. There is a tool that can generate Identity Mixer credentials in test
  environments known as `idexmigen`, the documentation for which can be found in
  :doc:`idemixgen`.

* :ref:`key-level-endorsement`:
  Allows the default chaincode-level endorsement policy to be overridden by a
  per-key endorsement policy.

* :ref:`cdb-pagination`:
  Clients can now page through result sets from chaincode queries, making it
  feasible to support large result sets with high performance.

* :doc:`chaincode4ade`:
  As an addition to the current Fabric support for chaincode written in Go and
  node.js, Java is now supported. You can find a javadoc for this
  `here <https://fabric-chaincode-java.github.io/>`__.

* :doc:`peer_event_services`:
  The peer channel-based event service itself is not new (it first debuted in v1.1),
  but the v1.3 release marks the end of the old event hub. Applications using
  the old event hub must switch over to the new peer channel-based event service prior to
  upgrading to v1.3.

New tutorials
-------------

* :doc:`upgrade_to_newest_version`:
  Leverages the BYFN network to show how an upgrade flow should work. Includes
  both a script (which can serve as a template for upgrades), as well as the
  individual commands.

* :ref:`cdb-pagination`:
  Expands the current CouchDB tutorial to add pagination.

Other new documentation
-----------------------

* :doc:`network/network`:
  Conceptual documentation that shows how the parts of a network interact with
  each other. The initial version of this document was added in v1.2.

Release notes
=============

For more information, including `FAB` numbers for the issues and code reviews
that made up these changes (in addition to other hygiene/performance/bug fixes
we did not explicitly document), check out the release notes. Note that these
links will not work on the release candidate, only on the GA release.

* `Fabric release notes <https://github.com/hyperledger/fabric/releases/tag/v1.3.0>`_.
* `Fabric CA release notes <https://github.com/hyperledger/fabric-ca/releases/tag/v1.3.0>`_.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
