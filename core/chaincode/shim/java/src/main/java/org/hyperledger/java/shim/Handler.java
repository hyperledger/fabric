/*
Copyright DTCC 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

         http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package org.hyperledger.java.shim;

import com.google.protobuf.ByteString;
import io.grpc.stub.StreamObserver;
import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;
import org.hyperledger.java.fsm.CBDesc;
import org.hyperledger.java.fsm.Event;
import org.hyperledger.java.fsm.EventDesc;
import org.hyperledger.java.fsm.FSM;
import org.hyperledger.java.fsm.exceptions.CancelledException;
import org.hyperledger.java.fsm.exceptions.NoTransitionException;
import org.hyperledger.java.helper.Channel;
import org.hyperledger.protos.Chaincode.*;
import org.hyperledger.protos.Chaincode.ChaincodeMessage.Builder;

import java.util.HashMap;
import java.util.List;
import java.util.Map;

import static org.hyperledger.java.fsm.CallbackType.*;
import static org.hyperledger.protos.Chaincode.ChaincodeMessage.Type.*;

public class Handler {

	private static Log logger = LogFactory.getLog(Handler.class);
	
	private StreamObserver<ChaincodeMessage> chatStream;
	private ChaincodeBase chaincode;

	private Map<String, Boolean> isTransaction;
	private Map<String, Channel<ChaincodeMessage>> responseChannel;
	public Channel<NextStateInfo> nextState; 

	private FSM fsm;

	public Handler(StreamObserver<ChaincodeMessage> chatStream, ChaincodeBase chaincode) {
		this.chatStream = chatStream;
		this.chaincode = chaincode;

		responseChannel = new HashMap<String, Channel<ChaincodeMessage>>();
		isTransaction = new HashMap<String, Boolean>();
		nextState = new Channel<NextStateInfo>();

		fsm = new FSM("created");

		fsm.addEvents(
				//				Event Name				Destination		Sources States
				new EventDesc(REGISTERED.toString(), 	"established",	"created"),
				new EventDesc(INIT.toString(), 			"init", 		"established"),
				new EventDesc(READY.toString(), 		"ready", 		"established"),
				new EventDesc(ERROR.toString(), 		"established", 	"init"),
				new EventDesc(RESPONSE.toString(),		"init", 		"init"),
				new EventDesc(COMPLETED.toString(), 	"ready", 		"init"),
				new EventDesc(TRANSACTION.toString(),	"transaction", 	"ready"),
				new EventDesc(COMPLETED.toString(), 	"ready", 		"transaction"),
				new EventDesc(ERROR.toString(), 		"ready", 		"transaction"),
				new EventDesc(RESPONSE.toString(), 		"transaction", 	"transaction"),
				new EventDesc(QUERY.toString(), 		"transaction", 	"transaction"),
				new EventDesc(QUERY.toString(), 		"ready", 		"ready"),
				new EventDesc(RESPONSE.toString(), 		"ready", 		"ready")
				);

		fsm.addCallbacks(
				//			Type			Trigger					Callback
				new CBDesc(BEFORE_EVENT,	REGISTERED.toString(), 	(event) -> beforeRegistered(event)),
				new CBDesc(AFTER_EVENT, 	RESPONSE.toString(), 	(event) -> afterResponse(event)),
				new CBDesc(AFTER_EVENT, 	ERROR.toString(), 		(event) -> afterError(event)),
				new CBDesc(ENTER_STATE, 	"init", 				(event) -> enterInitState(event)),
				new CBDesc(ENTER_STATE, 	"transaction", 			(event) -> enterTransactionState(event)),
				new CBDesc(BEFORE_EVENT, 	QUERY.toString(), 		(event) -> beforeQuery(event))
				);
	}

	public static String shortID(String uuid) {
		if (uuid.length() < 8) {
			return uuid;
		} else {
			return uuid.substring(0, 8);
		}
	}

	public void triggerNextState(ChaincodeMessage message, boolean send) {
		if(logger.isTraceEnabled())logger.trace("triggerNextState for message "+message);
		nextState.add(new NextStateInfo(message, send));
	}

	public synchronized void serialSend(ChaincodeMessage message) {
		try {
			chatStream.onNext(message);
		} catch (Exception e) {
			logger.error(String.format("[%s]Error sending %s: %s",
					shortID(message), message.getType(), e));
			throw new RuntimeException(String.format("Error sending %s: %s", message.getType(), e));
		}
		if(logger.isTraceEnabled())logger.trace("serialSend complete for message "+message);
	}

	public synchronized Channel<ChaincodeMessage> createChannel(String uuid) {
		if (responseChannel.containsKey(uuid)) {
			throw new IllegalStateException("[" + shortID(uuid) + "] Channel exists");
		}

		Channel<ChaincodeMessage> channel = new Channel<ChaincodeMessage>();
		responseChannel.put(uuid, channel);
		if(logger.isTraceEnabled())logger.trace("channel created with uuid "+uuid);

		return channel;
	}

	public synchronized void sendChannel(ChaincodeMessage message) {
		if (!responseChannel.containsKey(message.getTxid())) {
			throw new IllegalStateException("[" + shortID(message) + "]sendChannel does not exist");
		}

		logger.debug(String.format("[%s]Before send", shortID(message)));
		responseChannel.get(message.getTxid()).add(message);
		logger.debug(String.format("[%s]After send", shortID(message)));
	}

	public ChaincodeMessage receiveChannel(Channel<ChaincodeMessage> channel) {
		try {
			return channel.take();
		} catch (InterruptedException e) {
			logger.debug("channel.take() failed with InterruptedException");
			
			//Channel has been closed?
			//TODO
			return null;
		}	
	}

	public synchronized void deleteChannel(String uuid) {
		Channel<ChaincodeMessage> channel = responseChannel.remove(uuid);
		if (channel != null) {
			channel.close();
		}

		if(logger.isTraceEnabled())logger.trace("deleteChannel done with uuid "+uuid);
	}

	/**
	 * Marks a UUID as either a transaction or a query
	 * @param uuid ID to be marked
	 * @param isTransaction true for transaction, false for query
	 * @return whether or not the UUID was successfully marked
	 */
	public synchronized boolean markIsTransaction(String uuid, boolean isTransaction) {
		if (this.isTransaction == null) {
			return false;
		}

		this.isTransaction.put(uuid, isTransaction);
		return true;
	}

	public synchronized void deleteIsTransaction(String uuid) {
		isTransaction.remove(uuid);
	}

	public void beforeRegistered(Event event) {
		messageHelper(event);
		logger.debug(String.format("Received %s, ready for invocations", REGISTERED));
	}

	/**
	 * Handles requests to initialize chaincode
	 * @param message chaincode to be initialized
	 */
	public void handleInit(ChaincodeMessage message) {
		Runnable task = () -> {
			ChaincodeMessage nextStatemessage = null;
			boolean send = true;
			try {
				// Get the function and args from Payload
				ChaincodeInput input;
				try {
					input = ChaincodeInput.parseFrom(message.getPayload());
				} catch (Exception e) {
					//				payload = []byte(unmarshalErr.Error())
					//				// Send ERROR message to chaincode support and change state
					//				logger.debug(String.format("[%s]Incorrect payload format. Sending %s", shortID(message), ERROR)
					//				nextStatemessage = ChaincodeMessage.newBuilder(){Type: ERROR, Payload: payload, Uuid: message.getTxid()}
					return;
				}

				//			// Mark as a transaction (allow put/del state)
				markIsTransaction(message.getTxid(), true);

				// Create the ChaincodeStub which the chaincode can use to callback
				ChaincodeStub stub = new ChaincodeStub(message.getTxid(), this);

				// Call chaincode's Run
				ByteString result;
				try {
					result = chaincode.runHelper(stub, getFunction(input.getArgsList()), getParameters(input.getArgsList()));
				} catch (Exception e) {
					// Send ERROR message to chaincode support and change state
					logger.debug(String.format("[%s]Init failed. Sending %s", shortID(message), ERROR));
					nextStatemessage = ChaincodeMessage.newBuilder()
							.setType(ERROR)
							.setPayload(ByteString.copyFromUtf8(e.getMessage()))
							.setTxid(message.getTxid())
							.build();
					return;	
				} finally {
					// delete isTransaction entry
					deleteIsTransaction(message.getTxid());
				}

				// Send COMPLETED message to chaincode support and change state
				nextStatemessage = ChaincodeMessage.newBuilder()
						.setType(COMPLETED)
						.setPayload(result)
						.setTxid(message.getTxid())
						.build();

				logger.debug(String.format(String.format("[%s]Init succeeded. Sending %s",
						shortID(message), COMPLETED)));

				//TODO put in all exception states
			} catch (Exception e) {
				throw e;
			} finally {
				triggerNextState(nextStatemessage, send);
			}
		};

		//Run above task
		new Thread(task).start();
	}

	private String getFunction(List<ByteString> args) {
		return (args.size() > 0) ? args.get(0).toStringUtf8() : "";
	}

	private String[] getParameters(List<ByteString> args) {
		int size = (args.size() == 0) ? 0 : args.size() - 1;
		String[] strArgs = new String[size];
		for(int i = 1; i < args.size(); ++i) {
			strArgs[i-1] = args.get(i).toStringUtf8();
		}
		return strArgs;
	}

	// enterInitState will initialize the chaincode if entering init from established.
	public void enterInitState(Event event) {
		logger.debug(String.format("Entered state %s", fsm.current()));
		ChaincodeMessage message = messageHelper(event);
		logger.debug(String.format("[%s]Received %s, initializing chaincode",
				shortID(message), message.getType().toString()));
		if (message.getType() == INIT) {
			// Call the chaincode's Run function to initialize
			handleInit(message);
		}
	}

	//
	// handleTransaction Handles request to execute a transaction.
	public void handleTransaction(ChaincodeMessage message) {
		// The defer followed by triggering a go routine dance is needed to ensure that the previous state transition
		// is completed before the next one is triggered. The previous state transition is deemed complete only when
		// the beforeInit function is exited. Interesting bug fix!!
		Runnable task = () -> {
			//better not be nil
			ChaincodeMessage nextStatemessage = null;
			boolean send = true;

			//Defer
			try {
				// Get the function and args from Payload
				ChaincodeInput input;
				try {
					input = ChaincodeInput.parseFrom(message.getPayload());
				} catch (Exception e) {
					logger.debug(String.format("[%s]Incorrect payload format. Sending %s", shortID(message), ERROR));
					// Send ERROR message to chaincode support and change state
					nextStatemessage = ChaincodeMessage.newBuilder()
							.setType(ERROR)
							.setPayload(message.getPayload())
							.setTxid(message.getTxid())
							.build();
					return;
				}

				// Mark as a transaction (allow put/del state)
				markIsTransaction(message.getTxid(), true);

				// Create the ChaincodeStub which the chaincode can use to callback
				ChaincodeStub stub = new ChaincodeStub(message.getTxid(), this);

				// Call chaincode's Run
				ByteString response;
				try {
					response = chaincode.runHelper(stub, getFunction(input.getArgsList()), getParameters(input.getArgsList()));
				} catch (Exception e) {
					e.printStackTrace();
					System.err.flush();
					// Send ERROR message to chaincode support and change state
					logger.error(String.format("[%s]Error running chaincode. Transaction execution failed. Sending %s",
							shortID(message), ERROR));
					nextStatemessage = ChaincodeMessage.newBuilder()
							.setType(ERROR)
							.setPayload(message.getPayload())
							.setTxid(message.getTxid())
							.build();
					return;
				} finally {
					deleteIsTransaction(message.getTxid());	
				}

				logger.debug(String.format("[%s]Transaction completed. Sending %s",
						shortID(message), COMPLETED));

				// Send COMPLETED message to chaincode support and change state
				Builder builder = ChaincodeMessage.newBuilder()
						.setType(COMPLETED)
						.setTxid(message.getTxid());
				if (response != null) builder.setPayload(response);
				nextStatemessage = builder.build();
			} finally {
				triggerNextState(nextStatemessage, send);
			}
		};

		new Thread(task).start();
	}

	// handleQuery handles request to execute a query.
	public void handleQuery(ChaincodeMessage message) {
		// Query does not transition state. It can happen anytime after Ready
		Runnable task = () -> {
			ChaincodeMessage serialSendMessage = null;
			try {
				// Get the function and args from Payload
				ChaincodeInput input;
				try {
					input = ChaincodeInput.parseFrom(message.getPayload());
				} catch (Exception e) {
					// Send ERROR message to chaincode support and change state
					logger.debug(String.format("[%s]Incorrect payload format. Sending %s",
							shortID(message), QUERY_ERROR));
					serialSendMessage = ChaincodeMessage.newBuilder()
							.setType(QUERY_ERROR)
							.setPayload(ByteString.copyFromUtf8(e.getMessage()))
							.setTxid(message.getTxid())
							.build();
					return;
				}

				// Mark as a query (do not allow put/del state)
				markIsTransaction(message.getTxid(), false);

				// Call chaincode's Query
				// Create the ChaincodeStub which the chaincode can use to callback
				ChaincodeStub stub = new ChaincodeStub(message.getTxid(), this);


				ByteString response;
				try {
					response = chaincode.queryHelper(stub, getFunction(input.getArgsList()), getParameters(input.getArgsList()));
				} catch (Exception e) {
					// Send ERROR message to chaincode support and change state
					logger.debug(String.format("[%s]Query execution failed. Sending %s",
							shortID(message), QUERY_ERROR));
					serialSendMessage = ChaincodeMessage.newBuilder()
							.setType(QUERY_ERROR)
							.setPayload(ByteString.copyFromUtf8(e.getMessage()))
							.setTxid(message.getTxid())
							.build();
					return;
				} finally {
					deleteIsTransaction(message.getTxid());
				}

				// Send COMPLETED message to chaincode support
				logger.debug("["+ shortID(message)+"]Query completed. Sending "+ QUERY_COMPLETED);
				serialSendMessage = ChaincodeMessage.newBuilder()
						.setType(QUERY_COMPLETED)
						.setPayload(response)
						.setTxid(message.getTxid())
						.build();
			} finally {
				serialSend(serialSendMessage);
			}
		};

		new Thread(task).start();
	}

	// enterTransactionState will execute chaincode's Run if coming from a TRANSACTION event.
	public void enterTransactionState(Event event) {
		ChaincodeMessage message = messageHelper(event);
		logger.debug(String.format("[%s]Received %s, invoking transaction on chaincode(src:%s, dst:%s)",
				shortID(message), message.getType().toString(), event.src, event.dst));
		if (message.getType() == TRANSACTION) {
			// Call the chaincode's Run function to invoke transaction
			handleTransaction(message);
		}
	}

	// afterCompleted will need to handle COMPLETED event by sending message to the peer
	public void afterCompleted(Event event) {
		ChaincodeMessage message = messageHelper(event);
		logger.debug(String.format("[%s]sending COMPLETED to validator for tid", shortID(message)));
		try {
			serialSend(message);
		} catch (Exception e) {
			event.cancel(new Exception("send COMPLETED failed %s", e));
		}
	}

	// beforeQuery is invoked when a query message is received from the validator
	public void beforeQuery(Event event) {
		ChaincodeMessage message = messageHelper(event);
		handleQuery(message);
	}

	// afterResponse is called to deliver a response or error to the chaincode stub.
	public void afterResponse(Event event) {
		ChaincodeMessage message = messageHelper(event);
		try {
			sendChannel(message);
			logger.debug(String.format("[%s]Received %s, communicated (state:%s)",
					shortID(message), message.getType(), fsm.current()));
		} catch (Exception e) {
			logger.error(String.format("[%s]error sending %s (state:%s): %s", shortID(message),
					message.getType(), fsm.current(), e));
		}
	}

	private ChaincodeMessage messageHelper(Event event) {
		try {
			return (ChaincodeMessage) event.args[0];
		} catch (Exception e) {
			RuntimeException error = new RuntimeException("Received unexpected message type");
			event.cancel(error);
			throw error;
		}	
	}

	public void afterError(Event event) {
		ChaincodeMessage message = messageHelper(event);
		/* TODO- revisit. This may no longer be needed with the serialized/streamlined messaging model
		 * There are two situations in which the ERROR event can be triggered:
		 * 1. When an error is encountered within handleInit or handleTransaction - some issue at the chaincode side; In this case there will be no responseChannel and the message has been sent to the validator.
		 * 2. The chaincode has initiated a request (get/put/del state) to the validator and is expecting a response on the responseChannel; If ERROR is received from validator, this needs to be notified on the responseChannel.
		 */
		try {
			sendChannel(message);
		} catch (Exception e) {
			logger.debug(String.format("[%s]Error received from validator %s, communicated(state:%s)",
					shortID(message), message.getType(), fsm.current()));
		}
	}

	// handleGetState communicates with the validator to fetch the requested state information from the ledger.
	public ByteString handleGetState(String key, String uuid) {
		try {
			//TODO Implement method to get and put entire state map and not one key at a time?
			// Create the channel on which to communicate the response from validating peer
			// Create the channel on which to communicate the response from validating peer
			Channel<ChaincodeMessage> responseChannel;
			try {
				responseChannel = createChannel(uuid);
			} catch (Exception e) {
				logger.debug("Another state request pending for this Uuid. Cannot process.");
				throw e;
			}

			// Send GET_STATE message to validator chaincode support
			ChaincodeMessage message = ChaincodeMessage.newBuilder()
					.setType(GET_STATE)
					.setPayload(ByteString.copyFromUtf8(key))
					.setTxid(uuid)
					.build();

			logger.debug(String.format("[%s]Sending %s", shortID(message), GET_STATE));
			try {
				serialSend(message);
			} catch (Exception e) {
				logger.error(String.format("[%s]error sending GET_STATE %s", shortID(uuid), e));
				throw new RuntimeException("could not send message");
			}

			// Wait on responseChannel for response
			ChaincodeMessage response;
			try {
				response = receiveChannel(responseChannel);
			} catch (Exception e) {
				logger.error(String.format("[%s]Received unexpected message type", shortID(uuid)));
				throw new RuntimeException("Received unexpected message type");
			}

			// Success response
			if (response.getType() == RESPONSE) {
				logger.debug(String.format("[%s]GetState received payload %s", shortID(response.getTxid()), RESPONSE));
				return response.getPayload();
			}

			// Error response
			if (response.getType() == ERROR) {
				logger.error(String.format("[%s]GetState received error %s", shortID(response.getTxid()), ERROR));
				throw new RuntimeException(response.getPayload().toString());
			}

			// Incorrect chaincode message received
			logger.error(String.format("[%s]Incorrect chaincode message %s received. Expecting %s or %s",
					shortID(response.getTxid()), response.getType(), RESPONSE, ERROR));
			throw new RuntimeException("Incorrect chaincode message received");
		} finally {
			deleteChannel(uuid);
		}
	}

	private boolean isTransaction(String uuid) {
		return isTransaction.containsKey(uuid) && isTransaction.get(uuid);
	}

	public void handlePutState(String key, ByteString value, String uuid) {
		// Check if this is a transaction
		logger.debug("["+ shortID(uuid)+"]Inside putstate (\""+key+"\":\""+value+"\"), isTransaction = "+isTransaction(uuid));

		if (!isTransaction(uuid)) {
			throw new IllegalStateException("Cannot put state in query context");
		}

		PutStateInfo payload = PutStateInfo.newBuilder()
				.setKey(key)
				.setValue(value)
				.build();

		// Create the channel on which to communicate the response from validating peer
		Channel<ChaincodeMessage> responseChannel;
		try {
			responseChannel = createChannel(uuid);
		} catch (Exception e) {
			logger.error(String.format("[%s]Another state request pending for this Uuid. Cannot process.", shortID(uuid)));
			throw e;
		}

		//Defer
		try {
			// Send PUT_STATE message to validator chaincode support
			ChaincodeMessage message = ChaincodeMessage.newBuilder()
					.setType(PUT_STATE)
					.setPayload(payload.toByteString())
					.setTxid(uuid)
					.build();

			logger.debug(String.format("[%s]Sending %s", shortID(message), PUT_STATE));

			try {
				serialSend(message);
			} catch (Exception e) {
				logger.error(String.format("[%s]error sending PUT_STATE %s", message.getTxid(), e));				
				throw new RuntimeException("could not send message");
			}

			// Wait on responseChannel for response
			ChaincodeMessage response;
			try {
				response = receiveChannel(responseChannel);
			} catch (Exception e) {
				//TODO figure out how to get uuid of receive channel
				logger.error(String.format("[%s]Received unexpected message type", e));
				throw e;
			}

			// Success response
			if (response.getType() == RESPONSE) {
				logger.debug(String.format("[%s]Received %s. Successfully updated state", shortID(response.getTxid()), RESPONSE));
				return;
			}

			// Error response
			if (response.getType() == ERROR) {
				logger.error(String.format("[%s]Received %s. Payload: %s", shortID(response.getTxid()), ERROR, response.getPayload()));
				throw new RuntimeException(response.getPayload().toStringUtf8());
			}

			// Incorrect chaincode message received
			logger.error(String.format("[%s]Incorrect chaincode message %s received. Expecting %s or %s",
					shortID(response.getTxid()), response.getType(), RESPONSE, ERROR));

			throw new RuntimeException("Incorrect chaincode message received");
		} catch (Exception e) {
			throw e;
		} finally {
			deleteChannel(uuid);
		}
	}

	public void handleDeleteState(String key, String uuid) {
		// Check if this is a transaction
		if (!isTransaction(uuid)) {
			throw new RuntimeException("Cannot del state in query context");
		}

		// Create the channel on which to communicate the response from validating peer
		Channel<ChaincodeMessage> responseChannel;
		try {
			responseChannel = createChannel(uuid);
		} catch (Exception e) {
			logger.error(String.format("[%s]Another state request pending for this Uuid."
					+ " Cannot process create createChannel.", shortID(uuid)));
			throw e;
		}

		//Defer
		try {
			// Send DEL_STATE message to validator chaincode support
			ChaincodeMessage message = ChaincodeMessage.newBuilder()
					.setType(DEL_STATE)
					.setPayload(ByteString.copyFromUtf8(key))
					.setTxid(uuid)
					.build();
			logger.debug(String.format("[%s]Sending %s", shortID(uuid), DEL_STATE));
			try {
				serialSend(message);
			} catch (Exception e) {
				logger.error(String.format("[%s]error sending DEL_STATE %s", shortID(message), DEL_STATE));
				throw new RuntimeException("could not send message");
			}

			// Wait on responseChannel for response
			ChaincodeMessage response;
			try {
				response = receiveChannel(responseChannel);
			} catch (Exception e) {
				logger.error(String.format("[%s]Received unexpected message type", shortID(message)));
				throw new RuntimeException("Received unexpected message type");
			}

			if (response.getType() == RESPONSE) {
				// Success response
				logger.debug(String.format("[%s]Received %s. Successfully deleted state", message.getTxid(), RESPONSE));
				return;
			}

			if (response.getType() == ERROR) {
				// Error response
				logger.error(String.format("[%s]Received %s. Payload: %s", message.getTxid(), ERROR, response.getPayload()));
				throw new RuntimeException(response.getPayload().toStringUtf8());
			}

			// Incorrect chaincode message received
			logger.error(String.format("[%s]Incorrect chaincode message %s received. Expecting %s or %s",
					shortID(response.getTxid()), response.getType(), RESPONSE, ERROR));
			throw new RuntimeException("Incorrect chaincode message received");
		} finally {
			deleteChannel(uuid);
		}
	}

	public RangeQueryStateResponse handleRangeQueryState(String startKey, String endKey, String uuid) {
		// Create the channel on which to communicate the response from validating peer
		Channel<ChaincodeMessage> responseChannel;
		try {
			responseChannel = createChannel(uuid);
		} catch (Exception e) {
			logger.debug(String.format("[%s]Another state request pending for this Uuid."
					+ " Cannot process.", shortID(uuid)));
			throw e;
		}

		//Defer
		try {
			// Send RANGE_QUERY_STATE message to validator chaincode support
			RangeQueryState payload = RangeQueryState.newBuilder()
					.setStartKey(startKey)
					.setEndKey(endKey)
					.build();

			ChaincodeMessage message = ChaincodeMessage.newBuilder()
					.setType(RANGE_QUERY_STATE)
					.setPayload(payload.toByteString())
					.setTxid(uuid)
					.build();

			logger.debug(String.format("[%s]Sending %s", shortID(message), RANGE_QUERY_STATE));
			try {
				serialSend(message);
			} catch (Exception e){
				logger.error(String.format("[%s]error sending %s", shortID(message), RANGE_QUERY_STATE));
				throw new RuntimeException("could not send message");
			}

			// Wait on responseChannel for response
			ChaincodeMessage response;
			try {
				response = receiveChannel(responseChannel);
			} catch (Exception e) {
				logger.error(String.format("[%s]Received unexpected message type", uuid));
				throw new RuntimeException("Received unexpected message type");
			}

			if (response.getType() == RESPONSE) {
				// Success response
				logger.debug(String.format("[%s]Received %s. Successfully got range",
						shortID(response.getTxid()), RESPONSE));

				RangeQueryStateResponse rangeQueryResponse;
				try {
					rangeQueryResponse = RangeQueryStateResponse.parseFrom(response.getPayload());
				} catch (Exception e) {
					logger.error(String.format("[%s]unmarshall error", shortID(response.getTxid())));
					throw new RuntimeException("Error unmarshalling RangeQueryStateResponse.");
				}

				return rangeQueryResponse;
			}

			if (response.getType() == ERROR) {
				// Error response
				logger.error(String.format("[%s]Received %s",
						shortID(response.getTxid()), ERROR));
				throw new RuntimeException(response.getPayload().toStringUtf8());
			}

			// Incorrect chaincode message received
			logger.error(String.format("Incorrect chaincode message %s recieved. Expecting %s or %s",
					response.getType(), RESPONSE, ERROR));
			throw new RuntimeException("Incorrect chaincode message received");
		} finally {
			deleteChannel(uuid);
		}
	}

	public ByteString handleInvokeChaincode(String chaincodeName, String function, List<ByteString> args, String uuid) {
		// Check if this is a transaction
		if (!isTransaction.containsKey(uuid)) {
			throw new RuntimeException("Cannot invoke chaincode in query context");
		}

		ChaincodeID id = ChaincodeID.newBuilder()
				.setName(chaincodeName).build();
		ChaincodeInput input = ChaincodeInput.newBuilder()
				.addArgs(ByteString.copyFromUtf8(function))
				.addAllArgs(args)
				.build();
		ChaincodeSpec payload = ChaincodeSpec.newBuilder()
				.setChaincodeID(id)
				.setCtorMsg(input)
				.build();

		// Create the channel on which to communicate the response from validating peer
		Channel<ChaincodeMessage> responseChannel;
		try {
			responseChannel = createChannel(uuid);
		} catch (Exception e) {
			logger.error(String.format("[%s]Another state request pending for this Uuid. Cannot process.", shortID(uuid)));
			throw e;
		}

		//Defer
		try {
			// Send INVOKE_CHAINCODE message to validator chaincode support
			ChaincodeMessage message = ChaincodeMessage.newBuilder()
					.setType(INVOKE_CHAINCODE)
					.setPayload(payload.toByteString())
					.setTxid(uuid)
					.build();

			logger.debug(String.format("[%s]Sending %s",
					shortID(message), INVOKE_CHAINCODE));

			try {
				serialSend(message);
			} catch (Exception e) {
				logger.error("["+ shortID(message)+"]Error sending "+INVOKE_CHAINCODE+": "+e.getMessage());
				throw e;
			}

			// Wait on responseChannel for response
			ChaincodeMessage response;
			try {
				response = receiveChannel(responseChannel);
			} catch (Exception e) {
				logger.error(String.format("[%s]Received unexpected message type", shortID(message)));
				throw new RuntimeException("Received unexpected message type");
			}

			if (response.getType() == RESPONSE) {
				// Success response
				logger.debug(String.format("[%s]Received %s. Successfully invoked chaincode", shortID(response.getTxid()), RESPONSE));
				return response.getPayload();
			}

			if (response.getType() == ERROR) {
				// Error response
				logger.error(String.format("[%s]Received %s.", shortID(response.getTxid()), ERROR));
				throw new RuntimeException(response.getPayload().toStringUtf8());
			}

			// Incorrect chaincode message received
			logger.debug(String.format("[%s]Incorrect chaincode message %s received. Expecting %s or %s",
					shortID(response.getTxid()), response.getType(), RESPONSE, ERROR));
			throw new RuntimeException("Incorrect chaincode message received");
		} finally {
			deleteChannel(uuid);
		}
	}

	public ByteString handleQueryChaincode(String chaincodeName, String function, List<ByteString> args, String uuid) {
		ChaincodeID id = ChaincodeID.newBuilder().setName(chaincodeName).build();
		ChaincodeInput input = ChaincodeInput.newBuilder()
				.addArgs(ByteString.copyFromUtf8(function))
				.addAllArgs(args)
				.build();
		ChaincodeSpec payload = ChaincodeSpec.newBuilder()
				.setChaincodeID(id)
				.setCtorMsg(input)
				.build();

		// Create the channel on which to communicate the response from validating peer
		Channel<ChaincodeMessage> responseChannel;
		try {
			responseChannel = createChannel(uuid);
		} catch (Exception e) {
			logger.debug(String.format("Another request pending for this Uuid. Cannot process."));
			throw e;
		}

		//Defer
		try {

			// Send INVOKE_QUERY message to validator chaincode support
			ChaincodeMessage message = ChaincodeMessage.newBuilder()
					.setType(INVOKE_QUERY)
					.setPayload(payload.toByteString())
					.setTxid(uuid)
					.build();

			logger.debug(String.format("[%s]Sending %s", shortID(message), INVOKE_QUERY));

			try {
				serialSend(message);
			} catch (Exception e) {
				logger.error(String.format("[%s]error sending %s", shortID(message), INVOKE_QUERY));
				throw new RuntimeException("could not send message");
			}

			// Wait on responseChannel for response
			ChaincodeMessage response;
			try {
				response = receiveChannel(responseChannel);
			} catch (Exception e) {				
				logger.error(String.format("[%s]Received unexpected message type", shortID(message)));
				throw new RuntimeException("Received unexpected message type");
			}

			if (response.getType() == RESPONSE) {
				// Success response
				logger.debug(String.format("[%s]Received %s. Successfully queried chaincode",
						shortID(response.getTxid()), RESPONSE));
				return response.getPayload();
			}

			if (response.getType() == ERROR) {
				// Error response
				logger.error(String.format("[%s]Received %s.",
						shortID(response.getTxid()), ERROR));
				throw new RuntimeException(response.getPayload().toStringUtf8());
			}

			// Incorrect chaincode message received
			logger.error(String.format("[%s]Incorrect chaincode message %s recieved. Expecting %s or %s",
					shortID(response.getTxid()), response.getType(), RESPONSE, ERROR));
			throw new RuntimeException("Incorrect chaincode message received");
		} finally {
			deleteChannel(uuid);
		}
	}

	// handleMessage message handles loop for org.hyperledger.java.shim side of chaincode/validator stream.
	public synchronized void handleMessage(ChaincodeMessage message) throws Exception {

		if (message.getType() == ChaincodeMessage.Type.KEEPALIVE){
			logger.debug(String.format("[%s] Recieved KEEPALIVE message, do nothing",
					shortID(message)));
			// Received a keep alive message, we don't do anything with it for now
			// and it does not touch the state machine
				return;
		}

		logger.debug(String.format("[%s]Handling ChaincodeMessage of type: %s(state:%s)",
				shortID(message), message.getType(), fsm.current()));

		if (fsm.eventCannotOccur(message.getType().toString())) {
			String errStr = String.format("[%s]Chaincode handler org.hyperledger.java.fsm cannot handle message (%s) with payload size (%d) while in state: %s",
					message.getTxid(), message.getType(), message.getPayload().size(), fsm.current());
			ByteString payload = ByteString.copyFromUtf8(errStr);
			ChaincodeMessage errormessage = ChaincodeMessage.newBuilder()
					.setType(ERROR)
					.setPayload(payload)
					.setTxid(message.getTxid())
					.build();
			serialSend(errormessage);
			throw new RuntimeException(errStr);
		}

		// Filter errors to allow NoTransitionError and CanceledError 
		// to not propagate for cases where embedded Err == nil.
		try {
			fsm.raiseEvent(message.getType().toString(), message);
		} catch (NoTransitionException e) {
			if (e.error != null) throw e;
			logger.debug("["+ shortID(message)+"]Ignoring NoTransitionError");
		} catch (CancelledException e) {
			if (e.error != null) throw e;
			logger.debug("["+ shortID(message)+"]Ignoring CanceledError");
		}
	}
	
	private String shortID(ChaincodeMessage message) {
		return shortID(message.getTxid());
	}
	
}
