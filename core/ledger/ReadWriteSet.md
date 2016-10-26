### Read-Write set semantics

This documents discusses the details of the current implementation about the semantics of read-write sets.

##### Transaction simulation and read-write set
During simulation of a transaction at an `endorser`, a read-write set is prepared for the transaction. The `read set` contains a list of unique keys and their their committed versions that the transaction reads during simulation. The `write set` contains a list of unique keys (though there can be overlap with the keys present in the read set) and their new values that the transaction writes. A delete marker is set (in the place of new value) for the key if the update performed by the transaction is to delete the key.

Further, if the transaction writes a value multiple times for a key, only the last written value is retained. Also, if a transaction reads a value for a key that the transaction itself has written before, the last written value is returned instead of the value present in the committed snapshot; one implication of this is that if a transaction writes a value for a key before reading it from the committed snapshot, the key does not appear in the read set of the transaction.

As noted earlier, the versions of the keys are recorded only in the read set; the write set just contains the list of unique keys and their latest values set by the transaction.

Following is an illustration of an example read-write set prepared by simulation of an hypothetical transaction.

```
<TxReadWriteSet>
  <NsReadWriteSet name="chaincode1">
    <read-set>
      <read key="K1", version="1">
      <read key="K2", version="1">
    </read-set>
    <write-set>
      <write key="K1", value="V1"
      <write key="K3", value="V2"
      <write key="K4", isDelete="true"
    </write-set>
  </NsReadWriteSet>
<TxReadWriteSet>
```

##### Transaction validation and updating world state using read-write set
A `committer` uses the read set portion of the read-write set for checking the validity of a transaction and the write set portion of the read-write set for updating the versions and the values of the affected keys.

In the validation phase, a transaction is considered `valid` iff the version of each key present in the read-set of the transaction matches the version for the same key in the world state - assuming all the preceding `valid` transactions (including the preceding transactions in the same block) are committed.

If a transaction passes the validity check, the committer uses the write set for updating the world state. In the update phase, for each key present in the write set, the value in the world state for the same key is set to the value as specified in the write set. Further, the version of the key in the world state is incremented by one.

##### Example simulation and validation
This section helps understanding the semantics with the help of an example scenario.
For the purpose of this example, the presence of a key `k` in the world state is represented by a tuple `(k,ver,val)` where `ver` is the latest version of the key `k` having `val` as its value.

Now, consider a set of file transactions `T1, T2, T3, T4, and T5`, all simulated on the same snapshot of the world state. Following snippet shows the snapshot of the world state against witch the transactions are simulated and the sequence of read and write activities performed by each of these transactions.

```
World state: (k1,1,v1), (k2,1,v2), (k3,1,v3), (k4,1,v4), (k5,1,v5)
T1 -> Write(k1, v1'), Write(k2, v2')
T2 -> Read(k1), Write(k3, v3')
T3 -> Write(k2, v2'')
T4 -> Write(k2, v2'''), read(k2)
T5 -> Write(k6, v6'), read(k1)
```
Now, assume that these transactions are ordered in the sequence of T1,..,T5 (could be contained in a single block or different blocks)

1. `T1` passes the validation because it does not perform any read. Further, the tuple of keys `k1` and `k2` in the world state are updated to `(k1,2,v1'), (k2,2,v2')`

2. `T2` fails the validation because it reads a key `k1` which is modified by a preceding transaction `T1`

3. `T3` passes the validation because it does not perform a read. Further the tuple of the key `k2` in the world state are updated to `(k2,3,v2'')`

4. `T4` passes the validation because it performs a read the key `k2` after writing the new value (though the key was modified by a preceding transaction `T1`). Further the tuple of the key `k2` in the world state are updated to `(k2,4,v2''')`

5. `T5` fails the validation because it performs a read for key `k1` which is modified by a preceding transaction `T1`

#### Transactions with multiple read-write sets
If a transaction contains multiple read-write sets as a result of including different simulations results in a single transaction, the validation also checks for read conflicts between the read-write sets in addition to the read conflicts check with preceding transactions.

#### Questions
1. In the final block, is there a benefit of persisting read-set portion of the read-write set? The advantage of not storing clearly reduces the storage space requirement. If we chose not to store the read-set, the endorsers should sign only the write set portion of the read-write set which means that the
`actionBytes` field in the `EndorsedAction` would contain only write set and a separate field would be required for the read set.

2. Is there a benefit of deciding the version for a key in the write-set at simulation time instead of commit time? If we fix the version at the simulation time, then we would have to discard the transactions that have only write conflicts (i.e., some other transaction has written the version).
