Ledger
======

The ledger is the sequenced, tamper-resistant record of all state transitions.  State
transitions are a result of chaincode invocations ('transactions') submitted by participating
parties.  Each transaction results in a set of asset key-value pairs that are committed to the
ledger as creates, updates, or deletes.

The ledger is comprised of a blockchain ('chain') to store the immutable, sequenced record in
blocks, as well as a state database to maintain current state.  There is one ledger per
channel. Each peer maintains a copy of the ledger for each channel of which they are a member.

Chain
-----

The chain is a transaction log, structured as hash-linked blocks, where each block contains a
sequence of N transactions. The block header includes a hash of the block's transactions, as
well as a hash of the prior block's header. In this way, all transactions on the ledger are
sequenced and cryptographically linked together. In other words, it is not possible to tamper with
the ledger data, without breaking the hash links. The hash of the latest block represents every
transaction that has come before, making it possible to ensure that all peers are in a consistent
and trusted state.

The chain is stored on the peer file system (either local or attached storage), efficiently
supporting the append-only nature of the blockchain workload.

State Database
--------------

The ledger's current state data represents the latest values for all keys ever included in the chain
transaction log. Since current state represents all latest key values known to the channel, it is
sometimes referred to as World State.

Chaincode invocations execute transactions against the current state data. To make these
chaincode interactions extremely efficient, the latest values of all keys are stored in a state
database. The state database is simply an indexed view into the chain's transaction log, it can
therefore be regenerated from the chain at any time.  The state database will automatically get
recovered (or generated if needed) upon peer startup, before transactions are accepted.

Transaction Flow
----------------

At a high level, the transaction flow consists of a transaction proposal sent by an application
client to specific endorsing peers.  The endorsing peers verify the client signature, and execute
a chaincode function to simulate the transaction.  The output is the chaincode results,
a set of key/value versions that were read in the chaincode (read set), and the set of keys/values
that were written in chaincode (write set).  The proposal response gets sent back to the client
along with an endorsement signature.

The client assembles the endorsements into a transaction payload and broadcasts it to an ordering
service.  The ordering service delivers ordered transactions as blocks to all peers on a channel.

Before committal, peers will validate the transactions.  First, they will check the endorsement
policy to ensure that the correct allotment of the specified peers have signed the results, and they
will authenticate the signatures against the transaction payload.

Secondly, peers will perform a versioning check against the transaction read set, to ensure
data integrity and protect against threats such as double-spending.
Hyperledger Fabric has concurrency control whereby transactions execute in parallel (by endorsers)
to increase throughput, and upon commit (by all peers) each transaction is verified to ensure
that no other transaction has modified data it has read. In other words, it ensures that the data
that was read during chaincode execution has not changed since execution (endorsement) time,
and therefore the execution results are still valid and can be committed to the ledger state
database. If the data that was read has been changed by another transaction, then the
transaction in the block is marked as invalid and is not applied to the ledger state database.
The client application is alerted, and can handle the error or retry as appropriate.

See the :doc:`txflow` and :doc:`readwrite` topics for a deeper dive on transaction structure,
concurrency control, and the state DB.

State Database options
----------------------

State database options include LevelDB and CouchDB. LevelDB is the default key/value state
database embedded in the peer process. CouchDB is an optional alternative external state database.
Like the LevelDB key/value store, CouchDB can store any binary data that is modeled in chaincode
(CouchDB attachment functionality is used internally for non-JSON binary data). But as a JSON
document store, CouchDB additionally enables rich query against the chaincode data, when chaincode
values (e.g. assets) are modeled as JSON data.

Both LevelDB and CouchDB support core chaincode operations such as getting and setting a key
(asset), and querying based on keys. Keys can be queried by range, and composite keys can be
modeled to enable equivalence queries against multiple parameters. For example a composite
key of (owner,asset_id) can be used to query all assets owned by a certain entity. These key-based
queries can be used for read-only queries against the ledger, as well as in transactions that
update the ledger.

If you model assets as JSON and use CouchDB, you can also perform complex rich queries against the
chaincode data values, using the CouchDB JSON query language within chaincode. These types of
queries are excellent for understanding what is on the ledger. Proposal responses for these types
of queries are typically useful to the client application, but are not typically submitted as
transactions to the ordering service. In fact, there is no guarantee the result set is stable
between chaincode execution and commit time for rich queries, and therefore rich queries
are not appropriate for use in update transactions, unless your application can guarantee the
result set is stable between chaincode execution time and commit time, or can handle potential
changes in subsequent transactions.  For example, if you perform a rich query for all assets
owned by Alice and transfer them to Bob, a new asset may be assigned to Alice by another
transaction between chaincode execution time and commit time, and you would miss this 'phantom'
item.

CouchDB runs as a separate database process alongside the peer, therefore there are additional
considerations in terms of setup, management, and operations. You may consider starting with the
default embedded LevelDB, and move to CouchDB if you require the additional complex rich queries.
It is a good practice to model chaincode asset data as JSON, so that you have the option to perform
complex rich queries if needed in the future.

CouchDB Configuration
----------------------

CouchDB is enabled as the state database by changing the stateDatabase configuration option from
goleveldb to CouchDB.   Additionally, the ``couchDBAddress`` needs to configured to point to the
CouchDB to be used by the peer.  The username and password properties should be populated with
an admin username and password if CouchDB is configured with a username and password.  Additional
options are provided in the ``couchDBConfig`` section and are documented in place.  Changes to the
*core.yaml* will be effective immediately after restarting the peer.

You can also pass in docker environment variables to override core.yaml values, for example
``CORE_LEDGER_STATE_STATEDATABASE`` and ``CORE_LEDGER_STATE_COUCHDBCONFIG_COUCHDBADDRESS``.

Below is the ``stateDatabase`` section from *core.yaml*:

.. code:: bash

    state:
      # stateDatabase - options are "goleveldb", "CouchDB"
      # goleveldb - default state database stored in goleveldb.
      # CouchDB - store state database in CouchDB
      stateDatabase: goleveldb
      couchDBConfig:
         # It is recommended to run CouchDB on the same server as the peer, and
         # not map the CouchDB container port to a server port in docker-compose.
         # Otherwise proper security must be provided on the connection between
         # CouchDB client (on the peer) and server.
         couchDBAddress: couchdb:5984
         # This username must have read and write authority on CouchDB
         username:
         # The password is recommended to pass as an environment variable
         # during start up (e.g. LEDGER_COUCHDBCONFIG_PASSWORD).
         # If it is stored here, the file must be access control protected
         # to prevent unintended users from discovering the password.
         password:
         # Number of retries for CouchDB errors
         maxRetries: 3
         # Number of retries for CouchDB errors during peer startup
         maxRetriesOnStartup: 10
         # CouchDB request timeout (unit: duration, e.g. 20s)
         requestTimeout: 35s
         # Limit on the number of records to return per query
         queryLimit: 10000


CouchDB hosted in docker containers supplied with Hyperledger Fabric have the
capability of setting the CouchDB username and password with environment
variables passed in with the ``COUCHDB_USER`` and ``COUCHDB_PASSWORD`` environment
variables using Docker Compose scripting.

For CouchDB installations outside of the docker images supplied with Fabric, the
*local.ini* file must be edited to set the admin username and password.

Docker compose scripts only set the username and password at the creation of
the container.  The *local.ini* file must be edited if the username or password
is to be changed after creation of the container.

.. note:: CouchDB peer options are read on each peer startup.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
