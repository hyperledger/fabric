Instructions to generate client key/certificate
-----------------------------------------------

Use CFSSL to generate the client certificate if they expire

```
cfssl gencert -ca=ca.pem -ca-key=ca_key.pem -config=config.json -profile=client client.json | cfssljson -bare client
cfssl gencert -ca=ca.pem -ca-key=ca_key.pem -config=config.json -profile=server server.json | cfssljson -bare server

```
