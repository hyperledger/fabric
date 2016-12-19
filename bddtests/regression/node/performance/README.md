## Performance test engine, scripts, and supporting files
Overview.

## SETUP

## USAGE


### Examples


### Examples - Daily Performance Tests:

Stress tests: example02 chaincode, node, gRPC

1. send invokes to all 4 peers for 3 min, each thread sends invokes to each of
   the 4 peers
2. send queries to all 4 peers for 3 min, each thread sends queries to each of
   the 4 peers

Concurrency tests: auction chaincode, node, gRPC

1. send 4000 concurrnt invokes with 1kb-2kb random payload, each thread sends
   1000 invokes to each of the 4 peers
2. send 4000 concurrnt queries, each thread sends 1000 queries to each of
   the 4 peers

### Examples - Long Run Performance Tests:

1. send 1 invoke with 1kb-2kb random payload per second use auction chaincode
   for 72 hours
2. still to add another test: mix (invoke followed by query)...

