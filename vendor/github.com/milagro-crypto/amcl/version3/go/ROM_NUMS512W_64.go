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

package NUMS512W

// Base Bits= 60
var Modulus= [...]Chunk {0xFFFFFFFFFFFFDC7,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFF}
var R2modp= [...]Chunk {0x100000000000000,0x4F0B,0x0,0x0,0x0,0x0,0x0,0x0,0x0}
const MConst Chunk=0x239

var CURVE_Cof=[...]Chunk {0x1,0x0,0x0,0x0,0x0,0x0,0x0,0x0,0x0}
const CURVE_A int= -3
const CURVE_B_I int= 121243
var CURVE_B= [...]Chunk {0x1D99B,0x0,0x0,0x0,0x0,0x0,0x0,0x0,0x0}
var CURVE_Order= [...]Chunk {0xE153F390433555D,0x568B36607CD243C,0x258ED97D0BDC63B,0xA4FB94E7831B4FC,0xFFFFFFFFFFF5B3C,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFF}
var CURVE_Gx= [...]Chunk {0xC8287958CABAE57,0x5D60137D6F5DE2D,0x94286255615831D,0xA151076B359E937,0xC25306D9F95021,0x3BB501F6854506E,0x2A03D3B5298CAD8,0x141D0A93DA2B700,0x3AC03447}
var CURVE_Gy= [...]Chunk {0x3A08760383527A6,0x2B5C1E4CFD0FE92,0x1A840B25A5602CF,0x15DA8B0EEDE9C12,0x60C7BD14F14A284,0xDEABBCBB8C8F4B2,0xC63EBB1004B97DB,0x29AD56B3CE0EEED,0x943A54CA}
