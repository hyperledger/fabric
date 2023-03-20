# ledgerutil

The `ledgerutil` or ledger utility suite is a set of tools useful for troubleshooting instances on a Fabric network where the ledgers of peers on a channel have diverged. The tools can be used individually or sequentially in a variety of divergence scenarios with the goal of helping the user locate the initial cause of divergence.

## Syntax

The `ledgerutil` command has three subcommands

  * `compare`
  * `identifytxs`
  * `verify`

## compare

The `ledgerutil compare` command allows administrators to compare channel snapshots from two different peers. Although channel snapshots from the same height should be identical across peers, if the snapshot files indicate different state hashes (as seen in `_snapshot_signable_metadata.json` file from each snapshot) then it indicates that at least one of the peer's state databases is not correct relative to the blockchain and may have gotten corrupted.

The `ledgerutil compare` command will output a directory containing a set of JSON files if the snapshots are not identical to assist with troubleshooting in these situations. Two output JSON files will include any key/value differences sorted by key, one for public key/value differences and one for private key/value differences. A third JSON file will include any key/value differences (public or private) sorted by block and transaction height so that the user can identify the height where a divergence may have first occurred. The ordered key/value difference JSON file will contain the earliest n differences, where n is a user chosen number passed as a flag to the `ledgerutil compare` command or a default value of 10 when no flag is passed.

The output files may help an administrator to understand the scope of a state database issue and identify which keys are impacted. Snapshots from additional peers can be compared to determine which peer has the incorrect state. The block and transaction height of the first difference can be used as a reference point when looking at the peer's logs to understand what may have happened to the state database at that time. Below is an example of an ordered difference output JSON file:

```
{
  "ledgerid":"mychannel",
  "diffRecords":[
    {
      "namespace":"_lifecycle$$h_implicit_org_Org1MSP",
      "key":"f0e4c2f76c58916ec258f246851bea091d14d4247a2fc3e18694461b1816e13b",
      "hashed":true,
      "snapshotrecord1":null,
      "snapshotrecord2":{
        "value":"0c52a3dbc7b322ff35728afdd691244cfc0fc9c4743c254b57059a2394e14daf",
        "blockNum":1,
        "txNum":0
      }
    },
    {
      "namespace":"_lifecycle$$h_implicit_org_Org1MSP",
      "key":"e01e1c4304282cc5eda5d51c41795bbe49636fbf174514dbd4b98dc9b9ecf5da",
      "hashed":true,
      "snapshotrecord1":{
        "value":"0c52a3dbc7b322ff35728afdd691244cfc0fc9c4743c254b57059a2394e14daf",
        "blockNum":1,
        "txNum":0
      },
      "snapshotrecord2":null
    },
    {
      "namespace":"marbles",
      "key":"marble1",
      "hashed":false,
      "snapshotrecord1":{
        "value":"{\"docType\":\"marble\",\"name\":\"marble1\",\"color\":\"blue\",\"size\":35,\"owner\":\"tom\"}"
        "blockNum":3,
        "txNum":0
      },
      "snapshotrecord2":{
        "value":"{\"docType\":\"marble\",\"name\":\"marble1\",\"color\":\"blue\",\"size\":35,\"owner\":\"jerry\"}",
        "blockNum":4,
        "txNum":0
      }
    }
  ]
}
```

The field `ledgerid` indicates the name of the channel where the comparison took place. The field `diffRecords` provides a list of key/value differences found between the compared snapshots. The compare tool found 3 differences. The first diffRecord has a null value for the field `snapshotrecord1` indicating that the key is present in the second snapshot and missing in the first snapshot. The second diffRecord has a null value for the field `snapshotrecord2` indiciating the opposite scenario. Both diffRecords have a `hashed` value of true, indicating that they are private key/value differences. The third diffRecord contains data in both snapshot fields, indicating that the snapshots have divergent data for the same key. Further examination reveals the snapshots are in disagreement about the owner of the key `marbles marble1` and the height at which that value is set. The `hashed` field is set to false indicating this is a public key/value difference.

## identifytxs

The `ledgerutil identifytxs` command allows administrators to identify lists of transactions that have written to a set of keys in a peer's local block store. The command takes a JSON file input containing a list of keys and outputs a JSON file per key, each file containing a list of transactions, if the available block range is valid. When troubleshooting a divergence, the command is most effective when paired with the `ledgerutil compare` command, by taking a `ledger compare` generated output JSON file of key differences as input, so that `ledgerutil identifytxs` can identify the transactions on the blockchain associated with the in doubt keys. The command does not necessarily have to be used in tandem with the `ledgerutil compare` command and can be used with a user generated JSON list of keys or an edited `ledgerutil compare` output JSON file to filter transactions in a more general approach.

As previously mentioned, the `ledgerutil identifytxs` command is designed to accept the output of the `ledgerutil compare` command, however the tool is compatible with any JSON file that is formatted the same way. This allows for users to perform a more direct troubleshooting approach in circumstances where they may be more informed on the cause of the divergence. The input JSON file should contain a `ledgerid` that directs the tool to the channel name it must conduct the search on and a `diffRecords` list providing a list of keys for the tool to search for within transactions. Below is an example of a valid input JSON file:

```
{
  "ledgerid":"mychannel",
  "diffRecords":[
    {
      "namespace":"marbles",
      "key":"marble1",
      "snapshotrecord1":{
        "value":"{\"docType\":\"marble\",\"name\":\"marble1\",\"color\":\"blue\",\"size\":35,\"owner\":\"tom\"}","blockNum":3,
        "txNum":0
      },
      "snapshotrecord2":{
        "value":"{\"docType\":\"marble\",\"name\":\"marble1\",\"color\":\"blue\",\"size\":35,\"owner\":\"jerry\"}","blockNum":4,
        "txNum":0
      }
    }
    {
      "namespace":"marbles",
      "key":"marble2"
    }
  ]
}
```

Notice that in the above example for key `marbles marble2`, the fields for the snapshot records and the height of the key's found divergence which are present in output JSON files generated by the `ledgerutil compare` command are exempt. In this case, the entire available block store would be searched as there is no known height of divergence.

The output of the `ledgerutil identifytxs` command is a directory containing a set of JSON files, one per input key. Continuing the example above, if the example JSON was used as the input file for the `ledgerutil identifytxs` command, the resulting output would be a directory containing two JSON files named `txlist1.json` and `txlist2.json`. The output JSON files are named in the order they appear in the input JSON file, so `txlist1.json` would contain a list of transactions that wrote to the namespace key pairing `marbles marble1` and `txlist2.json` would contain a list of transactions that wrote to the namespace key pairing `marbles marble2`. Below is an example of the contents of `txlist1.json`:

```
{
  "namespace":"marbles",
  "key":"marble1",
  "txs": [
    {
      "txid":"9ccb0d0bf19f143b29f17254364ccae987a8d89317f8e8dd81228762fef9da5f",
      "blockNum":2,
      "txNum":2,
      "txValidationStatus":"VALID",
      "keyWrite":"{\"docType\":\"marble\",\"name\":\"marble1\",\"color\":\"blue\",\"size\":35,\"owner\":\"john\"}"
    },
    {
      "txid":"a67c735fa1ef3390199aa2669a4f8023ea469cfe213afebf1014e57bceaf0a57",
      "blockNum":4,
      "txNum":0,
      "txValidationStatus":"VALID",
      "keyWrite":"{\"docType\":\"marble\",\"name\":\"marble1\",\"color\":\"blue\",\"size\":35,\"owner\":\"tom\"}"
    }
  ],
  "blockStoreHeightSufficient":true
}
```

The above output JSON file indicates that for the key `marbles marble1`, two transactions were found in the available block store that wrote to `marbles marble1`, the first transaction occurring at block 2 transaction 2, the second occurring at block 4 transaction 0. The field `blockStoreHeightSufficient` is used to inform the user if the available block range was sufficient for a full search up to the given key's height. If true, the last available block in the block store was at least at the height of the key's height of divergence, indicating the block store height was sufficient. If false, the last available block in the block store was not at the height of the key's height of divergence, indicating there may be transactions relevant to the key that are not present. In the case of the example, `blockStoreHeightSufficient` indicates that the block store's block height is at least 4 and that any transactions past this height would be irrelevant for troubleshooting purposes. Since no height of divergence was provided for the key `marbles marble2`, the `blockStoreHeightSufficient` would default to true in the `txlist2.json` file and becomes a less useful point of troubleshooting information.

A block range is valid if the earliest available block in the local block store has a lower height than the height of the highest input key; if otherwise, a block store search for the input keys would be futile and the command throws an error. It is important to understand that, in cases where the earliest block available is greater than block 1 (which is typical for block stores of peers that have been bootstrapped by a snapshot), the output of the command may not be an exhaustive list of relevant transactions since the earliest blocks were not available to be searched. Further troubleshooting may be necessary in these circumstances.

## verify

The `ledgerutil verify` command allows administrator to check integrity of ledgers in a peer's local block store. While a blockchain ledger has an inherent structure such as hash chains that indicates whether it has got corrupted or not, peers usually do not verify it after the blocks are processed and persisted in a ledger. This subcommand verifies the local ledgers by using the hash values to show if they have any integrity error or not.

The `ledgerutil verify` accepts the path to a block store as the input, and outputs a directory containing one or multiple directories, each of which contains a result JSON file for one channel in the block store. The JSON file will contain errors detected in the ledger for the channel. If the verification is successful (i.e. all the hash values in the block headers are consistent with the block contents), the file will have just an empty array like:

```json
[
]
```

However, if it detects errors, each element of the array will contain the number of the block which has an error and the type of the error found like:

```json
[
{"blockNum":0,"valid":false,"errors":["DataHash mismatch"]}
,
{"blockNum":1,"valid":false,"errors":["PreviousHash mismatch"]}
]
```

The first element in the above output JSON file indicates that the hash value in the header of the block 0 (the genesis block), `DataHash`, does not match that calculated from the contents of the block. The second element indicates the "previous" hash value in the header of the block 1, `PreviousHash`, does not match the hash value calculated from the header of the previous block, i.e. Block 0. This implies that some data corruption exists in the header of the block 0. Then the administrator may want to compare ledgers from multiple peers using other `ledgerutil` subcommands above for further checks, or they may want to discard and rebuild the peer.
