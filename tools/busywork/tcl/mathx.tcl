# mathx -- Add new functions in the namespace of the standard math library

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

# The LCG parameterizations used here are taken from:

# Pierre L'Ecuyer, Tables of Linear Congruential Generators of Different
# Sizes and Good Lattice Structure, Mathemetics of Computation, 68 (225),
# January 1999, pp. 245-260.

namespace eval math {expr 0}

proc math::evenp {x} {
    return [expr {$x % 2 == 0}]
}

proc math::oddp {x} {
    return [expr {$x % 2 != 0}]
}

# urandom - a 32-bit random number obtained from /dev/urandom

proc math::urandom32 {} {

    set urandom [open /dev/urandom]
    fconfigure $urandom -translation binary
    binary scan [read $urandom 4] I rand
    close $urandom

    return [expr {wide($rand) & 0xffffffff}]
}

set math::_urand32_seed [math::urandom32]

# urand32 - A 32-bit linear conguential PRNG seeded by /dev/urandom.  
# If the argument is 0 a full 32-bit random number is returned, otherwise a
# random integer from 0..i_limit-1 is returned.

# This random number sequence will be different every time the application is
# run.

proc math::urand32 {i_limit} {

    set math::_urand32_seed \
        [expr {(($math::_urand32_seed * 32310901) + 1) & 0xffffffff}]

    if {$i_limit == 0} {
        return $math::_urand32_seed
    } else {
        return [expr {(wide($math::_urand32_seed) * $i_limit) >> 32}]
    }
}
    

# rand32 - A 32-bit linear conguential PRNG seeded with a fixed seed.  
# If the argument is 0 a full 32-bit random number is returned, otherwise a
# random integer from 0..i_limit-1 is returned.

# This random number sequence will be identical every time the application is
# run with the same seed.

set math::_rand32_seed 0x1140078758

proc math::rand32Seed {i_seed} {
    set math::_rand32_seed $i_seed
}

proc math::rand32 {i_limit} {

    set math::_rand32_seed \
        [expr {(($math::_rand32_seed * 32310901) + 1) & 0xffffffff}]

    if {$i_limit == 0} {
        return $math::_rand32_seed
    } else {
        return [expr {(wide($math::_rand32_seed) * $i_limit) >> 32}]
    }
}
