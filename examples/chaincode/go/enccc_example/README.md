# Using EncCC

To test `EncCC` you need to first generate an AES 256 bit key as a base64 
encoded string so that it can be passed as JSON to the peer chaincode 
invoke's transient parameter.

Note: Before getting started you must use govendor to add external dependencies.  Please issue the following commands inside the "enccc_example" folder:
```
govendor init
govendor add +external
```

Let's generate the encryption and decryption keys.  The example will simulate a shared key so the key is used for both encryption and decryption.
```
ENCKEY=`openssl rand 32 -base64` && DECKEY=$ENCKEY
```

At this point, you can invoke the chaincode to encrypt key-value pairs as
follows:

Note: the following assumes the env is initialized and peer has joined channel "my-ch".
```
peer chaincode invoke -n enccc -C my-ch -c '{"Args":["ENCRYPT","key1","value1"]}' --transient "{\"ENCKEY\":\"$ENCKEY\"}" 
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
peer chaincode invoke -n enccc -C my-ch -c '{"Args":["ENCRYPT","key2","value2"]}' --transient "{\"ENCKEY\":\"$ENCKEY\",\"IV\":\"$IV\"}" 
```

Two such invocations will produce equal KVS writes, which can be endorsed by multiple nodes.

The value can be retrieved back as follows

```
peer chaincode query -n enccc -C my-ch -c '{"Args":["DECRYPT","key1"]}' --transient "{\"DECKEY\":\"$DECKEY\"}"
```
```
peer chaincode query -n enccc -C my-ch -c '{"Args":["DECRYPT","key2"]}' --transient "{\"DECKEY\":\"$DECKEY\"}"
```
Note that in this case we use a chaincode query operation; while the use of the
transient field guarantees that the content will not be written to the ledger,
the chaincode decrypts the message and puts it in the proposal response. An
invocation would persist the result in the ledger for all channel readers to
see whereas a query can be discarded and so the result remains confidential.

To test signing and verifying, you also need to generate an ECDSA key for the appopriate
curve, as follows. 

```
On Intel:
SIGKEY=`openssl ecparam -name prime256v1 -genkey | tail -n5 | base64 -w0` && VERKEY=$SIGKEY

On Mac:
SIGKEY=`openssl ecparam -name prime256v1 -genkey | tail -n5 | base64` && VERKEY=$SIGKEY
```

At this point, you can invoke the chaincode to sign and then encrypt key-value
pairs as follows

```
peer chaincode invoke -n enccc -C my-ch -c '{"Args":["ENCRYPTSIGN","key3","value3"]}' --transient "{\"ENCKEY\":\"$ENCKEY\",\"SIGKEY\":\"$SIGKEY\"}"
```

And similarly to retrieve them using a query

```
peer chaincode query -n enccc -C my-ch -c '{"Args":["DECRYPTVERIFY","key3"]}' --transient "{\"DECKEY\":\"$DECKEY\",\"VERKEY\":\"$VERKEY\"}"
```
