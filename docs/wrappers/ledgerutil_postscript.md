## Exit Status

### ledgerutil compare

- `0` if the snapshots are identical
- `2` if the snapshots are not identical
- `1` if an error occurs

### ledgerutil identifytxs

- `0` if the block store was successfully searched for transactions
- `1` if an error occurs or the block range is invalid

### ledgerutil verify

- `0` if all the checks for the ledgers in the block store are successful
- `1` if an error occurs

## Example Usage

### ledgerutil compare example

Here is an example of the `ledgerutil compare` command.

  * Compare snapshots from two different peers for mychannel at snapshot height 5.

    ```
    ledgerutil compare -o ./compare_output -f 10 ./peer0.org1.example.com/snapshots/completed/mychannel/5 ./peer1.org1.example.com/snapshots/completed/mychannel/5

    Both snapshot public state and private state hashes were the same. No results were generated.
    ```

    The response above indicates that the snapshots are identical. If the snapshots were not identical, the command results will indicate where the comparison output files are written, for example:

    ```
    Successfully compared snapshots. Results saved to compare_output/mychannel_5_comparison. Total differences found: 3
    ```

  * Note that both snapshot locations must be accessible by the command, for example by mounting volumes from two different peers, or by copying the snapshots to a common location.

### ledgerutil identifytxs example

Here is an example of the `ledgerutil identifytxs` command.

  * Identify relevant transactions in mychannel from a block store within an allowed block range. The input JSON file contains a list of keys to search for in the block store. The block range is from the earliest block in the block store to the tallest block height for any key in the input JSON file. The block store must at least have a height equal to the height of the tallest key in the input JSON file to be considered a valid block range.

    ```
    ledgerutil identifytxs compare_output/mychannel_5_comparison/first_diffs_by_height.json -o identifytxs_output

    Successfully ran identify transactions tool. Transactions were checked between blocks 1 and 4.
    ```

    The response above indicates that the local block store was successfully searched. This means transactions within the block range that wrote to keys found in the output JSON of the compare command were identified. In the newly created directory identifytxs_output, a directory mychannel_identified_transactions was generated containing a JSON file of identified transactions for each key from the compare command JSON output.


### ledgerutil verify example

Here is an example of the `ledgerutil verify` command.

  * Check the integrity of the blocks in a block store.

    ```
    ledgerutil verify ./peer0.org1.example.com/ledgersData/chains -o ./verify_output
    ```

    When it finds no error, it will simply output:
    ```
    Successfully executed verify tool. No error is found.
    ```

    Otherwise, it will show:
    ```
    Successfully executed verify tool. Some error(s) are found.
    ```

    The details of the errors can be found in the JSON files under the result directory (`./verify_output` in the example above).

  * Note that since the `ledgerutil verify` command uses the indices in the block store, it is recommended to run the command against a copy of the block store, not the block store of a running peer directly.

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
