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

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
