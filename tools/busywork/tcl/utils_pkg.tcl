# utils_pkg.tcl - Define the 'utils' package

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

proc utils_pkg {} {
    
    set dir [file dirname [info script]]

    #  Load Tcl source files

    source $dir/logging.tcl
    source $dir/io.tcl

    source $dir/atExit.tcl
    source $dir/args.tcl
    source $dir/lists.tcl
    source $dir/mathx.tcl
    source $dir/os.tcl
    source $dir/rand.tcl
    source $dir/time.tcl
}

utils_pkg

package provide utils 0.0
