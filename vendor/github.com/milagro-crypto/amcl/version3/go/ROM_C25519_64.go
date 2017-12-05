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

package C25519

// Base Bits= 56
var Modulus= [...]Chunk {0xFFFFFFFFFFFFED,0xFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFF,0x7FFFFFFF}
var R2modp= [...]Chunk {0xA4000000000000,0x5,0x0,0x0,0x0}
const MConst Chunk=0x13

var CURVE_Cof=[...]Chunk {0x8,0x0,0x0,0x0,0x0}
const CURVE_A int= 486662
const CURVE_B_I int= 0
var CURVE_B= [...]Chunk {0x0,0x0,0x0,0x0,0x0}
var CURVE_Order= [...]Chunk {0x12631A5CF5D3ED,0xF9DEA2F79CD658,0x14DE,0x0,0x10000000}
var CURVE_Gx= [...]Chunk {0x9,0x0,0x0,0x0,0x0}
var CURVE_Gy= [...]Chunk {0x0,0x0,0x0,0x0,0x0}
