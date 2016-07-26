# Counters

**counters** is a simple but flexible Go language chaincode designed to
  support basic testing of Hyperledger fabric features. The state managed by
  **counters** is a set of named arrays of 64-bit unsigned counters. Arrays
  have fixed lengths, but each named array may have a different length, from
  an array of a single counter up to the largest array representable. Arrays
  are created with a 0 count in all counters. A named array can be recreated
  an unlimited number of times with different lengths during a test.
  
  Methods are provided to increment and decrement the sets of counters. All
  counters in a named set will all have the same value when stored. Arrays of
  different sizes are supported simply to exercise the hashing and storage
  primitives of the blockchain.
  
  64-bit unsigned overflow on increment is considered an error (and
  congratulations to anyone with enough patience to observe this
  happen). Underflow on decrement is also considered to be an error. Suitably
  constructed drivers can potentially use the underflow error to observe
  failures in transaction ordering.
  
  The [Makefile](Makefile) defines standardized tests that use the included 
  [driver](driver) script to exercise the chaincode.

  **NOTE**: This chaincode is only defined and supported for environments in
    which Go integers are defined as 8-byte integers.
	
  
## Init Methods

### `parms ?... <parm> ..?`

The `parms` method is required to be invoked exactly once, namely during the
initial deployment of the chaincode. No other INIT methods are currently
defined.  This method sets global parameters that apply to the invocation of
subsequent methods. 

All parameter specifications are optional and have suitable defaults.  All
parameter key names have an initial "dash". Keys are followed by 0 or more
values, interpreted as detailed for the individual keys below.

- `-id <id>` : The `<id>` is a string used to identify a deployment. Because
  of the way deployed chaincodes are identified, deploying with different IDs
  allows multiple instances of the **counters** chaincode to be active
  simultaneously. The default ID is the empty string. Note that the ID of
  individual instances of the chaincode can be later changed with the `parms`
  query.
  
- `-loggingLevel <level>` : Set the logging level for the chaincode
  logger. The `<level>` is a case-insensitive string chosen from DEBUG, INFO,
  NOTICE, WARNING, ERROR or CRITICAL, in increasing order of severity. If an
  unrecognized or illegal `<level>` is given, the logging level will be set to
  ERROR and execution will continue. If `<level>` is supplied as the string
  "default" the default level will remain in force. The default level for
  chaincode logging is INFO.

- `-shimLoggingLevel <level>` : Set the logging level for the chaincode *shim*
  interface. See the `-loggingLevel` option for details on the `<level>`
  argument. The default level for *shim* logging is WARNING.
  
Examples:

    parms -id cc0 -loggingLevel debug


## Invoke Methods

### `create ?... <name> <length> ...?`

The `create` method creates 0 or more named arrays of fixed lengths, and zeros
all of the counters in the array. Each `<name>` is a string. Each `<length>`
is a 64-bit unsigned integer greater than 0. Lengths can be entered using
either decimal or hex notation (0x...). A named array can be recreated with
different lengths any number of times during a run.  The argument list is
unstructured, that is it always contains an even number of elements. Examples:

	create a 1
	create a 1 b 10 c 100
	
Note that in the event of errors in command parsing, none of the counter
arrays are (re-)created. It is not considered an error to specify the same
array multiple times in one command. The array will be simply be recreated
multiple times in left-to-right order.
	

### `decrement ?... <name> ...?`

Decrement 0 or more counter arrays named in the argument list. Each counter in
each array is decremented, and the new array value is stored back into the
state. It is an error if any `<name>` has not been previosuly created. 64-bit
underflow is considered an error. Examples:

    decrement a
	decrement a b c
	
By design, the arrays are all brought into memory, checked for validity,
decremented, and then all arrays are re-written to the state. Any errors in
command processing or validity checking result in no changes to the state,
however errors from the fabric leave an unpredictable state. It is legal for
an array to be named multiple times, in which case it will be decremented once
for each occurrence, without rewriting the array back to the state between
decrements.
	
	
### `increment ?... <name> ...?`

Increment 0 or more counter arrays named in the argument list. Each counter in
each array is incremented, and the new array value is stored back into the
state. It is an error if any `<name>` has not been previosuly created. 64-bit
overflow is considered an error. Examples:

    increment a
	increment a b c
	
By design, the arrays are all brought into memory, checked for validity,
incremented, and then all arrays are re-written to the state. Any errors in
command processing or validity checking result in no changes to the state,
however errors from the fabric leave an unpredictable state. It is legal for
an array to be named multiple times, in which case it will be incremented once
for each occurrence, without rewriting the array back to the state between
increments.
	

## Query Methods

### `status ?... <name> ...?`

Return the status of 0 or more counter arrays. The status is returned as a
single string. Each `<name>` in the argument list generates the string
representations of 4 unsigned 64-bit values (decimal format, blank
separated):

    ... <expectedLength> <actualLength> <expectedCount> <actualCount> ...
	
The *expected* values are the values expected by the chaincode based on its
internal bookkeeping. The *actual* values are obtained from the blockchain
state database. The `<actualCount>` is the actual value of the first element
of the array.

Examples:

    status a      # Assume length 10 and count 100
	--> "10 10 100 100"
	
	status a b c  # Assume lengths 1, 2, 3 and counts 10, 20, 30
	--> "1 1 10 10 2 2 20 20 3 3 30 30"

It is an error to name an array that has not been created. 

The `<expectedLength>` is obtained from the database. This query *does not*
signal an error if the expected and actual values do not match. The
`<expectedCount>` and `<actualCount>` will currently always be equal, and
correspond to the value held in the first element of the array. The
implementation of state transfer does not allow the chaincode to independently
track the expected counter value, so correctness checking of the count will
need to be performed by the environment.

### `parms ?... <parm> ...?`

This query is used to uniquely re-parameterize individual instances of the
chaincode. The new/modified parameters will only apply to the chaincode
associated with the peer that handles the query. Only the following parameters
are allowed to be modified at runtime:

- `-id <id>`

Examples:

    -id cc0x
	
The return value of this query is an empty string.
	
### `ping`

This query always succeeds by returning the chaincode ID, as most recently set
by the `parms` invocation or query.


  
  
