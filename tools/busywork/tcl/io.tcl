# io.tcl -- Handy utilities for I/O, formatting and other junk.

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


#  This is a form of the Tcl append command that (re-)creates a string
#  variable by appending the arguments.

proc appendx {var args} {

    upvar $var s

    set s {}
    eval append s $args
}


# Tcl does not support C-style implicit concatenation of strings, making it a
# pain to enter long messages.  This is an extended form of 'puts' that
# appends its arguments, allowing nicely formatted code where text lines are
# limited in length, e.g., to 80 columns.  Unlike normal 'puts' a non-default
# stream is given as a -stream option. So the final form of the command is

#      putsx [-nonewline] [-stream <stream>] ... args ...

proc putsx {args} {

    set nonewline 0
    set stream stdout

    while {1} {
	if {[lindex $args 0] eq "-nonewline"} {
	    set nonewline 1
	    set args [lrange $args 1 end]
	    continue
	}
	if {[lindex $args 0] eq "-stream"} {
	    set stream [lindex $args 1]
	    set args [lrange $args 2 end]
	    continue
	}
	break
    }

    eval appendx s $args
    if {$nonewline} {
	puts -nonewline $stream $s
    } else {
	puts $stream $s
    }
    
    return {}
}


#  This is a form of the 'error' command that takes multiple strings

proc errorx {args} {

    eval appendx s $args
    error $s
}


# This is a more friendly error exit for scripts, where users may be confused
# by the Tcl backtraces. The input strings are concatenated and written to
# stderr, then we {exit 1} to signal script failure.

proc errorExit {args} {

    if {[null $args]} {
        err err Aborting
    } else {
        eval err err $args
    }
    exit 1
}

# Machine word formatting

proc hex8 {x {format -0x}} {
    
    switch -exact -- $format {

	-0x {return [format 0x%02x $x]}
	-x  {return [format %02x $x]}
	default {error "Illegal format '$format'"}
    }
}


proc hex16 {x {format -0x}} {
    
    switch -exact -- $format {

	-0x {return [format 0x%04x $x]}
	-x  {return [format %04x $x]}
	default {error "Illegal format '$format'"}
    }
}


proc hex32 {x {format -0x}} {
    
    switch -exact -- $format {

	-0x {return [format 0x%08x $x]}
	-x  {return [format %08x $x]}
	default {error "Illegal format '$format'"}
    }
}


proc hex64 {x {format -0x}} {

    switch -exact -- $format {

	-0x {return [format 0x%016lx $x]}
	-x  {return [format %016lx $x]}
	default {error "Illegal format '$format'"}
    }
}


#  Byte reverse values

proc byterev32 {data} {

    set newdata [expr {\
			   (($data & 0xff)       << 24) | \
			   (($data & 0xff00)     << 8)  | \
			   (($data & 0xff0000)   >> 8)  | \
			   (($data & 0xff000000) >> 24)}]

    return [hex32 $newdata]
}


# Unabbreviate an integer in the form
#
# <n>K, <n>M, <n>G
#
# <n> can be integer or float.  The optional mode (decimal/binary) determines
# whether K = 1000 or 1024 resp., etc.  The result is always a wide int.

proc unabbreviate_int {n {mode decimal}} {

    if {![regexp {^([0-9.]+)([kKmMgG])?$} $n match int pow]} {
	error "Huh? ==> '$n'"
    }

    switch -exact $mode {
	
	decimal {
	    switch -exact $pow {
		{}    {set fac 1}
		k - K {set fac 1000}
		m - M {set fac 1000000}
		g - G {set fac 1000000000}
	    }
	}
	binary {
	    switch -exact $pow {
		{}    {set fac 1}
		k - K {set fac 1024}
		m - M {set fac [expr 1024 * 1024]}
		g - G {set fac [expr 1024 * 1024 * 1024]}
	    }
	}
	default {error "'mode' must be either binary or decimal"}
    }

    return [expr {round($int * wide($fac))}]
}


#  Remove any package designator from the window name of a widget.  Why?
#  Because even though an IWidget may have a qualified name, the only valid
#  window names begin with a period.  In other words, $this may yield ::.foo,
#  but you can't pack ::.foo, only .foo.

proc window_name {x} {
    namespace tail $x
}


# Why oh why did we crash!

proc why {{msg {}}} {

    puts $::errorInfo
    if {$msg ne {}} {
	puts $msg
    }
}


#  'exec' with all output to stdout

proc execout {args} {
    eval exec $args >&@stdout
}


# 'Camelize' a name by eliminating underscores in favor of Capitalizing
# words. The optional argument i_capitalize applies to the final return
# value.

proc camelize {i_name {i_capitalize 0}} {

    set words [mapeach x [split $i_name _] {string tolower $x}]

    if {$i_capitalize} {
        set word1 [string totitle [first $words]]
    } else {
        set word1 [first $words]
    }
    
    set words [mapeach x [rest $words] {string totitle $x}]
    
    return $word1[join $words {}]
}


############################################################################
# ? i_expr i_true i_false

# A 3-way conditional in Tcl. The 'i_expr' is evaluated in the context of
# 'expr', and require a Boolean result from that evaluation.

#     [? {1 < 0} true false]            --> false

proc ? {i_expr i_true i_false} {
    if {[expr $i_expr]} {
        return $i_true
    } else {
        return $i_false
    }
}


############################################################################
# NonblockingGets

# The NonblockingGets object correctly handles asynchronous 'gets' to
# nonblocking streams. There is an issue when 'gets' is used to "follow" a
# file as it is written. 'gets' in this case may return a partial line and
# also signal EOF. This is unexpected, but makes sense in some ways. This
# object correctly handles this by not declaring a line until 'gets' succeeds
# without being blocked or EOF. Caveat: If the file does not end with a
# newline, then this object will never return the last line of the file.

# In use, call the gets{} method to run 'gets'. If the result is 1, then
# calling line{} returns the line. If 0 is returned then there is no (entire)
# line available yet.

oo::class create NonblockingGets {

    variable d_partialLine
    variable d_line
    variable d_channel

    constructor {i_channel} {

        set d_channel $i_channel
        set d_partialLine {}
    }

    method gets {} {

        gets $d_channel line
        set d_line $d_partialLine$line
        if {[eof $d_channel] || [fblocked $d_channel]} {
            set d_partialLine $d_line
            set d_line {}
            return 0
        }
        set d_partialLine {}
        return 1
    }

    method line {} {

        return $d_line
    }
}


############################################################################
# httpTokenDump i_token {i_level err} {i_module {}}
# httpErrorExit i_token

# It can be helpful for error diagnosis to dump the entire contents of an HTTP
# token when an error is encountered. That's what these routines do.

proc httpTokenDump {i_token {i_level err} {i_module {}}} {

    package require http

    $i_level $i_module "Full HTTP token dump"
    $i_level $i_module "HTTP 'ncode' = '[http::ncode $i_token]'"
    foreach {k v} [array get $i_token] {
        $i_level $i_module "    $k $v"
    }
}
                    
proc httpErrorExit {i_token} {

    httpTokenDump $i_token
    http::cleanup $i_token
    errorExit
}


############################################################################
# quote i_l

# 'quote' wraps each element of i_l in "" and returns the new list *as a
# string*. 

proc quote {i_l} {
    set ans ""
    set space ""
    foreach x $i_l {
        set ans "$ans$space\"$x\""
        set space " "
    }
    return $ans
}
