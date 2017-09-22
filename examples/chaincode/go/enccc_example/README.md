# Using EncCC

To test `EncCC` you need to first generate an AES 256 bit key as a base64 
encoded string so that it can be passed as JSON to the peer chaincode 
invoke's transient parameter

```
ENCKEY=`openssl rand 32 -base64`
```

At this point, you can invoke the chaincode to encrypt key-value pairs as
follows

```
peer chaincode invoke -n enccc -C my-ch -c '{"Args":["ENC","PUT","key","value"]}' --transient "{\"ENCKEY\":\"$ENCKEY\"}"
```

This call will encrypt using a random IV. This may be undesirable for
instance if the chaincode invocation needs to be endorsed by multiple
peers since it would cause the endorsement of conflicting read/write sets.
It is possible to encrypt deterministically by specifying the IV, as
follows: at first the IV must be created

```
IV=`openssl rand 16 -base64`
```

Then, the IV may be specified in the transient field

```
peer chaincode invoke -n enccc -C my-ch -c '{"Args":["ENC","PUT","key","value"]}' --transient "{\"ENCKEY\":\"$ENCKEY\",\"IV\":\"$IV\"}"
```

Two such invocations will produce equal KVS writes, which can be endorsed by multiple nodes.

The value can be retrieved back as follows

```
peer chaincode query -n enccc -C my-ch -c '{"Args":["ENC","GET","key"]}' --transient "{\"ENCKEY\":\"$ENCKEY\"}"
```

Note that in this case we use a chaincode query operation; while the use of the
transient field guarantees that the content will not be written to the ledger,
the chaincode decrypts the message and puts it in the proposal response. An
invocation would persist the result in the ledger for all channel readers to
see whereas a query can be discarded and so the result remains confidential.

To test signing, you also need to generate an ECDSA key for the appopriate
curve, as follows

```
SIGKEY=`openssl ecparam -name prime256v1 -genkey | tail -n5 | base64 -w0`
```

At this point, you can invoke the chaincode to sign and then encrypt key-value
pairs as follows

```
peer chaincode invoke -n enccc -C my-ch -c '{"Args":["SIG","PUT","key","value"]}' --logging-level debug -o 127.0.0.1:7050 --transient "{\"ENCKEY\":\"$ENCKEY\",\"SIGKEY\":\"$SIGKEY\"}"
```

And similarly to retrieve them using a query

```
peer chaincode query -n enccc -C my-ch -c '{"Args":["SIG","GET","key"]}' --logging-level debug -o 127.0.0.1:7050 --transient "{\"ENCKEY\":\"$ENCKEY\",\"SIGKEY\":\"$SIGKEY\"}"
```
