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

package ANSSI

// Base Bits= 56
var Modulus= [...]Chunk {0xFCF353D86E9C03,0xADBCABC8CA6DE8,0xE8CE42435B3961,0xB3AD58F10126D,0xF1FD178C}
var R2modp= [...]Chunk {0x18D2374288CC9C,0x4929E67646BD2B,0x220E6C1D6F7F2D,0x751B1FDABCE02E,0xE7401B78}
const MConst Chunk=0x97483A164E1155

var CURVE_Cof=[...]Chunk {0x1,0x0,0x0,0x0,0x0}
const CURVE_A int= -3
const CURVE_B_I int= 0
var CURVE_B= [...]Chunk {0x75ED967B7BB73F,0xC9AE4B1A18030,0x754A44C00FDFEC,0x5428A9300D4ABA,0xEE353FCA}
var CURVE_Order= [...]Chunk {0xFDD459C6D655E1,0x67E140D2BF941F,0xE8CE42435B53DC,0xB3AD58F10126D,0xF1FD178C}
var CURVE_Gx= [...]Chunk {0xC97A2DD98F5CFF,0xD2DCAF98B70164,0x4749D423958C27,0x56C139EB31183D,0xB6B3D4C3}
var CURVE_Gy= [...]Chunk {0x115A1554062CFB,0xC307E8E4C9E183,0xF0F3ECEF8C2701,0xC8B204911F9271,0x6142E0F7}
