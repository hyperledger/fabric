# time.tcl Time-related utilities

# Copyright IBM Corp. 2016. All Rights Reserved.
# 
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# 
# 		 http://www.apache.org/licenses/LICENSE-2.0
# 
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

#  A timestamp

proc timestamp {{format -slashplus}} {
    
    switch -exact -- $format {

	-slashplus {
	    clock format [clock seconds] -format %Y/%m/%d+%H:%M:%S
	}
	-numeric {
	    clock format [clock seconds] -format %Y%m%d%H%M%S
	}
        -hybrid {
            clock format [clock seconds] -format %Y%m%d+%H:%M:%S
        }
	default {error "Unrecognized format ==> '$format'"}
    }
}


# A time duration is specified as an integer or float value concatenated with
# a case-insensitive unit specification, defaulting to milliseconds. Units are
# 'ms', 's', 'm' and 'h' for milliseconds, seconds, minutes and hours
# respectively.

proc durationToMs {i_duration} {

    set duration [string tolower $i_duration]

    set base $duration
    set multiplier 1000;        # Default seconds
    if {[regexp {(.*)ms$} $duration match base]} {
        set multiplier 1
    } elseif {[regexp {(.*)s$} $duration match base]} {
        set multiplier 1000
    } elseif {[regexp {(.*)m$} $duration match base]} {
        set multiplier [expr {60 * 1000}]
    } elseif {[regexp {(.*)h$} $duration match base]} {
        set multiplier [expr {60 * 60 * 1000}]
    } else {
        set multiplier 1
    }

    if {[string is integer -strict $base]} {
        return [expr {$base * $multiplier}]
    } elseif {[string is double -strict $base]} {
        return [expr {int($base * $multiplier)}]
    } else {
        errorExit "The putative time duration '$i_duration' '$base' does not parse"
    }
}

proc durationToSeconds {i_duration} {
    return [expr [durationToMs $i_duration] / 1000]
}

proc durationToIntegerSeconds {i_duration} {
    return [expr int([durationToMs $i_duration] / 1000)]
}

# waitFor i_timeout i_script {i_poll}

# This is a busy-wait polling loop that returns 0 if the i_script evaluates
# non-0 before the timeout, otherwise returns -1 in the event of a
# timeout. Specify -1 as the timeout to wait forever. Note that the test is
# always guaranteed to be made at least one time, even for very short
# timeouts. The i_poll argument specifies the polling interval and defaults to
# 0, that is, continuous polling.

proc waitFor {i_timeout i_script {i_poll 0}} {

    set timeout [durationToMs $i_timeout]
    set poll [durationToMs $i_poll]

    if {$timeout < 0} {
        while {![uplevel $i_script]} {
            if {$poll != 0} {after $poll}
        }
        return 0
    }

    set start [clock clicks -milliseconds]
    set timedOut 0
    while {1} {
        if {[uplevel $i_script]} {return 0}
        if {$timedOut} {return -1}
        if {$poll != 0} {after $poll}
        set now [clock clicks -milliseconds]
        set timedOut [expr {($now - $start) >= $timeout}]
    }
}
