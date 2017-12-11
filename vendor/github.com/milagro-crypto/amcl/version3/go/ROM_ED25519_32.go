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

package ED25519

// Base Bits= 29
var Modulus= [...]Chunk {0x1FFFFFED,0x1FFFFFFF,0x1FFFFFFF,0x1FFFFFFF,0x1FFFFFFF,0x1FFFFFFF,0x1FFFFFFF,0x1FFFFFFF,0x7FFFFF}
var R2modp= [...]Chunk {0x169000,0x0,0x0,0x0,0x0,0x0,0x0,0x0,0x0}
const MConst Chunk= 0x13

var CURVE_Cof=[...]Chunk {0x8,0x0,0x0,0x0,0x0,0x0,0x0,0x0,0x0}
const CURVE_A int= -1
const CURVE_B_I int= 0
var CURVE_B= [...]Chunk {0x135978A3,0xF5A6E50,0x10762ADD,0x149A82,0x1E898007,0x3CBBBC,0x19CE331D,0x1DC56DFF,0x52036C}
var CURVE_Order= [...]Chunk {0x1CF5D3ED,0x9318D2,0x1DE73596,0x1DF3BD45,0x14D,0x0,0x0,0x0,0x100000}
var CURVE_Gx= [...]Chunk {0xF25D51A,0xAB16B04,0x969ECB2,0x198EC12A,0xDC5C692,0x1118FEEB,0xFFB0293,0x1A79ADCA,0x216936}
var CURVE_Gy= [...]Chunk {0x6666658,0x13333333,0x19999999,0xCCCCCCC,0x6666666,0x13333333,0x19999999,0xCCCCCCC,0x666666}

