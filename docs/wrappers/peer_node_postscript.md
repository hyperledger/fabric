## Example Usage

### peer node pause example

The following command:

```
peer node pause -c ch1
```

pauses a channel on the peer. When the peer starts after pause, the paused channel will not be started
and the peer will not receive blocks for the paused channel.


### peer node rebuild-dbs example

The following command:

```
peer node rebuild-dbs
```

drops the databases for all the channels. When the peer is started after running this command, the peer will
retrieve the blocks stored on the peer and rebuild the dropped databases for all the channels.

### peer node reset example

The following command:

```
peer node reset
```

resets all channels in the peer to the genesis block, i.e., the first block in the channel. The command also records the pre-reset height of each channel in the file system. Note that the peer process should be stopped while executing this command. If the peer process is running, this command detects that and returns an error instead of performing the reset. When the peer is started after performing the reset, the peer will fetch the blocks for each channel which were removed by the reset command (either from other peers or orderers) and commit the blocks up to the pre-reset height. Until all channels reach the pre-reset height, the peer will not endorse any transactions.

### peer node resume example

The following command:

```
peer node resume -c ch1
```

resumes a channel on the peer. When the peer starts after resume, the resumed channel will be started
and the peer will receive blocks for the resumed channel.

### peer node rollback example

The following command:

```
peer node rollback -c ch1 -b 150
```

rolls back the channel ch1 to block number 150. The command also records the pre-rolled back height of channel ch1 in the file system. Note that the peer should be stopped while executing this command. If the peer process is running, this command detects that and returns an error instead of performing the rollback. When the peer is started after performing the rollback, the peer will fetch the blocks for channel ch1 which were removed by the rollback command (either from other peers or orderers) and commit the blocks up to the pre-rolled back height. Until the channel ch1 reaches the pre-rolled back height, the peer will not endorse any transaction for any channel.

### peer node start example

The following command:

```
peer node start --peer-chaincodedev
```

starts a peer node in chaincode development mode. Normally chaincode containers are started
and maintained by peer. However in chaincode development mode, chaincode is built and started by the user. This mode is useful during chaincode development phase for iterative development.

### peer node unjoin example 

The following command: 

```
peer node unjoin -c mychannel
```

unjoins the peer from channel `mychannel`, removing all content from the ledger and transient storage.  When unjoining a channel, the peer must be shut down.


### peer node upgrade-dbs example

The following command:

```
peer node upgrade-dbs
```

checks the data format in the databases for all the channels and drops databases if data format is in the previous version.
The command will return an error if the data format is already up to date. When the peer is started after running this command,
the peer will retrieve the blocks stored on the peer and rebuild the dropped databases in the new format.

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
