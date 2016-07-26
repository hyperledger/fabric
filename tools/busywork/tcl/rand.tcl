# random.tcl -- Random number utilities

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

# Seed the 32-bit random number generator

proc rand32Seed {i_seed} {
    math::rand32Seed $i_seed
}

# Pick a random 32-bit unsigned integer from 0 to range - 1.  If range is 0,
# then the result is a random 32-bit pattern

proc rand32 {range} {

    return [math::rand32 $range]
}

# Pick an element of a list at random with equal probability.

proc randomFromList {l} {

    if {[null $l]} {
	error "The list argument 'l' is NULL"
    }

    lindex $l [rand32 [llength $l]]
}

# Pick an element from a uniform probability map.  This is a list of the form:

#  {{<frac0> <pick0>} ... {<fracn> <pickn>}}

# Where <fraci> is the fraction of the uniform distribution associated with
# each pick : <fraci> / (<frac0> + ... + <fracn>).  For example, in the simple
# case where the <fraci> are percentages they would all add up to 100 (or 1),
# but there is no restriction that the <fraci> add up to 1.

proc randomFromMap {map} {

    set r [expr {rand()}]

    set unity 0.0
    foreach clause $map {
	foreach {frac pick} $clause break
	set unity [expr {$unity + $frac}]
    }

    set cdf 0.0
    foreach clause $map {
	foreach {frac pick} $clause break
	set cdf [expr {$cdf + ($frac / $unity)}]
	if {$r <= $cdf} {
	    return $pick
	}
    }

    #  Rounding error?
    
    return [second [last $map]]
}
