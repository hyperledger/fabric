# Hyperledger Fabric - Application Access Control Lists

## Overview

We consider the following entities:

1. *HelloWorld*: is a chaincode that contains a single function called *hello*;
2. *Alice*: is the *HelloWorld* deployer;
3. *Bob*: is the *HelloWorld*'s functions invoker.

Alice wants to ensure that only Bob can invoke the function *hello*.

## Fabric Support

To allow Alice to specify her own access control lists and Bob to gain access, the fabric layer gives access to following capabilities:

1. Alice and Bob can sign and verify any message with specific transaction certificates or enrollment certificate they own;
2. The fabric allows to *name* each transaction by means of a unique *binding* to be used to bind application data
to the underlying transaction transporting it;
3. Extended transaction format.

The fabric layer exposes the following interfaces and functions to allow the application layer to define its own ACLS.

### Certificate Handler

The following interface allows to sign and verify any message using signing key-pair underlying the associated certificate.
The certificate can be a TCert or an ECert.

```
// CertificateHandler exposes methods to deal with an ECert/TCert
type CertificateHandler interface {

    // GetCertificate returns the certificate's DER
    GetCertificate() []byte

    // Sign signs msg using the signing key corresponding to the certificate
    Sign(msg []byte) ([]byte, error)

    // Verify verifies msg using the verifying key corresponding to the certificate
    Verify(signature []byte, msg []byte) error

    // GetTransactionHandler returns a new transaction handler relative to this certificate
    GetTransactionHandler() (TransactionHandler, error)
}
```


### Transaction Handler

The following interface allows to create transactions and give access to the underlying *binding* that can be leveraged to link
application data to the underlying transaction.


```
// TransactionHandler represents a single transaction that can be named by the output of the GetBinding method.
// This transaction is linked to a single Certificate (TCert or ECert).
type TransactionHandler interface {

    // GetCertificateHandler returns the certificate handler relative to the certificate mapped to this transaction
    GetCertificateHandler() (CertificateHandler, error)

    // GetBinding returns a binding to the underlying transaction
    GetBinding() ([]byte, error)

    // NewChaincodeDeployTransaction is used to deploy chaincode
    NewChaincodeDeployTransaction(chaincodeDeploymentSpec *obc.ChaincodeDeploymentSpec, uuid string) (*obc.Transaction, error)

    // NewChaincodeExecute is used to execute chaincode's functions
    NewChaincodeExecute(chaincodeInvocation *obc.ChaincodeInvocationSpec, uuid string) (*obc.Transaction, error)

    // NewChaincodeQuery is used to query chaincode's functions
    NewChaincodeQuery(chaincodeInvocation *obc.ChaincodeInvocationSpec, uuid string) (*obc.Transaction, error)
}
```

### Client

The following interface offers a mean to get instances of the previous interfaces.

```
type Client interface {

    ...

    // GetEnrollmentCertHandler returns a CertificateHandler whose certificate is the enrollment certificate
    GetEnrollmentCertificateHandler() (CertificateHandler, error)

    // GetTCertHandlerNext returns a CertificateHandler whose certificate is the next available TCert
    GetTCertificateHandlerNext() (CertificateHandler, error)

    // GetTCertHandlerFromDER returns a CertificateHandler whose certificate is the one passed
    GetTCertificateHandlerFromDER(der []byte) (CertificateHandler, error)

}
```

### Transaction Format

To support application-level ACLs, the fabric's transaction and chaincode specification format have an additional field to store application-specific metadata.
The content of this field is decided by the application. The fabric layer treats it as an unstructured stream of bytes.


```

message ChaincodeSpec {

    ...

    ConfidentialityLevel confidentialityLevel;
    bytes metadata;

    ...
}


message Transaction {
    ...

    bytes payload;
    bytes metadata;

    ...
}
```

Another way to achieve this is to have the payload contain the metadata itself.

### Validators

To assist chaincode execution, the validators provide the chaincode additional information, such as the metadata and the binding.

## Application-level access control

### Deploy Transaction

Alice has full control over the deployment transaction's metadata.
In particular, the metadata can be used to store a list of ACLs (one per function), or a list of roles.
To define each of these lists/roles, Alice can use any TCerts/ECerts of the users who have been
granted that (access control) privilege or have been assigned that role. The latter is done offline.

Now, Alice requires that in order to invoke the *hello* function, a certain message *M* has to be authenticated by an authorized invoker (Bob, in our case).
We distinguish the following two cases:

1. *M* is one of the chaincode's function arguments;
2. *M* is the invocation message itself, i.e., function-name, arguments.

### Execute Transaction

To invoke *hello*, Bob needs to sign *M* using the TCert/ECert Alice has used to name him in the deployment transaction's metadata.
Let's call this certificate CertBob. At this point Bob does the following:   

1. Bob obtains a *CertificateHandler* for CertBob, *cHandlerBob*;
2. Bob obtains a new *TransactionHandler* to issue the execute transaction, *txHandler* relative to his next available TCert or his ECert;
3. Bob obtains *txHandler*'s *binding* by invoking *txHandler.getBinding()*;
4. Bob signs *'M || txBinding'* by invoking *cHandlerBob.Sign('M || txBinding')*, let *signature* be the output of the signing function;
5. Bob issues a new execute transaction by invoking, *txHandler.NewChaincodeExecute(...)*. Now, *signature* can be included
  in the transaction as one of the argument to be passed to the function or as transaction metadata.

### Chaincode Execution

The validators, who receive the execute transaction issued by Bob, will provide to *hello* the following information:

1. The *binding* of the execute transaction;
2. The *metadata* of the execute transaction;
3. The *metadata* of the deploy transaction.

Then, *hello* is responsible for checking that *signature* is indeed a valid signature issued by Bob.
