# Ledger

[WIP]
...coming soon

The ledger exists as a peer process utilizing levelDB.  It supports the high level
transaction flow - read-write-set simulation, endorsement, MVCC check, file-based
blockchain transaction log, and state database.

v1 architecture has been designed to support various ledger implementations such
as couchDB, where more complexity with rich queries, pruning, archiving, etc...
becomes possible.

For more information on the current state of ledger development, explore the corresponding
JIRA issue - https://jira.hyperledger.org/browse/FAB-758
