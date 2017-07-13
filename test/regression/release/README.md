# Release Tests
These tests make up our Release Criteria. They must pass for a stamp of approval.

## This page is *WIP*

## Performance Traffic Engine (PTE) Tests
*Note: these are all of the PTE system tests; we should choose only a subset...*

```
        cd ../daily; py.test -v --junitxml results.xml test_pte.py
        FAB-2032,FAB-3584 LevelDB SkeletonQueries
        FAB-2032,FAB-3586 LevelDB SkeletonInvokes
        FAB-2032,FAB-3593 LevelDB TLS
        FAB-2032,FAB-3595 LevelDB 1M
        FAB-2032,FAB-3597 LevelDB Gossip
        FAB-2032,FAB-3599 LevelDB 12Hr
        FAB-2032,FAB-3585 CouchDB SkeletonQueries
        FAB-2032,FAB-3587 CouchDB SkeletonInvokes
        FAB-2032,FAB-3588 CouchDB Scaleup1
        FAB-2032,FAB-3589 CouchDB Scaleup2
        FAB-2032,FAB-3590 CouchDB Scaleup3
        FAB-2032,FAB-3591 CouchDB Scaleup4
        FAB-2032,FAB-3592 CouchDB Scaleup5
        FAB-2032,FAB-3594 CouchDB TLS
        FAB-2032,FAB-3596 CouchDB 1M
        FAB-2032,FAB-3598 CouchDB Gossip
        FAB-2032,FAB-3600 CouchDB 12Hr

        cd ../weekly; py.test -v --junitxml results.xml test_pte.py
        FAB-2032,FAB-3601 LevelDB 72Hr
        FAB-2032,FAB-3602 CouchDB 72Hr

```

## Chaincode Tests
These tests launch a network, and deploy and execute an end-to-end test for several different examples of chaincodes. *Note: this is all of them; we should choose a subset...*

```
        cd ../daily; py.test -v --junitxml results.xml test_pte.py
        chaincodeTests/runChaincodes.sh
        *TBD: list of tests names using example02, example05, etc.*

```

## Another Category of Tests
(Description...)

```
        (list of tests names, and how to run them)

```


<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
s
