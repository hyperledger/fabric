### NO-OP system chaincode
NO-OP is a system chaincode that does nothing when invoked. The parameters of the invoke transaction are stored on the ledger so it is possible to encode arbitrary data into them.

#### Functions and valid options
- Invoke transactions have to be called with *'execute'* as function name and at least one argument. Only the **first argument** is used. Note that it should be **encoded with BASE64**.
- Only one type of query is supported: *'getTran'* (passed as a function name). GetTran has to get a transaction ID as argument in hexadecimal format. The function looks up the corresponding transaction's (if any) **first argument** and tries to **decode it as a BASE64 encoded string**.

#### Testing
NO-OP has unit tests checking invocation and queries using proper/improper arguments. The chaincode implementation provides a facility for mocking the ledger under the chaincode (*mockLedgerH* in struct *chaincode.SystemChaincode*). This should only be used for testing as it is dangerous to rely on global variables in memory that can hold state across invokes.
