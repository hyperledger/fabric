## Example Usage

### peer snapshot cancelrequest example

Here is an example of the `peer snapshot cancelrequest` command.

  * Cancel a snapshot request for block number 1000 on channel `mychannel`
    for `peer0.org1.example.com:7051`:

    ```
    peer snapshot cancelrequest -c mychannel -b 1000 --peerAddress peer0.org1.example.com:7051

    Snapshot request cancelled successfully

    ```

    A successful response indicates that the snapshot request for block number 1000 has been removed from `mychannel`.
    If there is no pending snapshot request for block number 1000, the command will return an error.

  * Use the `--tlsRootCertFile` flag in a network with TLS enabled

### peer snapshot listpending example

Here is an example of the `peer snapshot listpending` command.
If a snapshot is already generated, the corresponding block number will be removed from the pending list.

  * List pending snapshot requests on channel `mychannel`
    for `peer0.org1.example.com:7051`:

    ```
    peer snapshot listpending -c mychannel --peerAddress peer0.org1.example.com:7051

    Successfully got pending snapshot requests: [1000 5000]

    ```

    You can see that the command returns a list of block numbers for the pending snapshot requests.

  * Use the `--tlsRootCertFile` flag in a network with TLS enabled

### peer snapshot submitrequest example

Here is an example of the `peer snapshot submitrequest` command.

  * Submit a snapshot request for block number 1000 on channel `mychannel`
    for `peer0.org1.example.com:7051`:

    ```
    peer snapshot submitrequest -c mychannel -b 1000 --peerAddress peer0.org1.example.com:7051

    Snapshot request submitted successfully

    ```

    You can see that the snapshot request is successfully submitted on channel `mychannel`.
    A snapshot will be automatically generated after the block number 1000 is committed on the channel.

    The specified block number must be at or above the last block number on the channel.
    Otherwise, the command will return an error.

  * Use the `--tlsRootCertFile` flag in a network with TLS enabled

