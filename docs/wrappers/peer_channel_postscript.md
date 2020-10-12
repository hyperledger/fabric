## Example Usage

### peer channel create examples

Here's an example that uses the `--orderer` global flag on the `peer channel
create` command.

* Create a sample channel `mychannel` defined by the configuration transaction
  contained in file `./createchannel.tx`. Use the orderer at `orderer.example.com:7050`.

  ```
  peer channel create -c mychannel -f ./createchannel.tx --orderer orderer.example.com:7050

  2018-02-25 08:23:57.548 UTC [channelCmd] InitCmdFactory -> INFO 003 Endorser and orderer connections initialized
  2018-02-25 08:23:57.626 UTC [channelCmd] InitCmdFactory -> INFO 019 Endorser and orderer connections initialized
  2018-02-25 08:23:57.834 UTC [channelCmd] readBlock -> INFO 020 Received block: 0
  2018-02-25 08:23:57.835 UTC [main] main -> INFO 021 Exiting.....

  ```

  Block 0 is returned indicating that the channel has been successfully created.

Here's an example of the `peer channel create` command option.

* Create a new channel `mychannel` for the network, using the orderer at ip
  address `orderer.example.com:7050`.  The configuration update transaction
  required to create this channel is defined the file `./createchannel.tx`.
  Wait 30 seconds for the channel to be created.

  ```
    peer channel create -c mychannel --orderer orderer.example.com:7050 -f ./createchannel.tx -t 30s

    2018-02-23 06:31:58.568 UTC [channelCmd] InitCmdFactory -> INFO 003 Endorser and orderer connections initialized
    2018-02-23 06:31:58.669 UTC [channelCmd] InitCmdFactory -> INFO 019 Endorser and orderer connections initialized
    2018-02-23 06:31:58.877 UTC [channelCmd] readBlock -> INFO 020 Received block: 0
    2018-02-23 06:31:58.878 UTC [main] main -> INFO 021 Exiting.....

    ls -l

    -rw-r--r-- 1 root root 11982 Feb 25 12:24 mychannel.block

  ```

  You can see that channel `mychannel` has been successfully created, as
  indicated in the output where block 0 (zero) is added to the blockchain for
  this channel and returned to the peer, where it is stored in the local
  directory as `mychannel.block`.

  Block zero is often called the *genesis block* as it provides the starting
  configuration for the channel.  All subsequent updates to the channel will be
  captured as configuration blocks on the channel's blockchain, each of which
  supersedes the previous configuration.

### peer channel fetch example

Here's some examples of the `peer channel fetch` command.

* Using the `newest` option to retrieve the most recent channel block, and
  store it in   the file `mychannel.block`.

  ```
  peer channel fetch newest mychannel.block -c mychannel --orderer orderer.example.com:7050

  2018-02-25 13:10:16.137 UTC [channelCmd] InitCmdFactory -> INFO 003 Endorser and orderer connections initialized
  2018-02-25 13:10:16.144 UTC [channelCmd] readBlock -> INFO 00a Received block: 32
  2018-02-25 13:10:16.145 UTC [main] main -> INFO 00b Exiting.....

  ls -l

  -rw-r--r-- 1 root root 11982 Feb 25 13:10 mychannel.block

  ```

  You can see that the retrieved block is number 32, and that the information
  has been written to the file `mychannel.block`.

* Using the `(block number)` option to retrieve a specific block -- in this
  case, block number 16 -- and store it in the default block file.

  ```
  peer channel fetch 16  -c mychannel --orderer orderer.example.com:7050

  2018-02-25 13:46:50.296 UTC [channelCmd] InitCmdFactory -> INFO 003 Endorser and orderer connections initialized
  2018-02-25 13:46:50.302 UTC [channelCmd] readBlock -> INFO 00a Received block: 16
  2018-02-25 13:46:50.302 UTC [main] main -> INFO 00b Exiting.....

  ls -l

  -rw-r--r-- 1 root root 11982 Feb 25 13:10 mychannel.block
  -rw-r--r-- 1 root root  4783 Feb 25 13:46 mychannel_16.block

  ```

  You can see that the retrieved block is number 16, and that the information
  has been written to the default file `mychannel_16.block`.

  For configuration blocks, the block file can be decoded using the
  [`configtxlator` command](./configtxlator.html). See this command for an example
  of decoded output. User transaction blocks can also be decoded, but a user
  program must be written to do this.

### peer channel getinfo example

Here's an example of the `peer channel getinfo` command.

* Get information about the local peer for channel `mychannel`.

  ```
  peer channel getinfo -c mychannel

  2018-02-25 15:15:44.135 UTC [channelCmd] InitCmdFactory -> INFO 003 Endorser and orderer connections initialized
  Blockchain info: {"height":5,"currentBlockHash":"JgK9lcaPUNmFb5Mp1qe1SVMsx3o/22Ct4+n5tejcXCw=","previousBlockHash":"f8lZXoAn3gF86zrFq7L1DzW2aKuabH9Ow6SIE5Y04a4="}
  2018-02-25 15:15:44.139 UTC [main] main -> INFO 006 Exiting.....

  ```

  You can see that the latest block for channel `mychannel` is block 5.  You
  can also see the cryptographic hashes for the most recent blocks in the
  channel's blockchain.

### peer channel join example

Here's an example of the `peer channel join` command.

* Join a peer to the channel defined in the genesis block identified by the file
  `./mychannel.genesis.block`. In this example, the channel block was
  previously retrieved by the `peer channel fetch` command.

  ```
  peer channel join -b ./mychannel.genesis.block

  2018-02-25 12:25:26.511 UTC [channelCmd] InitCmdFactory -> INFO 003 Endorser and orderer connections initialized
  2018-02-25 12:25:26.571 UTC [channelCmd] executeJoin -> INFO 006 Successfully submitted proposal to join channel
  2018-02-25 12:25:26.571 UTC [main] main -> INFO 007 Exiting.....

  ```

  You can see that the peer has successfully made a request to join the channel.

### peer channel joinbysnapshot example

Here's an example of the `peer channel joinbysnapshot` command.

* Join a peer to the channel from a snapshot identified by the directory
  `/snapshots/completed/testchannel/1000`. The snapshot was previously created on a different peer.

  ```
  peer channel joinbysnapshot --snapshotpath /snapshots/completed/testchannel/1000

  2020-10-12 11:41:45.442 EDT [channelCmd] InitCmdFactory -> INFO 001 Endorser and orderer connections initialized
  2020-10-12 11:41:45.444 EDT [channelCmd] executeJoin -> INFO 002 Successfully submitted proposal to join channel
  2020-10-12 11:41:45.444 EDT [channelCmd] joinBySnapshot -> INFO 003 The joinbysnapshot operation is in progress. Use "peer channel joinbysnapshotstatus" to check the status.

  ```

  You can see that the peer has successfully made a request to join the channel from the specified snapshot.
  When a `joinbysnapshot` operation is in progress, you cannot run another `peer channel join`
  or `peer channel joinbysnapshot` simultaneously. To know whether or not a joinbysnapshot operation is in progress,
  you can call the `peer channel joinbysnapshotstatus` command.


### peer channel joinbysnapshotstatus example

Here are some examples of the `peer channel joinbysnapshotstatus` command.

* Query if joinbysnapshot is in progress for any channel. If yes,
  it returns a message indicating a joinbysnapshot operation is in progress.

  ```
  peer channel joinbysnapshotstatus

  2020-10-12 11:41:45.952 EDT [channelCmd] InitCmdFactory -> INFO 001 Endorser and orderer connections initialized
  A joinbysnapshot operation is in progress for snapshot at /snapshots/completed/testchannel/1000
  ```

* If no `joinbysnapshot` operation is in progress, it returns a message indicating no joinbysnapshot operation is in progress.

  ```
  peer channel joinbysnapshotstatus

  2020-10-12 11:41:47.922 EDT [channelCmd] InitCmdFactory -> INFO 001 Endorser and orderer connections initialized
  No joinbysnapshot operation is in progress

  ```

### peer channel list example

  Here's an example of the `peer channel list` command.

  * List the channels to which a peer is joined.

    ```
    peer channel list

    2018-02-25 14:21:20.361 UTC [channelCmd] InitCmdFactory -> INFO 003 Endorser and orderer connections initialized
    Channels peers has joined:
    mychannel
    2018-02-25 14:21:20.372 UTC [main] main -> INFO 006 Exiting.....

    ```

    You can see that the peer is joined to channel `mychannel`.

### peer channel signconfigtx example

Here's an example of the `peer channel signconfigtx` command.

* Sign the `channel update` transaction defined in the file
  `./updatechannel.tx`. The example lists the configuration transaction file
  before and after the command.

  ```
  ls -l

  -rw-r--r--  1 anthonyodowd  staff   284 25 Feb 18:16 updatechannel.tx

  peer channel signconfigtx -f updatechannel.tx

  2018-02-25 18:16:44.456 GMT [channelCmd] InitCmdFactory -> INFO 001 Endorser and orderer connections initialized
  2018-02-25 18:16:44.459 GMT [main] main -> INFO 002 Exiting.....

  ls -l

  -rw-r--r--  1 anthonyodowd  staff  2180 25 Feb 18:16 updatechannel.tx

  ```

  You can see that the peer has successfully signed the configuration
  transaction by the increase in the size of the file `updatechannel.tx` from
  284 bytes to 2180 bytes.

### peer channel update example

Here's an example of the `peer channel update` command.

* Update the channel `mychannel` using the configuration transaction defined in
  the file `./updatechannel.tx`. Use the orderer at ip address
  `orderer.example.com:7050` to send the configuration transaction to all peers
  in the channel to update their copy of the channel configuration.

  ```
  peer channel update -c mychannel -f ./updatechannel.tx -o orderer.example.com:7050

  2018-02-23 06:32:11.569 UTC [channelCmd] InitCmdFactory -> INFO 003 Endorser and orderer connections initialized
  2018-02-23 06:32:11.626 UTC [main] main -> INFO 010 Exiting.....

  ```

  At this point, the channel `mychannel` has been successfully updated.

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
