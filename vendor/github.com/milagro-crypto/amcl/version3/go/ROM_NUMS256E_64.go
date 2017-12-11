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

package NUMS256E

// Base Bits= 56
var Modulus= [...]Chunk {0xFFFFFFFFFFFF43,0xFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFF,0xFFFFFFFF}
var R2modp= [...]Chunk {0x89000000000000,0x8B,0x0,0x0,0x0}
const MConst Chunk=0xBD

var CURVE_Cof=[...]Chunk {0x4,0x0,0x0,0x0,0x0}
const CURVE_A int= 1
const CURVE_B_I int= -15342
var CURVE_B= [...]Chunk {0xFFFFFFFFFFC355,0xFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFF,0xFFFFFFFFFFFFFF,0xFFFFFFFF}
var CURVE_Order= [...]Chunk {0x47B190EEDD4AF5,0x5AA52F59439B1A,0x4195,0x0,0x40000000}
var CURVE_Gx= [...]Chunk {0xDEC0902EED13DA,0x8A0EE3083586A0,0x5F69209BD60C39,0x6AEA237DCD1E3D,0x8A7514FB}
var CURVE_Gy= [...]Chunk {0xA616E7798A89E6,0x61D810856ED32F,0xD9A64B8010715F,0xD9D925C7CE9665,0x44D53E9F}