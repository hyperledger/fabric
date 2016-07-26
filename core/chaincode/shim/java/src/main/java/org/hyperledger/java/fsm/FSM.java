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

package org.hyperledger.java.fsm;

import java.util.HashMap;
import java.util.HashSet;

import org.hyperledger.java.fsm.exceptions.AsyncException;
import org.hyperledger.java.fsm.exceptions.CancelledException;
import org.hyperledger.java.fsm.exceptions.InTrasistionException;
import org.hyperledger.java.fsm.exceptions.InvalidEventException;
import org.hyperledger.java.fsm.exceptions.NoTransitionException;
import org.hyperledger.java.fsm.exceptions.NotInTransitionException;
import org.hyperledger.java.fsm.exceptions.UnknownEventException;

public class FSM {

	/** The current state of the FSM */
	private String current;

	/** Maps events and sources states to destination states */
	private final HashMap<EventKey, String> transitions;

	/** Maps events and triggers to callback functions */
	private final HashMap<CallbackKey, Callback> callbacks;

	/** The internal transaction function used either directly
	 * or when transition is called in an asynchronous state transition. */
	protected Runnable transition;

	/** Calls the FSM's transition function */
	private Transitioner transitioner;
	
	private final HashSet<String> allStates;
	private final HashSet<String> allEvents;


	// NewFSM constructs a FSM from events and callbacks.
	//
	// The events and transitions are specified as a slice of Event structs
	// specified as Events. Each Event is mapped to one or more internal
	// transitions from Event.Src to Event.Dst.
	//
	// Callbacks are added as a map specified as Callbacks where the key is parsed
	// as the callback event as follows, and called in the same order:
	//
	// 1. before_<EVENT> - called before event named <EVENT>
	//
	// 2. before_event - called before all events
	//
	// 3. leave_<OLD_STATE> - called before leaving <OLD_STATE>
	//
	// 4. leave_state - called before leaving all states
	//
	// 5. enter_<NEW_STATE> - called after eftering <NEW_STATE>
	//
	// 6. enter_state - called after entering all states
	//
	// 7. after_<EVENT> - called after event named <EVENT>
	//
	// 8. after_event - called after all events
	//
	// There are also two short form versions for the most commonly used callbacks.
	// They are simply the name of the event or state:
	//
	// 1. <NEW_STATE> - called after entering <NEW_STATE>
	//
	// 2. <EVENT> - called after event named <EVENT>
	//

	public FSM(String initialState) {
		current = initialState;
		transitioner = new Transitioner();

		transitions = new HashMap<EventKey, String>();
		callbacks = new HashMap<CallbackKey, Callback>();
	
		allEvents = new HashSet<String>();
		allStates = new HashSet<String>();
	}
	
	/** Returns the current state of the FSM */
	public String current() {
		return current;
	}

	/** Returns whether or not the given state is the current state */
	public boolean isCurrentState(String state) {
		return state.equals(current);
	}

	/** Returns whether or not the given event can occur in the current state */
	public boolean eventCanOccur(String eventName) {
		return transitions.containsKey(new EventKey(eventName, current));
	}
	
	/** Returns whether or not the given event can occur in the current state */
	public boolean eventCannotOccur(String eventName) {
		return !eventCanOccur(eventName);
	}

	/** Initiates a state transition with the named event.
	 * The call takes a variable number of arguments
	 * that will be passed to the callback, if defined.
	 * 
	 * It  if the state change is ok or one of these errors:
	 *  - event X inappropriate because previous transition did not complete
	 *  - event X inappropriate in current state Y
	 *  - event X does not exist
	 *  - internal error on state transition
	 * @throws InTrasistionException 
	 * @throws InvalidEventException 
	 * @throws UnknownEventException 
	 * @throws NoTransitionException 
	 * @throws AsyncException 
	 * @throws CancelledException 
	 * @throws NotInTransitionException 
	 */
	public void raiseEvent(String eventName, Object... args)
			throws InTrasistionException, InvalidEventException,
			UnknownEventException, NoTransitionException, CancelledException,
			AsyncException, NotInTransitionException {
		
		if (transition != null) throw new InTrasistionException(eventName);

		String dst = transitions.get(new EventKey(eventName, current));
		if (dst == null) {
			for (EventKey key : transitions.keySet()) {
				if (key.event.equals(eventName)) {
					throw new InvalidEventException(eventName, current);
				}
			}
			throw new UnknownEventException(eventName);
		}

		Event event = new Event(this, eventName, current, dst, null, false, false, args);
		callCallbacks(event, CallbackType.BEFORE_EVENT);

		if (current.equals(dst)) {
			callCallbacks(event, CallbackType.AFTER_EVENT);
			throw new NoTransitionException(event.error);
		}

		// Setup the transition, call it later.
		transition = () -> {
			current = dst;
			try {
				callCallbacks(event, CallbackType.ENTER_STATE);
				callCallbacks(event, CallbackType.AFTER_EVENT);
			} catch (Exception e) {
				throw new InternalError(e);
			}
		};

		callCallbacks(event, CallbackType.LEAVE_STATE);

		// Perform the rest of the transition, if not asynchronous.
		transition();
	}

	// Transition wraps transitioner.transition.
	public void transition() throws NotInTransitionException {
		transitioner.transition(this);
	}


	/** Calls the callbacks of type 'type'; first the named then the general version. 
	 * @throws CancelledException 
	 * @throws AsyncException */
	public void callCallbacks(Event event, CallbackType type) throws CancelledException, AsyncException {
		String trigger = event.name;
		if (type == CallbackType.LEAVE_STATE) trigger = event.src;
		else if (type == CallbackType.ENTER_STATE) trigger = event.dst;
		
		Callback[] callbacks = new Callback[] {
				this.callbacks.get(new CallbackKey(trigger, type)),	//Primary
				this.callbacks.get(new CallbackKey("", type)),			//General
		};

		for (Callback callback : callbacks) {
			if (callback != null) {
				callback.run(event);
				if (type == CallbackType.LEAVE_STATE) {
					if (event.cancelled) {
						transition = null;
						throw new CancelledException(event.error);
					} else if (event.async) {
						throw new AsyncException(event.error);
					}
				} else if (type == CallbackType.BEFORE_EVENT) {
					if (event.cancelled) {
						throw new CancelledException(event.error);
					}
				}
			}
		}
	}

	public void addEvents(EventDesc... events) {
		// Build transition map and store sets of all events and states.
		for (EventDesc event : events) {
			for (String src : event.src) {
				transitions.put(new EventKey(event.name, src), event.dst);
				allStates.add(src);
			}
			allStates.add(event.dst);
			allEvents.add(event.name);
		}
	}


	public void addCallbacks(CBDesc... descs) {
		for (CBDesc desc : descs) { 
			callbacks.put(new CallbackKey(desc.trigger, desc.type), desc.callback);
		}
	}
}
