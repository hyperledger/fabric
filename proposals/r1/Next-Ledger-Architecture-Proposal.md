**Draft** / **Work in Progress**

This page documents a proposal for a future ledger architecture based on community feedback. All input is welcome as the goal is to make this a community effort.

##### Table of Contents  
[Motivation](#motivation)  
[API](#api)  
[Point-in-Time Queries](#pointintime)  
[Query Language](#querylanguage)


## <a name="motivation"></a> Motivation
The motivation for exploring a new ledger architecture is based on community feedback. While the existing ledger is able to support some (but not all) of the below requirements, we wanted to explore what a new ledger would look like given all that has been learned. Based on many discussions in the community over Slack, GitHub, and the face to face hackathons, it is clear that there is a strong desire to support the following requirements:

1. Point in time queries - The ability to query chaincode state at previous blocks and easily trace lineage **without** replaying transactions
2. SQL like query language
3. Privacy - The complete ledger may not reside on all committers
4. Cryptographically secure ledger - Data integrity without consulting other nodes
5. Support for consensus algorithms that provides immediate finality like PBFT
6. Support consensus algorithms that require stochastic convergence like PoW, PoET
7. Pruning - Ability to remove old transaction data as needed.
8. Support separation of endorsement from consensus as described in the [Next Consensus Architecture Proposal](https://github.com/hyperledger/fabric/wiki/Next-Consensus-Architecture-Proposal). This implies that some peers may apply endorsed results to their ledger **without** executing transactions or viewing chaincode logic.
9. API / Enginer separation. The ability to plug in different storage engines as needed.

<a name="api"></a>
## API

Proposed API in Go pseudocode

```
package ledger

import "github.com/hyperledger/fabric/protos/peer"

// Encryptor is an interface that a ledger implementation can use for Encrypt/Decrypt the chaincode state
type Encryptor interface {
	Encrypt([]byte) []byte
	Decrypt([]byte) []byte
}

// PeerMgmt is an interface that a ledger implementation expects from peer implementation
type PeerMgmt interface {
	// IsPeerEndorserFor returns 'true' if the peer is endorser for given chaincodeID
	IsPeerEndorserFor(chaincodeID string) bool

	// ListEndorsingChaincodes return the chaincodeIDs for which the peer acts as one of the endorsers
	ListEndorsingChaincodes() []string

	// GetEncryptor returns the Encryptor for the given chaincodeID
	GetEncryptor(chaincodeID string) (Encryptor, error)
}

// In the case of a confidential chaincode, the simulation results from ledger are expected to be encrypted using the 'Encryptor' corresponding to the chaincode.
// Similarly, the blocks returned by the GetBlock(s) method of the ledger are expected to have the state updates in the encrypted form.
// However, internally, the ledger can maintain the latest and historical state for the chaincodes for which the peer is one of the endorsers - in plain text form.
// TODO - Is this assumption correct?

// General purpose interface for forcing a data element to be serializable/de-serializable
type DataHolder interface {
	GetData() interface{}
	GetBytes() []byte
	DecodeBytes(b []byte) interface{}
}

type SimulationResults interface {
	DataHolder
}

type QueryResult interface {
	DataHolder
}

type BlockHeader struct {
}

type PrunePolicy interface {
}

type BlockRangePrunePolicy struct {
	FirstBlockHash string
	LastBlockHash  string
}

// QueryExecutor executes the queries
// Get* methods are for supporting KV-based data model. ExecuteQuery method is for supporting a rich datamodel and query support
//
// ExecuteQuery method in the case of a rich data model is expected to support queries on
// latest state, historical state and on the intersection of state and transactions
type QueryExecutor interface {
	GetState(key string) ([]byte, error)
	GetStateRangeScanIterator(startKey string, endKey string) (ResultsIterator, error)
	GetStateMultipleKeys(keys []string) ([][]byte, error)
	GetTransactionsForKey(key string) (ResultsIterator, error)

	ExecuteQuery(query string) (ResultsIterator, error)
}

// TxSimulator simulates a transaction on a consistent snapshot of the as recent state as possible
type TxSimulator interface {
	QueryExecutor
	StartNewTx()

	// KV data model
	SetState(key string, value []byte)
	DeleteState(key string)
	SetStateMultipleKeys(kvs map[string][]byte)

	// for supporting rich data model (see comments on QueryExecutor above)
	ExecuteUpdate(query string)

	// This can be a large payload
	CopyState(sourceChaincodeID string) error

	// GetTxSimulationResults encapsulates the results of the transaction simulation.
	// This should contain enough detail for
	// - The update in the chaincode state that would be caused if the transaction is to be committed
	// - The environment in which the transaction is executed so as to be able to decide the validity of the enviroment
	//   (at a later time on a different peer) during committing the transactions
	// Different ledger implementation (or configurations of a single implementation) may want to represent the above two pieces
	// of information in different way in order to support different data-models or optimize the information representations.
	// TODO detailed illustration of a couple of representations.
	GetTxSimulationResults() SimulationResults
	HasConflicts() bool
	Clear()
}

type ResultsIterator interface {
	// Next moves to next key-value. Returns true if next key-value exists
	Next() bool
	// GetKeyValue returns next key-value
	GetResult() QueryResult
	// Close releases resources occupied by the iterator
	Close()
}

// OrdererLedger implements methods required by 'orderer ledger'
type OrdererLedger interface {
	Ledger
	// CommitBlock adds a new block
	CommitBlock(block *common.Block) error
}

// PeerLedger differs from the OrdererLedger in that PeerLedger locally maintain a bitmask
// that tells apart valid transactions from invalid ones
type PeerLedger interface {
	Ledger
	// GetTransactionByID retrieves a transaction by id
	GetTransactionByID(txID string) (*pb.Transaction, error)
	// GetBlockByHash returns a block given it's hash
	GetBlockByHash(blockHash []byte) (*common.Block, error)
	// NewTxSimulator gives handle to a transaction simulator.
	// A client can obtain more than one 'TxSimulator's for parallel execution.
	// Any snapshoting/synchronization should be performed at the implementation level if required
	NewTxSimulator() (TxSimulator, error)
	// NewQueryExecutor gives handle to a query executor.
	// A client can obtain more than one 'QueryExecutor's for parallel execution.
	// Any synchronization should be performed at the implementation level if required
	NewQueryExecutor() (QueryExecutor, error)
	// NewHistoryQueryExecutor gives handle to a history query executor.
	// A client can obtain more than one 'HistoryQueryExecutor's for parallel execution.
	// Any synchronization should be performed at the implementation level if required
	NewHistoryQueryExecutor() (HistoryQueryExecutor, error)
	// Commits block into the ledger
	Commit(block *common.Block) error
	//Prune prunes the blocks/transactions that satisfy the given policy
	Prune(policy PrunePolicy) error
}

// ValidatedLedger represents the 'final ledger' after filtering out invalid transactions from PeerLedger.
// Post-v1
type ValidatedLedger interface {
	Ledger
}

// Ledger captures the methods that are common across the 'PeerLedger', 'OrdererLedger', and 'ValidatedLedger'
type Ledger interface {
	// GetBlockchainInfo returns basic info about blockchain
	GetBlockchainInfo() (*pb.BlockchainInfo, error)
	// GetBlockByNumber returns block at a given height
	// blockNumber of  math.MaxUint64 will return last block
	GetBlockByNumber(blockNumber uint64) (*common.Block, error)
	// GetBlocksIterator returns an iterator that starts from `startBlockNumber`(inclusive).
	// The iterator is a blocking iterator i.e., it blocks till the next block gets available in the ledger
	// ResultsIterator contains type BlockHolder
	GetBlocksIterator(startBlockNumber uint64) (ResultsIterator, error)
	// Close closes the ledger
	Close()
}

//BlockChain represents an instance of a block chain. In the case of a consensus algorithm that could cause a fork, an instance of BlockChain
// represent one of the forks (i.e., one of the chains starting from the genesis block to the one of the top most blocks)
type BlockChain interface {
	GetTopBlockHash() string
	GetBlockchainInfo() (*protos.BlockchainInfo, error)
	GetBlockHeaders(startingBlockHash, endingBlockHash string) []*BlockHeader
	GetBlocks(startingBlockHash, endingBlockHash string) []*protos.Block
	GetBlockByNumber(blockNumber uint64) *protos.Block
	GetBlocksByNumber(startingBlockNumber, endingBlockNumber uint64) []*protos.Block
	GetBlockchainSize() uint64
	VerifyChain(highBlock, lowBlock uint64) (uint64, error)
}
```

# Engine specific thoughts

<a name="pointintime"></a>
### Point-in-Time Queries
In abstract temporal terms, there are three varieties of query important to chaincode and application developers:

1. Retrieve the most recent value of a key. (type: current; ex. How much money is in Alice's account?)
2. Retrieve the value of a key at a specific time. (type: historical; ex. What was Alice's account balance at the end of last month?)
3. Retrieve all values of a key over time. (type: lineage; ex. Produce a statement listing all of Alice's transactions.)

When formulating a query, a developer will benefit from the ability to filter, project, and relate transactions to one-another. Consider the following examples:

1. Simple Filtering: Find all accounts that fell below a balance of $100 in the last month.
2. Complex Filtering: Find all of Trudy's transactions that occurred in Iraq or Syria where the amount is above a threshold and the other party has a name that matches a regular expression.
3. Relating: Determine if Alice has ever bought from the same gas station more than once in the same day. Feed this information into a fraud detection model.
4. Projection: Retrieve the city, state, country, and amount of Alice's last ten transactions. This information will be fed into a risk/fraud detection model.

<a name="querylanguage"></a>
### Query Language
Developing a query language to support such a diverse range of queries will not be simple. The challenges are:

1. Scaling the query language with developers as their needs grow. To date, the requests from developers have been modest. As the Hyperledger project's user base grows, so will the query complexity.
2. There are two nearly disjoint classes of query:
    1. Find a single value matching a set of constraints. Amenable to existing SQL and NoSQL grammars.
    2. Find a chain or chains of transactions satisfying a set of constraints. Amenable to graph query languages, such as Neo4J's Cypher or SPARQL.

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
s
