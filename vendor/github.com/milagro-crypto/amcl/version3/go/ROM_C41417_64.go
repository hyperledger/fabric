/*
Licensed to the Apache Software Foundation (ASF) under one
or more contributor license agreements.  See the NOTICE file
distributed with this work for additional information
regarding copyright ownership.  The ASF licenses this file
to you under the Apache License, Version 2.0 (the
"License"); you may not use this file except in compliance
with the License.  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing,
software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
KIND, either express or implied.  See the License for the
specific language governing permissions and limitations
under the License.
*/

/* Fixed Data in ROM - Field and Curve parameters */

package C41417

// Base Bits= 60
var Modulus= [...]Chunk {0xFFFFFFFFFFFFFEF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0x3FFFFFFFFFFFFF}
var R2modp= [...]Chunk {0x121000,0x0,0x0,0x0,0x0,0x0,0x0}
const MConst Chunk=0x11

var CURVE_Cof=[...]Chunk {0x8,0x0,0x0,0x0,0x0,0x0,0x0}
const CURVE_A int= 1
const CURVE_B_I int= 3617
var CURVE_B= [...]Chunk {0xE21,0x0,0x0,0x0,0x0,0x0,0x0}
var CURVE_Order= [...]Chunk {0xB0E71A5E106AF79,0x1C0338AD63CF181,0x414CF706022B36F,0xFFFFFFFFEB3CC92,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0x7FFFFFFFFFFFF}
var CURVE_Gx= [...]Chunk {0x4FD3812F3CBC595,0x1A73FAA8537C64C,0x4AB4D6D6BA11130,0x3EC7F57FF35498A,0xE5FCD46369F44C0,0x300218C0631C326,0x1A334905141443}
var CURVE_Gy= [...]Chunk {0x22,0x0,0x0,0x0,0x0,0x0,0x0}
