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

package NIST521

// Base Bits= 60
var Modulus= [...]Chunk {0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0x1FFFFFFFFFF}
var R2modp= [...]Chunk {0x4000000000,0x0,0x0,0x0,0x0,0x0,0x0,0x0,0x0}
const MConst Chunk=0x1

var CURVE_Cof=[...]Chunk {0x1,0x0,0x0,0x0,0x0,0x0,0x0,0x0,0x0}
const CURVE_A int= -3
const CURVE_B_I int= 0
var CURVE_B= [...]Chunk {0xF451FD46B503F00,0x73DF883D2C34F1E,0x2C0BD3BB1BF0735,0x3951EC7E937B165,0x9918EF109E15619,0x5B99B315F3B8B48,0xB68540EEA2DA72,0x8E1C9A1F929A21A,0x51953EB961}
var CURVE_Order= [...]Chunk {0xB6FB71E91386409,0xB5C9B8899C47AEB,0xC0148F709A5D03B,0x8783BF2F966B7FC,0xFFFFFFFFFFA5186,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0x1FFFFFFFFFF}
var CURVE_Gx= [...]Chunk {0x97E7E31C2E5BD66,0x48B3C1856A429BF,0xDC127A2FFA8DE33,0x5E77EFE75928FE1,0xF606B4D3DBAA14B,0x39053FB521F828A,0x62395B4429C6481,0x404E9CD9E3ECB6,0xC6858E06B7}
var CURVE_Gy= [...]Chunk {0x8BE94769FD16650,0x3C7086A272C2408,0xB9013FAD076135,0x72995EF42640C55,0xD17273E662C97EE,0x49579B446817AFB,0x42C7D1BD998F544,0x9A3BC0045C8A5FB,0x11839296A78}

