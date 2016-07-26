# args.tcl -- A package of handy utilities for 'args' processing

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

############################################################################
# mapKeywordArgs arglist map ?other?

# Standard processing for keyword argument lists

# mapKeywordArgs processes the 'args' argument of Tcl functions (or any other
# Tcl list), binding and/or defaulting variables in the caller's environment
# based on a keyword/default map plus an optional catch-all 'other' argument.
# mapKeywordArgs is designed to provide a full range of features while
# remaining simple and fast enough for the majority of keyword-parameterized
# procedures.

# The keyword/default map is a list of lists. Each sublist defines processing
# for a keyword-style argument.  The valid sublist forms are:

#     {key      <keylist> <var> <default> [<present_var>]}
#     {args     <keylist> <var> <default> [<present_var>]}
#     {bool     <keylist> <var> <default> [<present_var>]}
#     {enum     <keylist> <var> <default> [<present_var>]}
#     {key:req  <keylist> <var> [<default> [<present_var>]]}
#     {args:req <keylist> <var> [<default> [<present_var>]]}
#     {bool:req <keylist> <var> [<default> [<present_var>]]}
#     {enum:req <keylist> <var> [<default> [<present_var>]]}

# Each sublist names a variable <var> to be bound to data extracted or
# inferred from the argument list. If the parameter type is one of the *:req
# forms, then the parameter is required to be specified in one of its
# alternate forms.

# Each <keylist> is either a single keyword or a list of equivalent or
# enumerated keywords based on the parametr type (key, args, bool or enum).
# Keywords typically begin with the '-' character but this is not required.
# Along with a default value, each sublist contains an optional <present_var>
# that will be set to 1 if the keyword appears in the argument list, otherwise
# 0. The <default> and <present_var> are redundant for required parameters,
# but may be specified for consistency.  The <present_var> will always be
# bound to 1 for required parameters.

# Keywords and their values appear in the argument list separated by
# whitespace. Each keyword must be separately specified, i.e., there is no
# support for Unix-style combining of single-character flags. The
# implementation guarantees that each <var> is bound exactly once*, either
# to an explicitly specified or default value.

# The 'key' parameter type binds <var> to the value following the keyword in
# the argument list.  The <keylist> contains one or more equivalent keywords
# to specify the parameter.  Multi-element <keylist> are useful for cases like
# the ecmd::Target object where -core is also allowed as an alias for the more
# generic -chipUnitNum.

# The 'args' parameter type binds <var> to the entire remainder of the
# argument list following the keyword.

# The 'bool' parameter type specifies a Boolean switch.  The <keylist> is
# either a keyword representing a positive value, or a list of two keywords
# where the first is a positive keyword and the seconds is a negative keyword:

#    { ... {bool -print i_print 0} ... }

#    { ... {bool {-check -nocheck} i_check 1 ...}

# The binding rules for 'bool' arguments are as follows:

#     o If a positive keyword is present, <var> is bound to 1
#     o If a negative keyword is present, <var> is bound to 0
#     o Otherwise, <var> is bound to <default>**

# The 'enum' parameter type allows any of the elements of <keylist> to appear
# where a keyword is expected, and binds <var> to the selected member of the
# enumeration.

# If the optional 'other' argument is supplied and is not the empty string,
# then 'other' is taken as the name of a variable in the caller's context to
# be bound to all remaining arguments in the event that an argument is
# encountered that is not an element of any of the <keylist>. If 'other' is
# specified then it will always be bound to a list (which may be empty) of the
# other arguments***. The 'other' argument may also be specified in the style
# of a defaulted Tcl argument, i.e. as a two-element list whose first element
# is the 'other' variable name and whose second element is the default value.

# * Assuming that the agument list specifies one of an equivalent set of
# keywords at most once, a condition which is not checked.  In the event of
# redundant binding specifications the final binding in the argument list will
# prevail.

# ** Which <default> would typically be, but is not required to be either 0
# or 1.

# *** Note that although Tcl does not recoginze any difference between "a" and
# [list a], it does treat {} and [list {}] as distinct.  This means that a
# test of a non-defaulted 'other' variable against {} is sufficient to
# determine if there were in fact any 'other' arguments.
# ############################################################################

proc mapKeywordArgs {i_arglist i_map {i_other {}}} {

    # First crack any 'other' argument

    if {$i_other ne {}} {
        set bound(other) 0
        upvar [first $i_other] other
        set otherDefault [second $i_other]
    }

    # Next handle explicit arguments.  All arguments are first marked as
    # bound.  Keyword-type arguments reenter the loop with an argument type to
    # bind the value.
    
    set type none
    set arglist $i_arglist

    while {$arglist ne {}} {
        
        switch -exact $type {
            
            key {
                set var [first $arglist]
                set type none
            }
            args {
                set type none
                break
            }
            none {
                set index 0
                set type other
                set arg [first $arglist]
                foreach clause $i_map {
                    
                    if {[member $arg [second $clause]]} {
                        
                        upvar 1 [third $clause] var
                        
                        set bound($index) 1
                        
                        set present [fifth $clause]
                        if {$present ne {}} {
                            upvar 1 $present presentVar
                            set presentVar 1
                        }
                        
                        switch -exact [first $clause] {
                            
                            key -
                            key:req {
                                set type key
                            }
                            args -
                            args:req {
                                set type args
                                set var [lrange $arglist 1 end]
                                # Need to exit to outer loop to break
                            }
                            enum -
                            enum:req {
                                set type none
                                set var $arg
                            }
                            bool -
                            bool:req {
                                set type none
                                if {$arg eq [first [second $clause]]} {
                                    set var 1
                                } else {
                                    set var 0
                                }
                            }
                        }
                        break
                    }
                    incr index
                }
                
                if {$type eq "other"} {
                    
                    if {![info exists bound(other)]} {
                        errorExit \
                            "Expecting keyword, found '$arg'. " \
                            "Argument list below:\n$i_arglist\n" \
                            "Argument template below :\n$i_map$"
                    } else {
                        set other $arglist
                        set bound(other) 1
                        set type none
                        break
                    }
                }
                
            }
        }
        
        set arglist [lrange $arglist 1 end]
    }
    
    if {$type ne "none"} {
        errorExit "Expecting a value after keyword '$arg'"
    }
    
    # Finally bind defaults and check for unbound required parameters
    
    set index 0
    foreach clause $i_map {
        
        if {![info exists bound($index)]} {

            if {[string match *:req [first $clause]]} {
                errorExit "Required parameter is missing : [second $clause]"
            }
            
            upvar 1 [third $clause] var
            set var [fourth $clause]
            
            set present [fifth $clause]
            if {$present ne {}} {
                upvar 1 $present presentVar
                set presentVar 0
            }
        }
        incr index
    }
    
    if {[info exists bound(other)] && ($bound(other) == 0)} {
        set other $otherDefault
    }
}


############################################################################
# parms key
# parms key val
# parmsExists key

# Either get ([parms <key>]) or set ([parms <key> <val>]) parameters from a
# global array named ::parms.

array unset ::parms
proc parms {args} {

    switch [llength $args] {
        1 {
            return $::parms($args)
        }
        2 {
            set ::parms([lindex $args 0]) [lindex $args 1]
        }
        default {
            error "BUG : parms{} requires either 1 or 2 arguments"
        }
    }
}

# Does a parameter key exist?
proc parmsExists {i_key} {
    return [info exists ::parms($i_key)]
}

############################################################################
# addPortToHosts i_hosts i_defaultPort

# For processing arguments of the form of lists of hostnames, with a default
# port. If a host does not have an explicit port specified then the
# defaultPort is added.

proc addPortToHosts {i_hosts i_defaultPort} {

    mapeach host $i_hosts {
        if {![string match *:* $host]} {
            return $host:$i_defaultPort
        }
        return $host
    }
}
