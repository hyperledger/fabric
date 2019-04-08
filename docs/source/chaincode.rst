Chaincode Tutorials
===================

What is Chaincode?
------------------

Chaincode is a program, written in `Go <https://golang.org>`_, `node.js <https://nodejs.org>`_,
or `Java <https://java.com/en/>`_ that implements a prescribed interface.
Chaincode runs in a secured Docker container isolated from the endorsing peer
process. Chaincode initializes and manages ledger state through transactions
submitted by applications.

A chaincode typically handles business logic agreed to by members of the
network, so it may be considered as a "smart contract". State created by a
chaincode is scoped exclusively to that chaincode and can't be accessed
directly by another chaincode. However, within the same network, given
the appropriate permission a chaincode may invoke another chaincode to
access its state.

Two Personas
------------

We offer two different perspectives on chaincode. One, from the perspective of
an application developer developing a blockchain application/solution
entitled :doc:`chaincode4ade`, and the other, :doc:`chaincode4noah` oriented
to the blockchain network operator who is responsible for managing a blockchain
network, and who would leverage the Hyperledger Fabric API to install and govern
chaincode, but would likely not be involved in the development of a chaincode
application.

Fabric Chaincode Lifecycle
--------------------------

The new Fabric chaincode lifecycle is being introduced as part of the v2.0 Alpha.
The Fabric chaincode lifecycle is a process that allows multiple organizations
to agree on the parameters of a chaincode before it can be used to transact on a
channel. The new Fabric lifecycle offers several advantages over the old model.

* **Multiple organizations must agree to a chaincode endorsement policy:** In
  the release 1.x versions of Fabric, one organization had the ability to set a
  chaincode endorsement policy for all other channel members. The new Fabric
  chaincode lifecycle requires a sufficient number of organizations to agree on
  an endorsement policy before it goes into effect. In summary, the chaincode is
  now managed in the scope a of channel.

* **Safer chaincode upgrade process:** The original chaincode lifecycle required
  extensive coordination between organizations to upgrade a chaincode. The
  upgrade transaction was issued by one organization, creating a risk for an
  organization that had not yet installed the new chaincode. The new model
  allows for a chaincode to be upgraded only after a sufficient number of
  organizations approved the upgrade.

* **Easier endorsement policy updates:** Fabric lifecycle allows you to change
  an endorsement policy without having to repackage or reinstall the chaincode.

* **Inspectable chaincode packages:** The Fabric lifecycle packages chaincode in
  easily readable tar files. This makes it easier to inspect the chaincode
  package and coordinate installation across multiple organizations.

* **Start multiple chaincodes on a channel using one package:** The previous
  lifecycle defined each chaincode on the channel using a name and version that
  was specified when the chaincode package was installed. You can now use one
  chaincode package to deploy multiple chaincodes on a channel with different
  names.

To learn how more about how to use the new Fabric Lifecycle, visit
:doc:`chaincode4noah`

.. note:: The new Fabric chaincode lifecycle in the v2.0 Alpha release is not
          yet feature complete. Specifically, be aware of the following
          limitations in the Alpha release:

          - CouchDB indexes are not yet supported
          - Chaincodes defined with the new lifecycle are not yet discoverable
            via service discovery

          These limitations will be resolved after the Alpha release. To use the
          old lifecycle model to install and instantiate a chaincode, visit the
          v1.4 version of the `Chaincode for Operators tutorial <https://hyperledger-fabric.readthedocs.io/en/release-1.4/chaincode4noah.html>`_

You can use the Fabric chaincode lifecycle by creating a new channel and setting
the channel capabilities to V2_0. You will not be able to use the old lifecycle
to install, instantiate, or update a chaincode on a channels with V2_0 capabilities
enabled. However, you can still invoke chaincode installed using the previous
lifecycle model after you enable V2_0 capabilities. Migration from the previous
lifecycle to the new lifecycle is not supported for the Fabric v2.0 Alpha.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
