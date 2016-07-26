# lists.tcl

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

# Originally this was an implementation of a small subset of Lisp, with
# Lisp-style names for functions (car, cdr etc.), but that was wierd.  So this
# file now defines a couple of low-level and several higher-level functions
# from Lisp (plus other constructs) but the expectation is that people
# programming with Tcl lists will use the normal Tcl list access
# functions. Tcl does have a struct::list package but this one is simpler,
# more familiar (to a Lisp programmer anyway!) and more general (in some
# cases) and can be used with any garden-variety Tcl installation.

# NB: In Tcl, there's no distinction between a singleton list and an atom.
# That means that the first element of {} is {} (like Lisp), but the first
# element of an atom is the atom, which is an error in Lisp. The Tcl
# specification turns out to simplify things in some cases.

proc atom {x} {
    return [expr {[llength $x] == 1}]
}

proc null {l} {
    return [expr {[llength $l] == 0}]
}

proc first {l} {
    return [lindex $l 0]
}

proc second {l} {
    return [lindex $l 1]
}

proc third {l} {
    return [lindex $l 2]
}

proc fourth {l} {
    return [lindex $l 3]
}

proc fifth {l} {
    return [lindex $l 4]
}

proc sixth {l} {
    return [lindex $l 5]
}

proc seventh {l} {
    return [lindex $l 6]
}

proc eighth {l} {
    return [lindex $l 7]
}

proc rest {l} {
    return [lrange $l 1 end]
}

proc firstn {l n} {
    return [lrange $l 0 [expr {$n - 1}]]
}

proc restn {l n} {
    return [lrange $l $n end]
}

proc last {l} {
    return [lrange $l end end]
}

proc butlast {l {n 1}} {
    return [lrange $l 0 [expr {[llength $l] - (1 + $n)}]]
}

proc member {x l {option -exact}} {
    return [expr {[lsearch $option $l $x] >= 0}]
}

proc assoc {x l} {
    foreach pair $l {
	if {$x == [first $pair]} {
	    return $pair
	}
    }
    return {}
}

proc remove {x l} {
    set newl {}
    for {} {$l != {}} {set l [rest $l]} {
	if {$x != [first $l]} {
	    lappend newl [first $l]
	}
    }
    return $newl
}

proc removeN {x l n} {
    set newl {}
    for {} {($l != {}) && ($n > 0)} {set l [rest $l]} {
        if {$x != [first $l]} {
            lappend newl [first $l]
        } else {
            incr n -1
        }
    }
    return [concat $newl $l]
}

# Order is maintained
proc removeDuplicates {l {option -exact}} {

    set newl {}
    while {$l != {}} {
	set x [first $l]
	set l [rest $l]
	if {![member $x $l $option]} {
	    lappend newl $x
	}
    }
    return $newl
}
    
# Order is not maintained; Exact string comparisons only
proc removeDuplicatesFast {l} {

    foreach x $l {
        set elts($x) {}
    }
    return [array names elts]
}
    
# Order is maintained
proc duplicates {l {option -exact}} {

    set dups {}
    while {$l != {}} {
	set x [first $l]
	set l [rest $l]
	if {[member $x $l $option]} {
	    lappend dups $x
	}
    }
    return [removeDuplicates $dups $option]
}
    
# Order is not maintained; Exact string comparisons only
proc duplicatesFast {l} {

    foreach x $l {
        if {[info exists elts($x)]} {
            set dups($x) {}
        } else {
            set elts($x) {}
        }
    }
    return [array names dups]
}

#  Unlike common Lisp, our setDifference is guaranteed to preserve order.

proc setDifference {l1 l2} {
    set ans {}
    foreach x $l1 {
	if {![member $x $l2]} {
	    lappend ans $x
	}
    }
    return $ans
}

proc reverse {l} {

    set len [llength $l]

    set ans {}
    while {$len} {
	lappend ans [lindex $l [incr len -1]]
    }
    return $ans
}

# Return a new list composed of every Nth element of the original list,
# starting from a given position.

proc everyNth {l n {pos 0}} {

    set newL {}
    while {$pos < [llength $l]} {
        lappend newL [lindex $l $pos]
        incr pos $n
    }
    return $newL
}
    

############################################################################
# countMembers l {array {}}

# An arbitrary list 'l' is converted to a simple histogram by creating an
# array of the unique keys, with values being the count of each key. If an
# array is specified the array is first cleared, then updated, and the return
# value is empty. If no array is specified then an internal array is
# created, and the return value is the result of [array get ..] of the
# internal array, i.e., a list that alternates unique keys and counts.

proc countMembers {l {array {}}} {

    if {[null $array]} {
        array unset a
    } else {
        upvar $array a
    }
    foreach x $l {
        if {[info exists a($x)]} {
            incr a($x)
        } else {
            set a($x) 1
        }
    }
    if {[null $array]} {
        return [array get a]
    } else {
        return
    }
}
    

############################################################################
# range ?start? stop ?step?

# Modeled after the Python function. Plagarized (modified) documentaton:

# This is a versatile function to create lists containing arithmetic
# progressions. It is most often used in foreach and mapeach loops. The
# arguments must be plain integers. If the step argument is omitted, it
# defaults to 1. If the start argument is omitted, it defaults to 0. The
# two-argument form is assumed to specify {start stop}. The full form returns
# a list of plain integers [start, start + step, start + 2 * step, ...]. If
# step is positive, the last element is the largest start + i * step less than
# stop; if step is negative, the last element is the smallest start + i * step
# greater than stop. step must not be zero.
#############################################################################

proc range {args} {
    
    set start 0
    set step 1

    switch -exact [llength $args] {
        1 {set stop $args}
        2 {foreach {start stop} $args break}
        3 {foreach {start stop step} $args break}
        default {error "range requires 1 to 3 arguments "}
    }

    foreach {name val} [list start $start stop $stop step $step] {
        if {![string is integer -strict $val]} {
            error "$name value $val is not an integer"
        }
    }
    if {$step == 0} {
        error "step can't be 0"
    }

    set l {}
    set x $start
    if {$step > 0} {
        while {$x < $stop} {
            lappend l $x
            incr x $step
        }
    } else {
        while {$x > $stop} {
            lappend l $x
            incr x $step
        }
    }
    return $l
}       
      

############################################################################
# enumerate i_spec

# Modeled after eCMD command-line enumeration syntax.  A specification is
# either an integer, a range of integers, or a list of specifications.

# spec := <Int>
# spec := <Int0>..<Int1>; Int0 <= Int1
# spec := <spec0>,<spec1>

proc enumerate {i_spec} {

    if {[llength $i_spec] != 1} {
        error "Non-atomic specification '$i_spec'"
    }
    
    set specs [split $i_spec ,]
    set enumeration {}
    foreach spec $specs {

        if {[regexp {^(.+)\.\.(.+)$} $spec match min max]} {
            if {$min > $max} {
                error "Improperly ordered range specification '$spec'"
            }
            set enumeration \
                [concat $enumeration [range $min [expr {$max + 1}]]]
    
        } else {
            if {![string is integer $spec]} {
                error "Expecting an integer : $spec"
            }
            lappend enumeration $spec
        }
    }
    return $enumeration
}


############################################################################
# mapeach var list script
# mapeach {var0 ... varN} list script
# mapeach varlist0 list0 ... varlistN listN script

# mapeach is an analog of the standard Tcl foreach loop.  While foreach simply
# evaluates the script for side effects, mapeach collects the return values
# from each evaluation of the script and returns this list.  For example

#  mapeach s {Me and Mrs. Jones} {string length $s} ==> {2 3 4 5}

# Normally the value of a script is the value of the final statement of the
# script. mapeach also supports the standard Tcl control constructs return,
# continue and break.  The return statement can be used to immediately return
# a value from the script.  The continue statement causes value accumulation
# to be skipped, and can be used to selectively edit the list.

#  mapeach x {0 1 2 3} {if {$x % 2} {continue} else {return [expr {$x * $x}]}}
#    ==> {0 4}

# If the break statement appears in the script then accumulation terminates
# immediately:

#   mapeach x {0 1 2 3 4} {if {$x > 2} break; set x} ==> {0 1 2}

# Note that similarly to foreach, the mapeach iteration variables are bound in
# the caller's context. For example, after execution of the above, the
# variable 'x' will be bound to 3.
#############################################################################

proc mapeach {args} {

    if {[llength $args] == 3} {

        # Simple form : mapeach vars list script
        
        foreach {vars list script} $args break
        
        if {[llength $vars] == 1} {
            
            # Really simple form : mapeach var list script
            
            upvar 1 $vars x
            set varList x
            
        } else {
            
            # Multi-variable simple form : mapeach {var0 ... varn} list script
            
            set varlist [list]
            set n 0
            foreach var $vars {
                set localVar x$n
                incr n
                upvar 1 $var $localVar
                lappend varList $localVar
            }
        }
        
        set result [list]
        foreach $varList $list {
            set rc [catch {uplevel 1 $script} value]
            switch $rc {
                0 {lappend result $value}
                1 {return -code error -errorinfo $::errorInfo $value}
                2 {lappend result $value}
                3 {break}
                4 {continue}
                default {error "Unexpected return code '$rc'"}
            }
        }
        
    } else {
        
        error "Sorry, complex 'mapeach' not coded yet.  Please feel free!"
        
    }
    
    return $result
}


############################################################################
# reduce o_listVar i_list o_accumVar i_intialVal i_script

# The reduce procedure implements general-purpose list
# reduction. Special-purpose procedures based on the general-purpose reduce
# procedures are also available (c.f. reduce*).

# The procedure operates by first setting the o_accumVar to the i_initialVal.
# Then the procedure iterateso_ listVar over i_list, setting o_accumVar to the
# result of evaluating the script.  For example:

#   reduce x {1 2 3} accum 0 {expr {$x + $accum}} ==> 6

# Normally the value of a script is the value of the final statement of the
# script, however reduce also supports the standard Tcl control constructs
# return, continue and break.  The return statement can be used to immediately
# return a value from the script.  The continue statement causes value
# accumulation to be skipped.  For example, to sum all of the even numbers in
# a list:

#  reduce x {0 1 2 3 4 5} accum 0
#    {if {$x % 2} {continue} else {return [expr {$x + $accum}]}}
#      ==> 6

# If the break statement appears in the script then accumulation terminates
# immediately:

# reduce x {0 1 2 3 4} accum 0 \
    #   {if {$x > 3} break else {return [expr {$x + $accum}]}}
#     ==> 6

# Note that similarly to foreach, the reduce iteration variables are bound in
# the caller's context. For example, after execution of the above, the
# variable 'x' will be bound to 3, and 'accum' will be bound to 6.
############################################################################

proc reduce {o_listVar i_list o_accumVar i_initVal i_script} {
    
    upvar $o_listVar r_listVar $o_accumVar r_accumVar
    
    set r_accumVar $i_initVal
    foreach r_listVar $i_list {
        set catchVal [catch {uplevel 1 $i_script} scriptVal]
        if {($catchVal == 0) || ($catchVal == 2)} {
            # Value result or return
            set r_accumVar $scriptVal
        } else {
            switch -exact $catchVal {
                1 {
                    # error
                    puts $scriptVal
                    return -code error -errorinfo $::errorInfo
                }
                3 {
                    # break
                    break
                }
                4 {
                    # continue
                }
                default {
                    Error "Unexpected return code $scriptVal"
                }
            }
        }
    }
    return $r_accumVar
}


############################################################################
# reduce+ i_list
# reduce* i_list
# reduceMin i_list
# reduceMax i_list

# Special forms of the 'reduce' procedure.  

# - reduce+ reduces i_list using addition from am initial value of 0.

# - reduce* reduces i_list using multiplicationfrom am initial value of 1.

# - reduceMin and reduceMax compute the minimum and maximum values of i_list
#   respectively. These procedures arbitrarily define the minimum and maximum
#   values of the empty list as 0.
############################################################################

proc reduce+ {i_list} {
    reduce x $i_list accum 0 {expr {$x + $accum}}
}

proc reduce* {i_list} {
    reduce x $i_list accum 1 {expr {$x * $accum}}
}

proc reduceMin {i_list} {
    if {[null $i_list]} {
        return 0
    } else {
        return \
            [reduce \ 
             x [rest $i_list] \
                 accum [first $i_list] {
                     math::min $x $accum
                 }]
    }
}


proc reduceMax {i_list} {
    if {[null $i_list]} {
        return 0
    } else {
        return \
            [reduce \
                 x [rest $i_list] \
                 accum [first $i_list] {
                     math::max $x $accum
                 }]
    }
}


############################################################################
# CircularList i_list

# A CircularList is an object that returns elements of a linear list as if the
# list were actually made circular. The CircularList is initialized with a
# linear list, and each call of next{} returns the next element in the
# putative circular list, starting with the initial (index 0) element of the
# original list. Note that in Tcl semantics the "elements" of an empty list
# are the empty string.

catch {CircularList destroy}
oo::class create CircularList {
    variable d_list
    variable d_length
    variable d_i
    constructor {i_list} {
        set d_list $i_list
        set d_length [llength $i_list]
        set d_i 0
    }
    method next {} {
        if {$d_i < $d_length} {
            set elt [lindex $d_list $d_i]
            incr d_i
        } else {
            set elt [lindex $d_list 0]
            set d_i 1
        }
        return $elt
    }
}


############################################################################
# RandomBag i_list

# A RandomBag is an object that returns elements of a bag under various
# randomization schemes. The object is initialized with a list of
# elements. The elements need not be unique, however repeated elements are
# statistically more likely to be picked than unique elements.

# The pick{} method picks one of the elements at (uniform) random; pickList{}
# returns a list of (uniform) random elements. The add{} method adds new
# elements. The remove{} and removeN{} methods remove instances of an
# element. The list{} method returns the current list of elements.

catch {RandomBag destroy}
oo::class create RandomBag {
    variable d_list
    variable d_length

    # Build the bag by inserting the list
    constructor {i_list} {

        set d_list $i_list
        set d_length [llength $d_list]
    }

    # Add an element
    method add {i_elt} {

        lappend d_list $i_elt
        incr d_length
    }

    # Remove all copies of the element from the bag, returning the number of
    # elements removed (or 0 if the element was not in the bag).
    method remove {i_elt} {
        
        set oldLength $d_length
        set d_list [remove $i_elt $d_list]
        set d_length [llength $d_list]
        return [expr {$oldLength - $d_length}]
    }

    # Remove at most N copies of the element, returning the number removed.
    method removeN {i_elt i_n} {

        set oldLength $d_length
        set d_list [removeN $i_elt $d_list $i_n]
        set d_length [llength $d_list]
        return [expr {$oldLength - $d_length}]
    }

    # Pick an element at random
    method pick {} {

        return [lindex $d_list [rand32 $d_length]]
    }

    # Pick a list of elements at random
    method pickList {i_n} {

        set l {}
        for {set i 0} {$i < $i_n} {incr i} {
            lappend l [my pick]
        }
        return $l
    }

    # Return the current list of elements
    method list {} {

        return $d_list
    }
}
