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

package NIST384

// Base Bits= 56
var Modulus= [...]Chunk {0xFFFFFFFF,0xFFFF0000000000,0xFFFFFFFFFEFFFF,0xFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFF,0xFFFFFFFFFFFF}
var R2modp= [...]Chunk {0xFE000000010000,0xFFFFFF,0x2,0xFFFFFFFE00,0x1000000020000,0x0,0x0}
const MConst Chunk=0x100000001

var CURVE_Cof=[...]Chunk {0x1,0x0,0x0,0x0,0x0,0x0,0x0}
const CURVE_A int= -3
const CURVE_B_I int= 0
var CURVE_B= [...]Chunk {0x85C8EDD3EC2AEF,0x398D8A2ED19D2A,0x8F5013875AC656,0xFE814112031408,0xF82D19181D9C6E,0xE7E4988E056BE3,0xB3312FA7E23E}
var CURVE_Order= [...]Chunk {0xEC196ACCC52973,0xDB248B0A77AEC,0x81F4372DDF581A,0xFFFFFFFFC7634D,0xFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFF,0xFFFFFFFFFFFF}
var CURVE_Gx= [...]Chunk {0x545E3872760AB7,0xF25DBF55296C3A,0xE082542A385502,0x8BA79B9859F741,0x20AD746E1D3B62,0x5378EB1C71EF3,0xAA87CA22BE8B}
var CURVE_Gy= [...]Chunk {0x431D7C90EA0E5F,0xB1CE1D7E819D7A,0x13B5F0B8C00A60,0x289A147CE9DA31,0x92DC29F8F41DBD,0x2C6F5D9E98BF92,0x3617DE4A9626}
