# ledgerutil

The `ledgerutil compare` command allows administrators to compare channel snapshots from two different peers.
Although channel snapshots from the same height should be identical across peers, if the snapshot
files indicate different state hashes (as seen in `_snapshot_signable_metadata.json` file from each snapshot)
then it indicates that at least one of the peers state databases is not correct relative to the blockchain and
may have gotten corrupted.

The `ledgerutil compare` utility will output a set of JSON files if the snapshots are not identical to assist with troubleshooting in these situations.
Two output JSON files will include any key/value differences sorted by key (one for public key/value differences and one for private key/value differences),
another JSON file will include any key/value differences (public or private) sorted by block and transaction height so that you can identify the height where a divergence may have first occurred.

The output files may help an administrator to understand the scope of a state database issue and identify which keys are impacted.
Snapshots from additional peers can be compared to determine which peer has incorrect state.
The block and transaction height of the first difference can be used as a reference point when looking at the peer's logs to understand what may have happened to the state database at that time.

## Syntax

The `ledgerutil` command has one subcommand

  * compare
