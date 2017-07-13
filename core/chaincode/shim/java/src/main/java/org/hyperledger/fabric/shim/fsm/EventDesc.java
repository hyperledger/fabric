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

/**
 * Represents an event when initializing the FSM.
 * The event can have one or more source states that is valid for performing
 * the transition. If the FSM is in one of the source states it will end up in
 * the specified destination state, calling all defined callbacks as it goes.
 */
public class EventDesc {

		/** The event name used when calling for a transition */
		String name;

		/** A slice of source states that the FSM must be in to perform a state transition */
		String[] src;

		/** The destination state that the FSM will be in if the transition succeeds */
		String dst;
		
		public EventDesc(String name, String[] src, String dst) {
			this.name = name;
			this.src = src;
			this.dst = dst;
		}
		public EventDesc(String name, String src, String dst) {
			this.name = name;
			this.src = new String[] { src };
			this.dst = dst;
		}
}
