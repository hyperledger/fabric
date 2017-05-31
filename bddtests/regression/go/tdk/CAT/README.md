## Consensus Acceptance Testcases (CAT)
Hyperledger Fabric Go language Test Development Kit (TDK)
tests can be executed on local fabric in vagrant environment with docker
containers.
Additional details (the date the tests were run, commit image,
test config parameters, the test steps, output summary and details, etc.)
are available in the GO_TEST files in this folder.

### CAT test naming convention

The testnames themselves indicate the steps performed. For example:

```
    CAT_210_S2S1_IQ_R1_IQ.go    Consensus Acceptance Test (CAT) suite, testcase number 210
           _S2S1                Stop Validating PEERs VP2 and VP1 at virtually the same time
                _IQ             Send some Invoke requests to all running peers, and Query all to validate A/B/ChainHeight results
                   _R1          Restart VP1
                      _IQ       Send some Invoke requests to all running peers, and Query all to validate A/B/ChainHeight results
```

### Test Coverage

The objective of Consensus Acceptance Tests (CAT) is to ensure the
stability and resiliency of the
PBFT Batch design when Byzantine faults occur in a 4 peer network.
Test areas coverage:

Stop 1 peer: the network continues to process deploys, invokes, and query
transactions.
Perform this operation on each peer in the fabric.
While exactly 3 peers are running, their ledgers will remain in synch.
(In a 4 peer network, 3 represents 2F+1, the minimum number required for
consensus.)
Restarting a 4th peer will cause it to join in the network operations again.
Note: when queried after having been restarted, the ledger on extra
peers ( additional nodes beyond  2(F+1) )
may appear to lag for some time. It will catch up if another peer is
stopped (leaving it as one of exactly 2F+1 participating peers), OR, with
no further disruptions in the network,
it could catch up after huge numbers of transaction batches are processed.

Stop 2 peers: the network should halt advancement, due to a lack of consensus.
Restarting 1 or 2 peers will cause the network to resume processing
transactions because enough nodes are available to reach consensus. This may
include processing transactions that were received and queued by any running
peers while consensus was halted.

Stop 3 peers: the network should halt advancement due to a lack of consensus.
Restarting just one of the peers should not resume consensus.
Restarting 2 or 3 peers should cause the network to resume consensus.

Deploys should be processed, or queued if appropriate, with any number of
running peers.


### RESULTS SUMMARY

```
    Consensus - Failed Testcases            Opened Bugs        Description

    CAT_111_SnIQRnIQ_cycleDownLoop.go       FAB-337            dup Tx
    CAT_303_S0S1S2_IQ_R0R1R2_IQ.go          FAB-333            lost Tx after restart 3 peers together
    CAT_305_S1S2S3_IQ_R1R2R3_IQ.go          FAB-333            lost Tx after restart 3 peers together
    CAT_407_S0S1S2_D_I_R0R1_IQ.go           FAB-911            lost Tx from vp0 after stop vp0/vp1/vp2, deploy, restart vp0/vp1, invokes
    CAT_408_S0S1S2_D_I_R0R1R2_IQ.go         FAB-335            deploy failed when restart 3 peers together
    CAT_409_S1S2S3_D_I_R1R2_IQ.go           FAB-334/FAB-912    lost Tx after all peers after stop vp1/vp2/vp3, deploy, restart vp1/vp2, invokes
    CAT_410_S1S2S3_D_I_R1R2R3_IQ.go         FAB-335            deploy failed when restart 3 peers together
    CAT_412_S0S1S2_D_I_R1R2_IQ.go           FAB-334/FAB-912    lost Tx after all peers after stop vp1/vp2/vp3, deploy, restart vp1/vp2, invokes

    CRT_502_StopAndRestart1or2_10Hrs.go     FAB-331            acked Tx lost after stop/restart peers many times

    CAT_201_S2S1_IQDIQ.go (using Pauses)    FAB-336            no http response from vp0, while peers vp1,vp2 docker paused
```


<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
s
