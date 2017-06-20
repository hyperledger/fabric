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

package org.hyperledger.fabric.shim.fsm;


/** Holds the info that get passed as a reference in the callbacks */
public class Event {

	// A reference to the parent FSM.
	public final FSM fsm;
	// The event name.
	public final String name;
	// The state before the transition.
	public final String src;
	// The state after the transition.
	public final String dst;
	// An optional error that can be returned from a callback.
	public Exception error = null;

	// An internal flag set if the transition is canceled.
	public boolean cancelled = false;
	// An internal flag set if the transition should be asynchronous
	public boolean async;

	// An optional list of arguments passed to the callback.
	public final Object[] args;

	
	public Event(FSM fsm, String name, String src, String dst,
			Exception error, boolean cancelled, boolean async, Object... args) {
		this.fsm = fsm;
		this.name = name;
		this.src = src;
		this.dst = dst;
		this.error = error;
		this.cancelled = cancelled;
		this.async = async;
		this.args = args;
	}

	/**
	 * Can be called in before_<EVENT> or leave_<STATE> to cancel the 
	 * current transition before it happens. It takes an optional error,
	 * which will overwrite the event's error if it had already been set.
	 */
	public Exception cancel(Exception error) {
		cancelled = true;
		if (error != null) {
			this.error = error;
		}
		return error;
	}

	/**
	 * Can be called in leave_<STATE> to do an asynchronous state transition.
	 * The current state transition will be on hold in the old state until a final
	 * call to Transition is made. This will complete the transition and possibly
	 * call the other callbacks.
	 */
	public void async() {
		async = true;
	}

}