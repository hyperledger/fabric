# Client Identity Chaincode Library

The client identity chaincode library enables you to write chaincode which
makes access control decisions based on the identity of the client
(i.e. the invoker of the chaincode).  In particular, you may make access
control decisions based on either or both of the following associated with
the client:

* the client identity's MSP (Membership Service Provider) ID
* an attribute associated with the client identity

Attributes are simply name and value pairs associated with an identity.
For example, `email=me@gmail.com` indicates an identity has the `email`
attribute with a value of `me@gmail.com`.


## Using the client identity chaincode library

This section describes how to use the client identity chaincode library.

All code samples below assume two things:
1. The type of the `stub` variable is `ChaincodeStubInterface` as passed
   to your chaincode.
2. You have added the following import statement to your chaincode.
    ```
    import "github.com/hyperledger/fabric/core/chaincode/shim/ext/cid"
    ```
#### Getting the client's ID

The following demonstrates how to get an ID for the client which is guaranteed
to be unique within the MSP:

```
id, err := cid.GetID(stub)
```

#### Getting the MSP ID

The following demonstrates how to get the MSP ID of the client's identity:

```
mspid, err := cid.GetMSPID(stub)
```

#### Getting an attribute value

The following demonstrates how to get the value of the *attr1* attribute:

```
val, ok, err := cid.GetAttributeValue(stub, "attr1")
if err != nil {
   // There was an error trying to retrieve the attribute
}
if !ok {
   // The client identity does not possess the attribute
}
// Do something with the value of 'val'
```

#### Asserting an attribute value

Often all you want to do is to make an access control decision based on the value
of an attribute, i.e. to assert the value of an attribute.  For example, the following
will return an error if the client does not have the `myapp.admin` attribute
with a value of `true`:

```
err := cid.AssertAttributeValue(stub, "myapp.admin", "true")
if err != nil {
   // Return an error
}
```

This is effectively using attributes to implement role-based access control,
or RBAC for short.

#### Getting the client's X509 certificate

The following demonstrates how to get the X509 certificate of the client, or
nil if the client's identity was not based on an X509 certificate:

```
cert, err := cid.GetX509Certificate(stub)
```

Note that both `cert` and `err` may be nil as will be the case if the identity
is not using an X509 certificate.

#### Performing multiple operations more efficiently

Sometimes you may need to perform multiple operations in order to make an access
decision.  For example, the following demonstrates how to grant access to
identities with MSP *org1MSP* and *attr1* OR with MSP *org1MSP* and *attr2*.

```
// Get the client ID object
id, err := cid.New(stub)
if err != nil {
   // Handle error
}
mspid, err := id.GetMSPID()
if err != nil {
   // Handle error
}
switch mspid {
   case "org1MSP":
      err = id.AssertAttributeValue("attr1", "true")
   case "org2MSP":
      err = id.AssertAttributeValue("attr2", "true")
   default:
      err = errors.New("Wrong MSP")
}
```
Although it is not required, it is more efficient to make the `cid.New` call
to get the ClientIdentity object if you need to perform multiple operations,
as demonstrated above.

## Adding Attributes to Identities

This section describes how to add custom attributes to certificates when
using Hyperledger Fabric CA as well as when using an external CA.

#### Managing attributes with Fabric CA

There are two methods of adding attributes to an enrollment certificate
with fabric-ca:

  1. When you register an identity, you can specify that an enrollment certificate
     issued for the identity should by default contain an attribute.  This behavior
     can be overridden at enrollment time, but this is useful for establishing
     default behavior and, assuming registration occurs outside of your application,
     does not require any application change.

     The following shows how to register *user1* with two attributes:
     *app1Admin* and *email*.
     The ":ecert" suffix causes the *appAdmin* attribute to be inserted into user1's
     enrollment certificate by default.  The *email* attribute is not added
     to the enrollment certificate by default.

     ```
     fabric-ca-client register --id.name user1 --id.secret user1pw --id.type user --id.affiliation org1 --id.attrs 'app1Admin=true:ecert,email=user1@gmail.com'
     ```

  2. When you enroll an identity, you may request that one or more attributes
     be added to the certificate.
     For each attribute requested, you may specify whether the attribute is
     optional or not.  If it is not optional but does not exist for the identity,
     enrollment fails.

     The following shows how to enroll *user1* with the *email* attribute,
     without the *app1Admin* attribute and optionally with the *phone* attribute
     (if the user possesses *phone* attribute).
     ```
     fabric-ca-client enroll -u http://user1:user1pw@localhost:7054 --enrollment.attrs "email,phone:opt"
     ```
#### Attribute format in a certificate

Attributes are stored inside an X509 certificate as an extension with an
ASN.1 OID (Abstract Syntax Notation Object IDentifier)
of `1.2.3.4.5.6.7.8.1`.  The value of the extension is a JSON string of the
form `{"attrs":{<attrName>:<attrValue}}`.  The following is a sample of a
certificate which contains the `attr1` attribute with a value of `val1`.
See the final entry in the *X509v3 extensions* section.  Note also that the JSON
entry could contain multiple attributes, though this sample shows only one.

```
Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number:
            1e:49:98:e9:f4:4f:d0:03:53:bf:36:81:c0:a0:a4:31:96:4f:52:75
        Signature Algorithm: ecdsa-with-SHA256
        Issuer: CN=fabric-ca-server
        Validity
            Not Before: Sep  8 03:42:00 2017 GMT
            Not After : Sep  8 03:42:00 2018 GMT
        Subject: CN=MyTestUserWithAttrs
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
            EC Public Key:
                pub:
                    04:e6:07:5a:f7:09:d5:af:38:e3:f7:a2:90:77:0e:
                    32:67:5b:70:a7:37:ca:b5:c9:d8:91:77:39:ae:03:
                    a0:36:ad:72:b3:3c:89:6d:1e:f6:1b:6d:2a:88:49:
                    92:6e:6e:cc:bc:81:52:fa:19:88:18:5c:d7:6e:eb:
                    d4:73:cc:51:79
                ASN1 OID: prime256v1
        X509v3 extensions:
            X509v3 Key Usage: critical
                Certificate Sign
            X509v3 Basic Constraints: critical
                CA:FALSE
            X509v3 Subject Key Identifier:
                D8:28:B4:C0:BC:92:4A:D3:C3:8C:54:6C:08:86:33:10:A6:8D:83:AE
            X509v3 Authority Key Identifier:
                keyid:C4:B3:FE:76:0D:E2:DE:3C:FC:75:FB:AE:55:86:04:F0:BB:7F:F6:01

            X509v3 Subject Alternative Name:
                DNS:Anils-MacBook-Pro.local
            1.2.3.4.5.6.7.8.1:
                {"attrs":{"attr1":"val1"}}
    Signature Algorithm: ecdsa-with-SHA256
        30:45:02:21:00:fb:84:a9:65:29:b2:f4:d3:bc:1a:8b:47:92:
        5e:41:27:2d:26:ec:f7:cd:aa:86:46:a4:ac:da:25:be:40:1d:
        c5:02:20:08:3f:49:86:58:a7:20:48:64:4c:30:1b:da:a9:a2:
        f2:b4:16:28:f6:fd:e1:46:dd:6b:f2:3f:2f:37:4a:4c:72
```

If you want to use the client identity library to extract or assert attribute
values as described previously but you are not using Hyperledger Fabric CA,
then you must ensure that the certificates which are issued by your external CA
contain attributes of the form shown above.  In particular, the certificates
must contain the `1.2.3.4.5.6.7.8.1` X509v3 extension with a JSON value
containing the attribute names and values for the identity.
