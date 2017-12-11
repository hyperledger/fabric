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

package NIST256

// Base Bits= 56
var Modulus= [...]Chunk {0xFFFFFFFFFFFFFF,0xFFFFFFFFFF,0x0,0x1000000,0xFFFFFFFF}
var R2modp= [...]Chunk {0x3000000050000,0x0,0xFFFFFBFFFFFFFA,0xFFFAFFFFFFFEFF,0x2FFFF}
const MConst Chunk=0x1

var CURVE_Cof=[...]Chunk {0x1,0x0,0x0,0x0,0x0}
const CURVE_A int= -3
const CURVE_B_I int= 0
var CURVE_B= [...]Chunk {0xCE3C3E27D2604B,0x6B0CC53B0F63B,0x55769886BC651D,0xAA3A93E7B3EBBD,0x5AC635D8}
var CURVE_Order= [...]Chunk {0xB9CAC2FC632551,0xFAADA7179E84F3,0xFFFFFFFFFFBCE6,0xFFFFFF,0xFFFFFFFF}
var CURVE_Gx= [...]Chunk {0xA13945D898C296,0x7D812DEB33A0F4,0xE563A440F27703,0xE12C4247F8BCE6,0x6B17D1F2}
var CURVE_Gy= [...]Chunk {0xB6406837BF51F5,0x33576B315ECECB,0x4A7C0F9E162BCE,0xFE1A7F9B8EE7EB,0x4FE342E2}
