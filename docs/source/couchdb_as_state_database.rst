CouchDB as the State Database
=============================

State Database options
----------------------

The current options for the peer state database are LevelDB and CouchDB. LevelDB is the default
key-value state database embedded in the peer process. CouchDB is an alternative external state database.
Like the LevelDB key-value store, CouchDB can store any binary data that is modeled in chaincode
(CouchDB attachments are used internally for non-JSON data). As a document object store,
CouchDB allows you to store data in JSON format, issue JSON queries against your data,
and use indexes to support your queries.

Both LevelDB and CouchDB support core chaincode operations such as getting and setting a key
(asset), and querying based on keys. Keys can be queried by range, and composite keys can be
modeled to enable equivalence queries against multiple parameters. For example a composite
key of ``owner,asset_id`` can be used to query all assets owned by a certain entity. These key-based
queries can be used for read-only queries against the ledger, as well as in transactions that
update the ledger.

Modeling your data in JSON allows you to issue JSON queries against the values of your data,
instead of only being able to query the keys. This makes it easier for your applications and
chaincode to read the data stored on the blockchain ledger. Using CouchDB can help you meet
auditing and reporting requirements for many use cases that are not supported by LevelDB. If you use
CouchDB and model your data in JSON, you can also deploy indexes with your chaincode.
Using indexes makes queries more flexible and efficient and enables you to query large
datasets from chaincode.

CouchDB runs as a separate database process alongside the peer, therefore there are additional
considerations in terms of setup, management, and operations. It is a good practice to model
asset data as JSON, so that you have the option to perform complex JSON queries if needed in the future.

.. note:: The key for a CouchDB JSON document can only contain valid UTF-8 strings and cannot begin
   with an underscore ("_"). Whether you are using CouchDB or LevelDB, you should avoid using
   U+0000 (nil byte) in keys.

   JSON documents in CouchDB cannot use the following values as top level field names. These values
   are reserved for internal use.

   - ``Any field beginning with an underscore, "_"``
   - ``~version``

   Because of these data incompatibilities between LevelDB and CouchDB, the database choice
   must be finalized prior to deploying a production peer. The database cannot be converted at a
   later time.

Using CouchDB from Chaincode
----------------------------

Reading and writing JSON data
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

When writing JSON data values to CouchDB (e.g. using ``PutState``) and reading
JSON back in later chaincode requests (e.g. using ``GetState``), the format of the JSON and
the order of the JSON fields are not guaranteed, based on the JSON specification. Your chaincode
should therefore unmarshall the JSON before working with the data. Similarly, when marshaling
JSON, utilize a library that guarantees deterministic results, so that proposed chaincode writes
and responses to clients will be identical across endorsing peers (note that Go ``json.Marshal()``
does in fact sort keys deterministically, but in other languages you may need to utilize a canonical
JSON library).

Chaincode queries
~~~~~~~~~~~~~~~~~

Most of the `chaincode shim APIs <https://godoc.org/github.com/hyperledger/fabric-chaincode-go/shim#ChaincodeStubInterface>`__
can be utilized with either LevelDB or CouchDB state database, e.g. ``GetState``, ``PutState``,
``GetStateByRange``, ``GetStateByPartialCompositeKey``. Additionally when you utilize CouchDB as
the state database and model assets as JSON in chaincode, you can perform JSON queries against
the data in the state database by using the ``GetQueryResult`` API and passing a CouchDB query string.
The query string follows the `CouchDB JSON query syntax <http://docs.couchdb.org/en/3.2.2/api/database/find.html>`__.

The `asset transfer Fabric sample <https://github.com/hyperledger/fabric-samples/blob/main/asset-transfer-ledger-queries/chaincode-go/asset_transfer_ledger_chaincode.go>`__
demonstrates use of CouchDB queries from chaincode. It includes a ``queryAssetsByOwner()`` function
that demonstrates parameterized queries by passing an owner id into chaincode. It then queries the
state data for JSON documents matching the docType of "asset" and the owner id using the JSON query
syntax:

.. code:: bash

  {"selector":{"docType":"asset","owner":<OWNER_ID>}}

The responses to JSON queries are useful for understanding the data on the ledger. However,
there is no guarantee that the result set for a JSON query will be stable between
the chaincode execution and commit time. As a result, you should not use a JSON query and
update the channel ledger in a single transaction. For example, if you perform a
JSON query for all assets owned by Alice and transfer them to Bob, a new asset may
be assigned to Alice by another transaction between chaincode execution time
and commit time.


.. couchdb-pagination:

CouchDB pagination
^^^^^^^^^^^^^^^^^^

Fabric supports paging of query results for JSON queries and key range based queries.
APIs supporting pagination allow the use of page size and bookmarks to be used for
both key range and JSON queries. To support efficient pagination, the Fabric
pagination APIs must be used. Specifically, the CouchDB ``limit`` keyword will
not be honored in CouchDB queries since Fabric itself manages the pagination of
query results and implicitly sets the pageSize limit that is passed to CouchDB.

If a pageSize is specified using the paginated query APIs (``GetStateByRangeWithPagination()``,
``GetStateByPartialCompositeKeyWithPagination()``, and ``GetQueryResultWithPagination()``),
a set of results (bound by the pageSize) will be returned to the chaincode along with
a bookmark. The bookmark can be returned from chaincode to invoking clients,
which can use the bookmark in a follow on query to receive the next "page" of results.

The pagination APIs are for use in read-only transactions only, the query results
are intended to support client paging requirements. For transactions
that need to read and write, use the non-paginated chaincode query APIs. Within
chaincode you can iterate through result sets to your desired depth.

Regardless of whether the pagination APIs are utilized, all chaincode queries are
bound by ``totalQueryLimit`` (default 100000) from ``core.yaml``. This is the maximum
number of results that chaincode will iterate through and return to the client,
in order to avoid accidental or malicious long-running queries.

.. note:: Regardless of whether chaincode uses paginated queries or not, the peer will
          query CouchDB in batches based on ``internalQueryLimit`` (default 1000)
          from ``core.yaml``. This behavior ensures reasonably sized result sets are
          passed between the peer and CouchDB when executing chaincode, and is
          transparent to chaincode and the calling client.

An example using pagination is included in the :doc:`couchdb_tutorial` tutorial.

CouchDB indexes
~~~~~~~~~~~~~~~

Indexes in CouchDB are required in order to make JSON queries efficient and are required for
any JSON query with a sort. Indexes enable you to query data from chaincode when you have
a large amount of data on your ledger. Indexes can be packaged alongside chaincode
in a ``/META-INF/statedb/couchdb/indexes`` directory. Each index must be defined in
its own text file with extension ``*.json`` with the index definition formatted in JSON
following the `CouchDB index JSON syntax <http://docs.couchdb.org/en/3.2.2/api/database/find.html#db-index>`__.
For example, to support the above marble query, a sample index on the ``docType`` and ``owner``
fields is provided:

.. code:: bash

  {"index":{"fields":["docType","owner"]},"ddoc":"indexOwnerDoc", "name":"indexOwner","type":"json"}

The sample index can be found `here <https://github.com/hyperledger/fabric-samples/blob/main/asset-transfer-ledger-queries/chaincode-go/META-INF/statedb/couchdb/indexes/indexOwner.json>`__.

Any index in the chaincode’s ``META-INF/statedb/couchdb/indexes`` directory
will be packaged up with the chaincode for deployment. The index will be deployed
to a peers channel and chaincode specific database when the chaincode package is
installed on the peer and the chaincode definition is committed to the channel. If you
install the chaincode first and then commit the chaincode definition to the
channel, the index will be deployed at commit time. If the chaincode has already
been defined on the channel and the chaincode package subsequently installed on
a peer joined to the channel, the index will be deployed at chaincode
**installation** time.

Upon deployment, the index will automatically be utilized by chaincode queries. CouchDB can automatically
determine which index to use based on the fields being used in a query. Alternatively, in the
selector query the index can be specified using the ``use_index`` keyword.

The same index may exist in subsequent versions of the chaincode that gets installed. To change the
index, use the same index name but alter the index definition. Upon installation/instantiation, the index
definition will get re-deployed to the peer’s state database.

If you have a large volume of data already, and later install the chaincode, the index creation upon
installation may take some time. Similarly, if you have a large volume of data already and commit the
definition of a subsequent chaincode version, the index creation may take some time. Avoid calling chaincode
functions that query the state database at these times as the chaincode query may time out while the
index is getting initialized. During transaction processing, the indexes will automatically get refreshed
as blocks are committed to the ledger. If the peer crashes during chaincode installation, the couchdb
indexes may not get created. If this occurs, you need to reinstall the chaincode to create the indexes.

CouchDB Configuration
---------------------

Peer configuration for CouchDB
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

CouchDB is enabled as the state database by changing the ``stateDatabase`` configuration option from
goleveldb to CouchDB. Additionally, the ``couchDBAddress`` needs to configured to point to the
CouchDB to be used by the peer. The username and password properties should be populated with
an admin username and password. Additional
options are provided in the ``couchDBConfig`` section and are documented in place. Changes to the
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
      # Limit on the number of records to return per query
      totalQueryLimit: 10000
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
         # Limit on the number of records per each CouchDB query
         # Note that chaincode queries are only bound by totalQueryLimit.
         # Internally the chaincode may execute multiple CouchDB queries,
         # each of size internalQueryLimit.
         internalQueryLimit: 1000
         # Limit on the number of records per CouchDB bulk update batch
         maxBatchUpdateSize: 1000

.. note:: CouchDB peer options are read on each peer startup.

CouchDB container configuration
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Although full CouchDB configuration is beyond the scope of this document, some common considerations are provided in this section.

If using Docker compose, the environment variables ``COUCHDB_USER`` and ``COUCHDB_PASSWORD``
can be used to set the CouchDB user and password when the CouchDB container gets created.

Alternatively, set the CouchDB user and password in the CouchDB
`local.ini configuration file <http://docs.couchdb.org/en/3.2.2/config/intro.html#configuration-files>`__.

To change the CouchDB user and password after CouchDB container creation, the ``local.ini`` configuration approach must be used.

As of CouchDB 3.0.0 ``max_document_size`` defaults to ``8000000`` (8MB).
``max_document_size`` can be set as high as ``4294967296`` (4GB) if you need to store large JSON documents to the state database.
For more details see the CouchDB `max_document_size documentation <https://docs.couchdb.org/en/3.2.2-docs/config/couchdb.html#couchdb/max_document_size>`__.

In a production environment the CouchDB container port should only be exposed to the relevant peer.
In a development environment you may expose the CouchDB container port to allow for REST API interaction or CouchDB Fauxton user interface interaction,
for example to ensure that you have correct indexes defined for a given query.

Good practices for queries
--------------------------

Avoid using chaincode for queries that will result in a scan of the entire
CouchDB database. Full length database scans will result in long response
times and will degrade the performance of your network. You can take some of
the following steps to avoid long queries:

- When using JSON queries:

    * Be sure to create indexes in the chaincode package.
    * Avoid query operators such as ``$or``, ``$in`` and ``$regex``, which lead
      to full database scans.

- For range queries, composite key queries, and JSON queries:

    * Utilize paging support instead of one large result set.

- If you want to build a dashboard or collect aggregate data as part of your
  application, you can query an off-chain database that replicates the data
  from your blockchain network. This will allow you to query and analyze the
  blockchain data in a data store optimized for your needs, without degrading
  the performance of your network or disrupting transactions. To achieve this,
  applications may use block or chaincode events to write transaction data
  to an off-chain database or analytics engine. For each block received, the block
  listener application would iterate through the block transactions and build a
  data store using the key/value writes from each valid transaction's ``rwset``.
  The :doc:`peer_event_services` provide replayable events to ensure the
  integrity of downstream data stores.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
