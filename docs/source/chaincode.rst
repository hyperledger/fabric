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
network, and who would leverage the Hyperledger Fabric API to install,
instantiate, and upgrade chaincode, but would likely not be involved in the
development of a chaincode application.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
