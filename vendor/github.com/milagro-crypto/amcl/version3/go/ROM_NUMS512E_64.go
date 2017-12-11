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

package NUMS512E

// Base Bits= 60
var Modulus= [...]Chunk {0xFFFFFFFFFFFFDC7,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFF}
var R2modp= [...]Chunk {0x100000000000000,0x4F0B,0x0,0x0,0x0,0x0,0x0,0x0,0x0}
const MConst Chunk=0x239

var CURVE_Cof=[...]Chunk {0x4,0x0,0x0,0x0,0x0,0x0,0x0,0x0,0x0}
const CURVE_A int= 1
const CURVE_B_I int= -78296
var CURVE_B= [...]Chunk {0xFFFFFFFFFFECBEF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFF}
var CURVE_Order= [...]Chunk {0x7468CF51BEED46D,0x4605786DEFECFF6,0xFD8C970B686F52A,0x636D2FCF91BA9E3,0xFFFFFFFFFFFB4F0,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFFF,0x3FFFFFFF}
var CURVE_Gx= [...]Chunk {0x5B9AB2999EC57FE,0xE525427CC4F015C,0xDC992568904AD0F,0xC14EEE46730F78B,0xEBE273B81474621,0x9F4DC4A38227A17,0x888D3C5332FD1E7,0x128DB69C7A18CB7,0xDF8E316D}
var CURVE_Gy= [...]Chunk {0x26DDEC0C1E2F5E1,0x66D38A9BF1D01F3,0xA06862AECC1FD02,0x53F2E9963562601,0xB95909E834120CA,0x26D8259D22A92B6,0x7A82A256EE476F7,0x9D49CA7198B0F57,0x6D09BFF3}
