
@muralisrini and @pmullaney

## Introduction

The events framework supports the ability to emit 2 types of audible events, block and custom/chaincode events (of type ChaincodeEvent defined in events.proto). The basic idea is that clients (event consumers) will register for event types (currently "block" or "chaincode") and in the case of chaincode they may specify additional registration criteria namely chaincodeID and eventname. ChaincodeID indentifies the specific chaincode deployment that the client has interest in seeing events from and eventname is a string that the chaincode developer embeds when invoking the chaincode stub's `SetEvent` API. Invoke transactions are currently the only operations that can emit events and each invoke is limited to one event emitted per transaction. It should be noted that there are no “transient” or “ephemeral” events that are not stored on the ledger. Thus, events are as deterministic as any other transactional data on the block chain. This basic infrastructure can be extended for successful

## Tactical design: Use TransactionResult to store events

Add an Event message to TransactionResult

```
/ chaincodeEvent - any event emitted by a transaction
type TransactionResult struct {
	Uuid           string          
	Result         []byte
	ErrorCode      uint32
	Error          string
	ChaincodeEvent *ChaincodeEvent
}
```
Where **ChaincodeEvent** is

```
type ChaincodeEvent struct {
	ChaincodeID string
	TxID        string
	EventName   string
	Payload     []byte
}
```

Where ChaincodeID is the uuid associated with the chaincode, TxID is the transaction that generated the event, EventName is the name given to the event by the chaincode(see `SetEvent`), and Payload is the payload attached to the event by the chaincode. The fabric does not impose a structure or restriction on the Payload although it is good practice to not make it large in size.

SetEvent API on chaincode Shim

```
func (stub *ChaincodeStub) GetState(key string)
func (stub *ChaincodeStub) PutState(key string, value [\]byte)
func (stub *ChaincodeStub) DelState(key string)
…
…
func (stub *ChaincodeStub) SetEvent(name string, payload []byte)
…
…
```

When the transaction is completed, SetEvent will result in the event being added to TransactionResult and commited to ledger. The event will then be posted to all clients registered for that event by the event framework.


##General Event types and their relation to Chaincode Events


Events are associated with event types. Clients register interest in the event types they want to receive events of.
Life-cycle of an Event Type is illustrated by a “block” event

	1. On boot-up peer adds "block" to the supported event types
	2. Clients can register interest in “block” event type with a peer (or multiple peers)
	3. On Block creation Peers post an event to all registered clients
	4. Clients receive the “block” event and process the transactions in the block

Chaincode events add an additional level of registration filtering. Instead of registering for all events of a given event type, chaincode events allow clients to register for a specific event from a specific chaincode. For this first version, for simplicity, we have not implemented wildcard or regex matching on the eventname but that is planned. More information on this in "Client Interface" below.

## Client Interface

The current interface for client to register and then receive events is a gRPC interface with pb messages defined in protos/events.proto. The Register message is a series (`repeated`) of Interest messages. Each Interest represents a single event registration.

```
message Interest {
    EventType eventType = 1;
    //Ideally we should just have the following oneof for different
    //Reg types and get rid of EventType. But this is an API change
    //Additional Reg types may add messages specific to their type
    //to the oneof.
    oneof RegInfo {
        ChaincodeReg chaincodeRegInfo = 2;
    }
}

//ChaincodeReg is used for registering chaincode Interests
//when EventType is CHAINCODE
message ChaincodeReg {
    string chaincodeID = 1;
    string eventName = 2;
    ~~bool anyTransaction = 3;~~ //TO BE REMOVED
}

//Register is sent by consumers for registering events
//string type - "register"
message Register {
    repeated Interest events = 1;
}

```

As mentioned in previous section, clients should register for chaincode events using `ChaincodeReg` message where `chaincodeID` refers to the ID of chaincode as returned by the deploy transaction and `eventName` refers to the name of the event posted by the chaincode.  Setting `eventName` with empty string (or "*") will cause all events from a chaincode to be sent to the listener.

There is a service defined in events.proto with a single method that receives a stream of registration events and returns an stream that the client can use to read events from as they occur.

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
s
