Metrics Reference
=================

Prometheus Metrics
------------------

The following metrics are currently exported for consumption by Prometheus.

+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| Name                                                | Type      | Description                                                | Labels             |
+=====================================================+===========+============================================================+====================+
| blockcutter_block_fill_duration                     | histogram | The time from first transaction enqueing to the block      | channel            |
|                                                     |           | being cut in seconds.                                      |                    |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| broadcast_enqueue_duration                          | histogram | The time to enqueue a transaction in seconds.              | channel            |
|                                                     |           |                                                            | type               |
|                                                     |           |                                                            | status             |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| broadcast_processed_count                           | counter   | The number of transactions processed.                      | channel            |
|                                                     |           |                                                            | type               |
|                                                     |           |                                                            | status             |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| broadcast_validate_duration                         | histogram | The time to validate a transaction in seconds.             | channel            |
|                                                     |           |                                                            | type               |
|                                                     |           |                                                            | status             |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| chaincode_execute_timeouts                          | counter   | The number of chaincode executions (Init or Invoke) that   | chaincode          |
|                                                     |           | have timed out.                                            |                    |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| chaincode_launch_duration                           | histogram | The time to launch a chaincode.                            | chaincode          |
|                                                     |           |                                                            | success            |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| chaincode_launch_failures                           | counter   | The number of chaincode launches that have failed.         | chaincode          |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| chaincode_launch_timeouts                           | counter   | The number of chaincode launches that have timed out.      | chaincode          |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| chaincode_shim_request_duration                     | histogram | The time to complete chaincode shim requests.              | type               |
|                                                     |           |                                                            | channel            |
|                                                     |           |                                                            | chaincode          |
|                                                     |           |                                                            | success            |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| chaincode_shim_requests_completed                   | counter   | The number of chaincode shim requests completed.           | type               |
|                                                     |           |                                                            | channel            |
|                                                     |           |                                                            | chaincode          |
|                                                     |           |                                                            | success            |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| chaincode_shim_requests_received                    | counter   | The number of chaincode shim requests received.            | type               |
|                                                     |           |                                                            | channel            |
|                                                     |           |                                                            | chaincode          |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| consensus_kafka_batch_size                          | gauge     | The mean batch size in bytes sent to topics.               | topic              |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| consensus_kafka_compression_ratio                   | gauge     | The mean compression ratio (as percentage) for topics.     | topic              |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| consensus_kafka_incoming_byte_rate                  | gauge     | Bytes/second read off brokers.                             | broker_id          |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| consensus_kafka_outgoing_byte_rate                  | gauge     | Bytes/second written to brokers.                           | broker_id          |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| consensus_kafka_record_send_rate                    | gauge     | The number of records per second sent to topics.           | topic              |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| consensus_kafka_records_per_request                 | gauge     | The mean number of records sent per request to topics.     | topic              |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| consensus_kafka_request_latency                     | gauge     | The mean request latency in ms to brokers.                 | broker_id          |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| consensus_kafka_request_rate                        | gauge     | Requests/second sent to brokers.                           | broker_id          |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| consensus_kafka_request_size                        | gauge     | The mean request size in bytes to brokers.                 | broker_id          |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| consensus_kafka_response_rate                       | gauge     | Requests/second sent to brokers.                           | broker_id          |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| consensus_kafka_response_size                       | gauge     | The mean response size in bytes from brokers.              | broker_id          |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| couchdb_processing_time                             | histogram | Time taken in seconds for the function to complete request | database           |
|                                                     |           | to CouchDB                                                 | function_name      |
|                                                     |           |                                                            | result             |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| deliver_blocks_sent                                 | counter   | The number of blocks sent by the deliver service.          | channel            |
|                                                     |           |                                                            | filtered           |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| deliver_requests_completed                          | counter   | The number of deliver requests that have been completed.   | channel            |
|                                                     |           |                                                            | filtered           |
|                                                     |           |                                                            | success            |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| deliver_requests_received                           | counter   | The number of deliver requests that have been received.    | channel            |
|                                                     |           |                                                            | filtered           |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| deliver_streams_closed                              | counter   | The number of GRPC streams that have been closed for the   |                    |
|                                                     |           | deliver service.                                           |                    |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| deliver_streams_opened                              | counter   | The number of GRPC streams that have been opened for the   |                    |
|                                                     |           | deliver service.                                           |                    |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| dockercontroller_chaincode_container_build_duration | histogram | The time to build a chaincode image in seconds.            | chaincode          |
|                                                     |           |                                                            | success            |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| endorser_chaincode_instantiation_failures           | counter   | The number of chaincode instantiations or upgrade that     | channel            |
|                                                     |           | have failed.                                               | chaincode          |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| endorser_duplicate_transaction_failures             | counter   | The number of failed proposals due to duplicate            | channel            |
|                                                     |           | transaction ID.                                            | chaincode          |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| endorser_endorsement_failures                       | counter   | The number of failed endorsements.                         | channel            |
|                                                     |           |                                                            | chaincode          |
|                                                     |           |                                                            | chaincodeerror     |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| endorser_proposal_acl_failures                      | counter   | The number of proposals that failed ACL checks.            | channel            |
|                                                     |           |                                                            | chaincode          |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| endorser_proposal_validation_failures               | counter   | The number of proposals that have failed initial           |                    |
|                                                     |           | validation.                                                |                    |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| endorser_proposals_received                         | counter   | The number of proposals received.                          |                    |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| endorser_propsal_duration                           | histogram | The time to complete a proposal.                           | channel            |
|                                                     |           |                                                            | chaincode          |
|                                                     |           |                                                            | success            |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| endorser_successful_proposals                       | counter   | The number of successful proposals.                        |                    |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| fabric_version                                      | gauge     | The active version of Fabric.                              | version            |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| grpc_comm_conn_closed                               | counter   | gRPC connections closed. Open minus closed is the active   |                    |
|                                                     |           | number of connections.                                     |                    |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| grpc_comm_conn_opened                               | counter   | gRPC connections opened. Open minus closed is the active   |                    |
|                                                     |           | number of connections.                                     |                    |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| grpc_server_stream_messages_received                | counter   | The number of stream messages received.                    | service            |
|                                                     |           |                                                            | method             |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| grpc_server_stream_messages_sent                    | counter   | The number of stream messages sent.                        | service            |
|                                                     |           |                                                            | method             |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| grpc_server_stream_request_duration                 | histogram | The time to complete a stream request.                     | service            |
|                                                     |           |                                                            | method             |
|                                                     |           |                                                            | code               |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| grpc_server_stream_requests_completed               | counter   | The number of stream requests completed.                   | service            |
|                                                     |           |                                                            | method             |
|                                                     |           |                                                            | code               |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| grpc_server_stream_requests_received                | counter   | The number of stream requests received.                    | service            |
|                                                     |           |                                                            | method             |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| grpc_server_unary_request_duration                  | histogram | The time to complete a unary request.                      | service            |
|                                                     |           |                                                            | method             |
|                                                     |           |                                                            | code               |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| grpc_server_unary_requests_completed                | counter   | The number of unary requests completed.                    | service            |
|                                                     |           |                                                            | method             |
|                                                     |           |                                                            | code               |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| grpc_server_unary_requests_received                 | counter   | The number of unary requests received.                     | service            |
|                                                     |           |                                                            | method             |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| ledger_block_processing_time                        | histogram | Time taken in seconds for ledger block processing.         | channel            |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| ledger_blockchain_height                            | gauge     | Height of the chain in blocks.                             | channel            |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| ledger_blockstorage_commit_time                     | histogram | Time taken in seconds for committing the block and private | channel            |
|                                                     |           | data to storage.                                           |                    |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| ledger_statedb_commit_time                          | histogram | Time taken in seconds for committing block changes to      | channel            |
|                                                     |           | state db.                                                  |                    |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| ledger_transaction_count                            | counter   | Number of transactions processed.                          | channel            |
|                                                     |           |                                                            | transaction_type   |
|                                                     |           |                                                            | chaincode          |
|                                                     |           |                                                            | validation_code    |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| logging_entries_checked                             | counter   | Number of log entries checked against the active logging   | level              |
|                                                     |           | level                                                      |                    |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+
| logging_entries_written                             | counter   | Number of log entries that are written                     | level              |
+-----------------------------------------------------+-----------+------------------------------------------------------------+--------------------+


StatsD Metrics
--------------

The following metrics are currently emitted for consumption by StatsD. The
``%{variable_name}`` nomenclature represents segments that vary based on
context.

For example, ``%{channel}`` will be replaced with the name of the channel
associated with the metric.

+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| Bucket                                                                                  | Type      | Description                                                |
+=========================================================================================+===========+============================================================+
| blockcutter.block_fill_duration.%{channel}                                              | histogram | The time from first transaction enqueing to the block      |
|                                                                                         |           | being cut in seconds.                                      |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| broadcast.enqueue_duration.%{channel}.%{type}.%{status}                                 | histogram | The time to enqueue a transaction in seconds.              |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| broadcast.processed_count.%{channel}.%{type}.%{status}                                  | counter   | The number of transactions processed.                      |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| broadcast.validate_duration.%{channel}.%{type}.%{status}                                | histogram | The time to validate a transaction in seconds.             |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| chaincode.execute_timeouts.%{chaincode}                                                 | counter   | The number of chaincode executions (Init or Invoke) that   |
|                                                                                         |           | have timed out.                                            |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| chaincode.launch_duration.%{chaincode}.%{success}                                       | histogram | The time to launch a chaincode.                            |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| chaincode.launch_failures.%{chaincode}                                                  | counter   | The number of chaincode launches that have failed.         |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| chaincode.launch_timeouts.%{chaincode}                                                  | counter   | The number of chaincode launches that have timed out.      |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| chaincode.shim_request_duration.%{type}.%{channel}.%{chaincode}.%{success}              | histogram | The time to complete chaincode shim requests.              |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| chaincode.shim_requests_completed.%{type}.%{channel}.%{chaincode}.%{success}            | counter   | The number of chaincode shim requests completed.           |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| chaincode.shim_requests_received.%{type}.%{channel}.%{chaincode}                        | counter   | The number of chaincode shim requests received.            |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| consensus.kafka.batch_size.%{topic}                                                     | gauge     | The mean batch size in bytes sent to topics.               |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| consensus.kafka.compression_ratio.%{topic}                                              | gauge     | The mean compression ratio (as percentage) for topics.     |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| consensus.kafka.incoming_byte_rate.%{broker_id}                                         | gauge     | Bytes/second read off brokers.                             |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| consensus.kafka.outgoing_byte_rate.%{broker_id}                                         | gauge     | Bytes/second written to brokers.                           |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| consensus.kafka.record_send_rate.%{topic}                                               | gauge     | The number of records per second sent to topics.           |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| consensus.kafka.records_per_request.%{topic}                                            | gauge     | The mean number of records sent per request to topics.     |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| consensus.kafka.request_latency.%{broker_id}                                            | gauge     | The mean request latency in ms to brokers.                 |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| consensus.kafka.request_rate.%{broker_id}                                               | gauge     | Requests/second sent to brokers.                           |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| consensus.kafka.request_size.%{broker_id}                                               | gauge     | The mean request size in bytes to brokers.                 |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| consensus.kafka.response_rate.%{broker_id}                                              | gauge     | Requests/second sent to brokers.                           |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| consensus.kafka.response_size.%{broker_id}                                              | gauge     | The mean response size in bytes from brokers.              |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| couchdb.processing_time.%{database}.%{function_name}.%{result}                          | histogram | Time taken in seconds for the function to complete request |
|                                                                                         |           | to CouchDB                                                 |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| deliver.blocks_sent.%{channel}.%{filtered}                                              | counter   | The number of blocks sent by the deliver service.          |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| deliver.requests_completed.%{channel}.%{filtered}.%{success}                            | counter   | The number of deliver requests that have been completed.   |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| deliver.requests_received.%{channel}.%{filtered}                                        | counter   | The number of deliver requests that have been received.    |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| deliver.streams_closed                                                                  | counter   | The number of GRPC streams that have been closed for the   |
|                                                                                         |           | deliver service.                                           |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| deliver.streams_opened                                                                  | counter   | The number of GRPC streams that have been opened for the   |
|                                                                                         |           | deliver service.                                           |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| dockercontroller.chaincode_container_build_duration.%{chaincode}.%{success}             | histogram | The time to build a chaincode image in seconds.            |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| endorser.chaincode_instantiation_failures.%{channel}.%{chaincode}                       | counter   | The number of chaincode instantiations or upgrade that     |
|                                                                                         |           | have failed.                                               |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| endorser.duplicate_transaction_failures.%{channel}.%{chaincode}                         | counter   | The number of failed proposals due to duplicate            |
|                                                                                         |           | transaction ID.                                            |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| endorser.endorsement_failures.%{channel}.%{chaincode}.%{chaincodeerror}                 | counter   | The number of failed endorsements.                         |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| endorser.proposal_acl_failures.%{channel}.%{chaincode}                                  | counter   | The number of proposals that failed ACL checks.            |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| endorser.proposal_validation_failures                                                   | counter   | The number of proposals that have failed initial           |
|                                                                                         |           | validation.                                                |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| endorser.proposals_received                                                             | counter   | The number of proposals received.                          |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| endorser.propsal_duration.%{channel}.%{chaincode}.%{success}                            | histogram | The time to complete a proposal.                           |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| endorser.successful_proposals                                                           | counter   | The number of successful proposals.                        |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| fabric_version.%{version}                                                               | gauge     | The active version of Fabric.                              |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| grpc.comm.conn_closed                                                                   | counter   | gRPC connections closed. Open minus closed is the active   |
|                                                                                         |           | number of connections.                                     |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| grpc.comm.conn_opened                                                                   | counter   | gRPC connections opened. Open minus closed is the active   |
|                                                                                         |           | number of connections.                                     |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| grpc.server.stream_messages_received.%{service}.%{method}                               | counter   | The number of stream messages received.                    |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| grpc.server.stream_messages_sent.%{service}.%{method}                                   | counter   | The number of stream messages sent.                        |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| grpc.server.stream_request_duration.%{service}.%{method}.%{code}                        | histogram | The time to complete a stream request.                     |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| grpc.server.stream_requests_completed.%{service}.%{method}.%{code}                      | counter   | The number of stream requests completed.                   |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| grpc.server.stream_requests_received.%{service}.%{method}                               | counter   | The number of stream requests received.                    |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| grpc.server.unary_request_duration.%{service}.%{method}.%{code}                         | histogram | The time to complete a unary request.                      |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| grpc.server.unary_requests_completed.%{service}.%{method}.%{code}                       | counter   | The number of unary requests completed.                    |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| grpc.server.unary_requests_received.%{service}.%{method}                                | counter   | The number of unary requests received.                     |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| ledger.block_processing_time.%{channel}                                                 | histogram | Time taken in seconds for ledger block processing.         |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| ledger.blockchain_height.%{channel}                                                     | gauge     | Height of the chain in blocks.                             |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| ledger.blockstorage_commit_time.%{channel}                                              | histogram | Time taken in seconds for committing the block and private |
|                                                                                         |           | data to storage.                                           |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| ledger.statedb_commit_time.%{channel}                                                   | histogram | Time taken in seconds for committing block changes to      |
|                                                                                         |           | state db.                                                  |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| ledger.transaction_count.%{channel}.%{transaction_type}.%{chaincode}.%{validation_code} | counter   | Number of transactions processed.                          |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| logging.entries_checked.%{level}                                                        | counter   | Number of log entries checked against the active logging   |
|                                                                                         |           | level                                                      |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+
| logging.entries_written.%{level}                                                        | counter   | Number of log entries that are written                     |
+-----------------------------------------------------------------------------------------+-----------+------------------------------------------------------------+


.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
