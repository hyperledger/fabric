## Ledger Package

This package implements the ledger, which includes the blockchain and global state.

If you're looking for API to work with the blockchain or state, look in `ledger.go`. This is the file where all public functions are exposed and is extensively documented. The sections in the file are:

### Transaction-batch functions

These are functions that consensus should call. `BeginTxBatch` followed by `CommitTxBatch` or `RollbackTxBatch`. These functions will add a block to the blockchain with the specified transactions.

### World-state functions

These functions are used to modify the global state. They would generally be called by the VM based on requests from chaincode.

### Blockchain functions

These functions can be used to retrieve blocks/transactions from the blockchain or other information such as the blockchain size. Addition of blocks to the blockchain is done though the transaction-batch related functions.
