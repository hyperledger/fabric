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

/* AES Encryption */ 

package amcl

//import "fmt"

const aes_ECB int=0
const aes_CBC int=1
const aes_CFB1 int=2
const aes_CFB2 int=3
const aes_CFB4 int=5
const aes_OFB1 int=14
const aes_OFB2 int=15
const aes_OFB4 int=17
const aes_OFB8 int=21
const aes_OFB16 int=29
const aes_CTR1 int=30
const aes_CTR2 int=31
const aes_CTR4 int=33 
const aes_CTR8 int=37 
const aes_CTR16 int=45 

var aes_InCo = [...]byte {0xB,0xD,0x9,0xE}  /* Inverse Coefficients */

var aes_ptab = [...]byte {
     1, 3, 5, 15, 17, 51, 85, 255, 26, 46, 114, 150, 161, 248, 19, 53,
     95, 225, 56, 72, 216, 115, 149, 164, 247, 2, 6, 10, 30, 34, 102, 170,
     229, 52, 92, 228, 55, 89, 235, 38, 106, 190, 217, 112, 144, 171, 230, 49,
     83, 245, 4, 12, 20, 60, 68, 204, 79, 209, 104, 184, 211, 110, 178, 205,
     76, 212, 103, 169, 224, 59, 77, 215, 98, 166, 241, 8, 24, 40, 120, 136,
     131, 158, 185, 208, 107, 189, 220, 127, 129, 152, 179, 206, 73, 219, 118, 154,
     181, 196, 87, 249, 16, 48, 80, 240, 11, 29, 39, 105, 187, 214, 97, 163,
     254, 25, 43, 125, 135, 146, 173, 236, 47, 113, 147, 174, 233, 32, 96, 160,
     251, 22, 58, 78, 210, 109, 183, 194, 93, 231, 50, 86, 250, 21, 63, 65,
     195, 94, 226, 61, 71, 201, 64, 192, 91, 237, 44, 116, 156, 191, 218, 117,
     159, 186, 213, 100, 172, 239, 42, 126, 130, 157, 188, 223, 122, 142, 137, 128,
     155, 182, 193, 88, 232, 35, 101, 175, 234, 37, 111, 177, 200, 67, 197, 84,
     252, 31, 33, 99, 165, 244, 7, 9, 27, 45, 119, 153, 176, 203, 70, 202,
     69, 207, 74, 222, 121, 139, 134, 145, 168, 227, 62, 66, 198, 81, 243, 14,
     18, 54, 90, 238, 41, 123, 141, 140, 143, 138, 133, 148, 167, 242, 13, 23,
     57, 75, 221, 124, 132, 151, 162, 253, 28, 36, 108, 180, 199, 82, 246, 1}

var aes_ltab = [...]byte {
      0, 255, 25, 1, 50, 2, 26, 198, 75, 199, 27, 104, 51, 238, 223, 3,
     100, 4, 224, 14, 52, 141, 129, 239, 76, 113, 8, 200, 248, 105, 28, 193,
     125, 194, 29, 181, 249, 185, 39, 106, 77, 228, 166, 114, 154, 201, 9, 120,
     101, 47, 138, 5, 33, 15, 225, 36, 18, 240, 130, 69, 53, 147, 218, 142,
     150, 143, 219, 189, 54, 208, 206, 148, 19, 92, 210, 241, 64, 70, 131, 56,
     102, 221, 253, 48, 191, 6, 139, 98, 179, 37, 226, 152, 34, 136, 145, 16,
     126, 110, 72, 195, 163, 182, 30, 66, 58, 107, 40, 84, 250, 133, 61, 186,
     43, 121, 10, 21, 155, 159, 94, 202, 78, 212, 172, 229, 243, 115, 167, 87,
     175, 88, 168, 80, 244, 234, 214, 116, 79, 174, 233, 213, 231, 230, 173, 232,
     44, 215, 117, 122, 235, 22, 11, 245, 89, 203, 95, 176, 156, 169, 81, 160,
     127, 12, 246, 111, 23, 196, 73, 236, 216, 67, 31, 45, 164, 118, 123, 183,
     204, 187, 62, 90, 251, 96, 177, 134, 59, 82, 161, 108, 170, 85, 41, 157,
     151, 178, 135, 144, 97, 190, 220, 252, 188, 149, 207, 205, 55, 63, 91, 209,
     83, 57, 132, 60, 65, 162, 109, 71, 20, 42, 158, 93, 86, 242, 211, 171,
     68, 17, 146, 217, 35, 32, 46, 137, 180, 124, 184, 38, 119, 153, 227, 165,
     103, 74, 237, 222, 197, 49, 254, 24, 13, 99, 140, 128, 192, 247, 112, 7}
   

var aes_fbsub = [...]byte {
     99, 124, 119, 123, 242, 107, 111, 197, 48, 1, 103, 43, 254, 215, 171, 118,
     202, 130, 201, 125, 250, 89, 71, 240, 173, 212, 162, 175, 156, 164, 114, 192,
     183, 253, 147, 38, 54, 63, 247, 204, 52, 165, 229, 241, 113, 216, 49, 21,
     4, 199, 35, 195, 24, 150, 5, 154, 7, 18, 128, 226, 235, 39, 178, 117,
     9, 131, 44, 26, 27, 110, 90, 160, 82, 59, 214, 179, 41, 227, 47, 132,
     83, 209, 0, 237, 32, 252, 177, 91, 106, 203, 190, 57, 74, 76, 88, 207,
     208, 239, 170, 251, 67, 77, 51, 133, 69, 249, 2, 127, 80, 60, 159, 168,
     81, 163, 64, 143, 146, 157, 56, 245, 188, 182, 218, 33, 16, 255, 243, 210,
     205, 12, 19, 236, 95, 151, 68, 23, 196, 167, 126, 61, 100, 93, 25, 115,
     96, 129, 79, 220, 34, 42, 144, 136, 70, 238, 184, 20, 222, 94, 11, 219,
     224, 50, 58, 10, 73, 6, 36, 92, 194, 211, 172, 98, 145, 149, 228, 121,
     231, 200, 55, 109, 141, 213, 78, 169, 108, 86, 244, 234, 101, 122, 174, 8,
     186, 120, 37, 46, 28, 166, 180, 198, 232, 221, 116, 31, 75, 189, 139, 138,
     112, 62, 181, 102, 72, 3, 246, 14, 97, 53, 87, 185, 134, 193, 29, 158,
     225, 248, 152, 17, 105, 217, 142, 148, 155, 30, 135, 233, 206, 85, 40, 223,
     140, 161, 137, 13, 191, 230, 66, 104, 65, 153, 45, 15, 176, 84, 187, 22}
    
var aes_rbsub = [...]byte {
     82, 9, 106, 213, 48, 54, 165, 56, 191, 64, 163, 158, 129, 243, 215, 251,
     124, 227, 57, 130, 155, 47, 255, 135, 52, 142, 67, 68, 196, 222, 233, 203,
     84, 123, 148, 50, 166, 194, 35, 61, 238, 76, 149, 11, 66, 250, 195, 78,
     8, 46, 161, 102, 40, 217, 36, 178, 118, 91, 162, 73, 109, 139, 209, 37,
     114, 248, 246, 100, 134, 104, 152, 22, 212, 164, 92, 204, 93, 101, 182, 146,
     108, 112, 72, 80, 253, 237, 185, 218, 94, 21, 70, 87, 167, 141, 157, 132,
     144, 216, 171, 0, 140, 188, 211, 10, 247, 228, 88, 5, 184, 179, 69, 6,
     208, 44, 30, 143, 202, 63, 15, 2, 193, 175, 189, 3, 1, 19, 138, 107,
     58, 145, 17, 65, 79, 103, 220, 234, 151, 242, 207, 206, 240, 180, 230, 115,
     150, 172, 116, 34, 231, 173, 53, 133, 226, 249, 55, 232, 28, 117, 223, 110,
     71, 241, 26, 113, 29, 41, 197, 137, 111, 183, 98, 14, 170, 24, 190, 27,
     252, 86, 62, 75, 198, 210, 121, 32, 154, 219, 192, 254, 120, 205, 90, 244,
     31, 221, 168, 51, 136, 7, 199, 49, 177, 18, 16, 89, 39, 128, 236, 95,
     96, 81, 127, 169, 25, 181, 74, 13, 45, 229, 122, 159, 147, 201, 156, 239,
     160, 224, 59, 77, 174, 42, 245, 176, 200, 235, 187, 60, 131, 83, 153, 97,
     23, 43, 4, 126, 186, 119, 214, 38, 225, 105, 20, 99, 85, 33, 12, 125}
    

var aes_rco = [...]byte {1,2,4,8,16,32,64,128,27,54,108,216,171,77,154,47}

var aes_ftable = [...]uint32 {
    0xa56363c6,0x847c7cf8,0x997777ee,0x8d7b7bf6,0xdf2f2ff,0xbd6b6bd6,
    0xb16f6fde,0x54c5c591,0x50303060,0x3010102,0xa96767ce,0x7d2b2b56,
    0x19fefee7,0x62d7d7b5,0xe6abab4d,0x9a7676ec,0x45caca8f,0x9d82821f,
    0x40c9c989,0x877d7dfa,0x15fafaef,0xeb5959b2,0xc947478e,0xbf0f0fb,
    0xecadad41,0x67d4d4b3,0xfda2a25f,0xeaafaf45,0xbf9c9c23,0xf7a4a453,
    0x967272e4,0x5bc0c09b,0xc2b7b775,0x1cfdfde1,0xae93933d,0x6a26264c,
    0x5a36366c,0x413f3f7e,0x2f7f7f5,0x4fcccc83,0x5c343468,0xf4a5a551,
    0x34e5e5d1,0x8f1f1f9,0x937171e2,0x73d8d8ab,0x53313162,0x3f15152a,
    0xc040408,0x52c7c795,0x65232346,0x5ec3c39d,0x28181830,0xa1969637,
    0xf05050a,0xb59a9a2f,0x907070e,0x36121224,0x9b80801b,0x3de2e2df,
    0x26ebebcd,0x6927274e,0xcdb2b27f,0x9f7575ea,0x1b090912,0x9e83831d,
    0x742c2c58,0x2e1a1a34,0x2d1b1b36,0xb26e6edc,0xee5a5ab4,0xfba0a05b,
    0xf65252a4,0x4d3b3b76,0x61d6d6b7,0xceb3b37d,0x7b292952,0x3ee3e3dd,
    0x712f2f5e,0x97848413,0xf55353a6,0x68d1d1b9,0x0,0x2cededc1,
    0x60202040,0x1ffcfce3,0xc8b1b179,0xed5b5bb6,0xbe6a6ad4,0x46cbcb8d,
    0xd9bebe67,0x4b393972,0xde4a4a94,0xd44c4c98,0xe85858b0,0x4acfcf85,
    0x6bd0d0bb,0x2aefefc5,0xe5aaaa4f,0x16fbfbed,0xc5434386,0xd74d4d9a,
    0x55333366,0x94858511,0xcf45458a,0x10f9f9e9,0x6020204,0x817f7ffe,
    0xf05050a0,0x443c3c78,0xba9f9f25,0xe3a8a84b,0xf35151a2,0xfea3a35d,
    0xc0404080,0x8a8f8f05,0xad92923f,0xbc9d9d21,0x48383870,0x4f5f5f1,
    0xdfbcbc63,0xc1b6b677,0x75dadaaf,0x63212142,0x30101020,0x1affffe5,
    0xef3f3fd,0x6dd2d2bf,0x4ccdcd81,0x140c0c18,0x35131326,0x2fececc3,
    0xe15f5fbe,0xa2979735,0xcc444488,0x3917172e,0x57c4c493,0xf2a7a755,
    0x827e7efc,0x473d3d7a,0xac6464c8,0xe75d5dba,0x2b191932,0x957373e6,
    0xa06060c0,0x98818119,0xd14f4f9e,0x7fdcdca3,0x66222244,0x7e2a2a54,
    0xab90903b,0x8388880b,0xca46468c,0x29eeeec7,0xd3b8b86b,0x3c141428,
    0x79dedea7,0xe25e5ebc,0x1d0b0b16,0x76dbdbad,0x3be0e0db,0x56323264,
    0x4e3a3a74,0x1e0a0a14,0xdb494992,0xa06060c,0x6c242448,0xe45c5cb8,
    0x5dc2c29f,0x6ed3d3bd,0xefacac43,0xa66262c4,0xa8919139,0xa4959531,
    0x37e4e4d3,0x8b7979f2,0x32e7e7d5,0x43c8c88b,0x5937376e,0xb76d6dda,
    0x8c8d8d01,0x64d5d5b1,0xd24e4e9c,0xe0a9a949,0xb46c6cd8,0xfa5656ac,
    0x7f4f4f3,0x25eaeacf,0xaf6565ca,0x8e7a7af4,0xe9aeae47,0x18080810,
    0xd5baba6f,0x887878f0,0x6f25254a,0x722e2e5c,0x241c1c38,0xf1a6a657,
    0xc7b4b473,0x51c6c697,0x23e8e8cb,0x7cdddda1,0x9c7474e8,0x211f1f3e,
    0xdd4b4b96,0xdcbdbd61,0x868b8b0d,0x858a8a0f,0x907070e0,0x423e3e7c,
    0xc4b5b571,0xaa6666cc,0xd8484890,0x5030306,0x1f6f6f7,0x120e0e1c,
    0xa36161c2,0x5f35356a,0xf95757ae,0xd0b9b969,0x91868617,0x58c1c199,
    0x271d1d3a,0xb99e9e27,0x38e1e1d9,0x13f8f8eb,0xb398982b,0x33111122,
    0xbb6969d2,0x70d9d9a9,0x898e8e07,0xa7949433,0xb69b9b2d,0x221e1e3c,
    0x92878715,0x20e9e9c9,0x49cece87,0xff5555aa,0x78282850,0x7adfdfa5,
    0x8f8c8c03,0xf8a1a159,0x80898909,0x170d0d1a,0xdabfbf65,0x31e6e6d7,
    0xc6424284,0xb86868d0,0xc3414182,0xb0999929,0x772d2d5a,0x110f0f1e,
    0xcbb0b07b,0xfc5454a8,0xd6bbbb6d,0x3a16162c}

var aes_rtable = [...]uint32 {
    0x50a7f451,0x5365417e,0xc3a4171a,0x965e273a,0xcb6bab3b,0xf1459d1f,
    0xab58faac,0x9303e34b,0x55fa3020,0xf66d76ad,0x9176cc88,0x254c02f5,
    0xfcd7e54f,0xd7cb2ac5,0x80443526,0x8fa362b5,0x495ab1de,0x671bba25,
    0x980eea45,0xe1c0fe5d,0x2752fc3,0x12f04c81,0xa397468d,0xc6f9d36b,
    0xe75f8f03,0x959c9215,0xeb7a6dbf,0xda595295,0x2d83bed4,0xd3217458,
    0x2969e049,0x44c8c98e,0x6a89c275,0x78798ef4,0x6b3e5899,0xdd71b927,
    0xb64fe1be,0x17ad88f0,0x66ac20c9,0xb43ace7d,0x184adf63,0x82311ae5,
    0x60335197,0x457f5362,0xe07764b1,0x84ae6bbb,0x1ca081fe,0x942b08f9,
    0x58684870,0x19fd458f,0x876cde94,0xb7f87b52,0x23d373ab,0xe2024b72,
    0x578f1fe3,0x2aab5566,0x728ebb2,0x3c2b52f,0x9a7bc586,0xa50837d3,
    0xf2872830,0xb2a5bf23,0xba6a0302,0x5c8216ed,0x2b1ccf8a,0x92b479a7,
    0xf0f207f3,0xa1e2694e,0xcdf4da65,0xd5be0506,0x1f6234d1,0x8afea6c4,
    0x9d532e34,0xa055f3a2,0x32e18a05,0x75ebf6a4,0x39ec830b,0xaaef6040,
    0x69f715e,0x51106ebd,0xf98a213e,0x3d06dd96,0xae053edd,0x46bde64d,
    0xb58d5491,0x55dc471,0x6fd40604,0xff155060,0x24fb9819,0x97e9bdd6,
    0xcc434089,0x779ed967,0xbd42e8b0,0x888b8907,0x385b19e7,0xdbeec879,
    0x470a7ca1,0xe90f427c,0xc91e84f8,0x0,0x83868009,0x48ed2b32,
    0xac70111e,0x4e725a6c,0xfbff0efd,0x5638850f,0x1ed5ae3d,0x27392d36,
    0x64d90f0a,0x21a65c68,0xd1545b9b,0x3a2e3624,0xb1670a0c,0xfe75793,
    0xd296eeb4,0x9e919b1b,0x4fc5c080,0xa220dc61,0x694b775a,0x161a121c,
    0xaba93e2,0xe52aa0c0,0x43e0223c,0x1d171b12,0xb0d090e,0xadc78bf2,
    0xb9a8b62d,0xc8a91e14,0x8519f157,0x4c0775af,0xbbdd99ee,0xfd607fa3,
    0x9f2601f7,0xbcf5725c,0xc53b6644,0x347efb5b,0x7629438b,0xdcc623cb,
    0x68fcedb6,0x63f1e4b8,0xcadc31d7,0x10856342,0x40229713,0x2011c684,
    0x7d244a85,0xf83dbbd2,0x1132f9ae,0x6da129c7,0x4b2f9e1d,0xf330b2dc,
    0xec52860d,0xd0e3c177,0x6c16b32b,0x99b970a9,0xfa489411,0x2264e947,
    0xc48cfca8,0x1a3ff0a0,0xd82c7d56,0xef903322,0xc74e4987,0xc1d138d9,
    0xfea2ca8c,0x360bd498,0xcf81f5a6,0x28de7aa5,0x268eb7da,0xa4bfad3f,
    0xe49d3a2c,0xd927850,0x9bcc5f6a,0x62467e54,0xc2138df6,0xe8b8d890,
    0x5ef7392e,0xf5afc382,0xbe805d9f,0x7c93d069,0xa92dd56f,0xb31225cf,
    0x3b99acc8,0xa77d1810,0x6e639ce8,0x7bbb3bdb,0x97826cd,0xf418596e,
    0x1b79aec,0xa89a4f83,0x656e95e6,0x7ee6ffaa,0x8cfbc21,0xe6e815ef,
    0xd99be7ba,0xce366f4a,0xd4099fea,0xd67cb029,0xafb2a431,0x31233f2a,
    0x3094a5c6,0xc066a235,0x37bc4e74,0xa6ca82fc,0xb0d090e0,0x15d8a733,
    0x4a9804f1,0xf7daec41,0xe50cd7f,0x2ff69117,0x8dd64d76,0x4db0ef43,
    0x544daacc,0xdf0496e4,0xe3b5d19e,0x1b886a4c,0xb81f2cc1,0x7f516546,
    0x4ea5e9d,0x5d358c01,0x737487fa,0x2e410bfb,0x5a1d67b3,0x52d2db92,
    0x335610e9,0x1347d66d,0x8c61d79a,0x7a0ca137,0x8e14f859,0x893c13eb,
    0xee27a9ce,0x35c961b7,0xede51ce1,0x3cb1477a,0x59dfd29c,0x3f73f255,
    0x79ce1418,0xbf37c773,0xeacdf753,0x5baafd5f,0x146f3ddf,0x86db4478,
    0x81f3afca,0x3ec468b9,0x2c342438,0x5f40a3c2,0x72c31d16,0xc25e2bc,
    0x8b493c28,0x41950dff,0x7101a839,0xdeb30c08,0x9ce4b4d8,0x90c15664,
    0x6184cb7b,0x70b632d5,0x745c6c48,0x4257b8d0}

type AES struct {
	Nk int
	Nr int
	mode int
	fkey [60]uint32
	rkey [60]uint32
	f [16]byte
}

/* Rotates 32-bit word left by 1, 2 or 3 byte  */

func aes_ROTL8(x uint32) uint32 {
	return (((x)<<8)|((x)>>24))
}

func aes_ROTL16(x uint32) uint32 {
	return (((x)<<16)|((x)>>16))
}

func aes_ROTL24(x uint32) uint32 {
	return (((x)<<24)|((x)>>8))
}

func aes_pack(b [4]byte) uint32 { /* pack bytes into a 32-bit Word */
        return ((uint32(b[3])&0xff)<<24)|((uint32(b[2])&0xff)<<16)|((uint32(b[1])&0xff)<<8)|(uint32(b[0])&0xff)
}
  
func aes_unpack(a uint32) [4]byte { /* unpack bytes from a word */
        var b=[4]byte{byte(a&0xff),byte((a>>8)&0xff),byte((a>>16)&0xff),byte((a>>24)&0xff)}
	return b;
}
  
func aes_bmul(x byte,y byte) byte { /* x.y= AntiLog(Log(x) + Log(y)) */
    
        ix:=int(x)&0xff
        iy:=int(y)&0xff
        lx:=int(aes_ltab[ix])&0xff
        ly:=int(aes_ltab[iy])&0xff
    
        if x != 0 && y != 0 {
		return aes_ptab[(lx+ly)%255]
	} else {return byte(0)}
}
  
func aes_SubByte(a uint32) uint32 {
        b:=aes_unpack(a)
        b[0]=aes_fbsub[int(b[0])]
        b[1]=aes_fbsub[int(b[1])]
        b[2]=aes_fbsub[int(b[2])]
        b[3]=aes_fbsub[int(b[3])]
        return aes_pack(b);
}    

func aes_product(x uint32,y uint32) byte { /* dot product of two 4-byte arrays */
        xb:=aes_unpack(x)
        yb:=aes_unpack(y)
    
        return (aes_bmul(xb[0],yb[0])^aes_bmul(xb[1],yb[1])^aes_bmul(xb[2],yb[2])^aes_bmul(xb[3],yb[3]))
}

func aes_InvMixCol(x uint32) uint32 { /* matrix Multiplication */
        var b [4]byte
        m:=aes_pack(aes_InCo)
        b[3]=aes_product(m,x)
        m=aes_ROTL24(m)
        b[2]=aes_product(m,x)
        m=aes_ROTL24(m)
        b[1]=aes_product(m,x)
        m=aes_ROTL24(m)
        b[0]=aes_product(m,x)
        var y=aes_pack(b)
        return y
}

func aes_increment(f []byte) {
	for i:=0;i<16;i++ {
		f[i]++
		if f[i]!=0 {break}
	}
}

/* reset cipher */
func (A *AES) Reset(m int,iv []byte) { /* reset mode, or reset iv */
	A.mode=m;
        for i:=0;i<16;i++ {A.f[i]=0}
        if (A.mode != aes_ECB) && (iv != nil) {
            for i:=0;i<16;i++ {A.f[i]=iv[i]}
	}
}

func (A *AES) Init(m int,nk int,key []byte,iv []byte) bool { 
/* Key Scheduler. Create expanded encryption key */
	var CipherKey [8]uint32
        var b [4]byte
        nk/=4
	if nk!=4 && nk!=6 && nk!=8 {return false}
	nr:=6+nk
	A.Nk=nk
	A.Nr=nr
        A.Reset(m,iv);
        N:=4*(nr+1)
        
        j:=0
        for  i:=0;i<nk;i++ {
            for k:=0;k<4;k++ {b[k]=key[j+k]}
            CipherKey[i]=aes_pack(b);
            j+=4;
        }
        for i:=0;i<nk;i++ {A.fkey[i]=CipherKey[i]}
        j=nk
        for k:=0;j<N;k++ {
            A.fkey[j]=A.fkey[j-nk]^aes_SubByte(aes_ROTL24(A.fkey[j-1]))^uint32(aes_rco[k])
            for i:=1;i<nk && (i+j)<N;i++ {
                A.fkey[i+j]=A.fkey[i+j-nk]^A.fkey[i+j-1]
            }
            j+=nk
        }
        
        /* now for the expanded decrypt key in reverse order */
        
        for j:=0;j<4;j++ {A.rkey[j+N-4]=A.fkey[j]}
        for i:=4;i<N-4;i+=4 {
            k:=N-4-i;
            for j:=0;j<4;j++ {A.rkey[k+j]=aes_InvMixCol(A.fkey[i+j])}
        }
        for j:=N-4;j<N;j++ {A.rkey[j-N+4]=A.fkey[j]}
	return true
}

func NewAES() *AES {
	var A=new(AES)
	return A
}

func (A *AES) Getreg() [16]byte {
        var ir [16]byte
        for i:=0;i<16;i++ {ir[i]=A.f[i]}
        return ir
}

    /* Encrypt a single block */
func (A *AES) ecb_encrypt(buff []byte) {
        var b [4]byte
        var p [4]uint32
        var q [4]uint32
    
        j:=0
        for i:=0;i<4;i++ {
            for k:=0;k<4;k++ {b[k]=buff[j+k]}
            p[i]=aes_pack(b)
            p[i]^=A.fkey[i]
            j+=4
        }
    
        k:=4
    
    /* State alternates between p and q */
        for i:=1;i<A.Nr;i++ {
            q[0]=A.fkey[k]^aes_ftable[int(p[0]&0xff)]^aes_ROTL8(aes_ftable[int((p[1]>>8)&0xff)])^aes_ROTL16(aes_ftable[int((p[2]>>16)&0xff)])^aes_ROTL24(aes_ftable[int((p[3]>>24)&0xff)])
            
            q[1]=A.fkey[k+1]^aes_ftable[int(p[1]&0xff)]^aes_ROTL8(aes_ftable[int((p[2]>>8)&0xff)])^aes_ROTL16(aes_ftable[int((p[3]>>16)&0xff)])^aes_ROTL24(aes_ftable[int((p[0]>>24)&0xff)])
            
            q[2]=A.fkey[k+2]^aes_ftable[int(p[2]&0xff)]^aes_ROTL8(aes_ftable[int((p[3]>>8)&0xff)])^aes_ROTL16(aes_ftable[int((p[0]>>16)&0xff)])^aes_ROTL24(aes_ftable[int((p[1]>>24)&0xff)])
            
            q[3]=A.fkey[k+3]^aes_ftable[int(p[3]&0xff)]^aes_ROTL8(aes_ftable[int((p[0]>>8)&0xff)])^aes_ROTL16(aes_ftable[int((p[1]>>16)&0xff)])^aes_ROTL24(aes_ftable[int((p[2]>>24)&0xff)])
            
            k+=4;
            for j=0;j<4;j++ {
		t:=p[j]; p[j]=q[j]; q[j]=t
            }
        }
    
    /* Last Round */
    
        q[0]=A.fkey[k]^uint32(aes_fbsub[int(p[0]&0xff)])^aes_ROTL8(uint32(aes_fbsub[int((p[1]>>8)&0xff)]))^aes_ROTL16(uint32(aes_fbsub[int((p[2]>>16)&0xff)]))^aes_ROTL24(uint32(aes_fbsub[int((p[3]>>24)&0xff)]))
    
        q[1]=A.fkey[k+1]^uint32(aes_fbsub[int(p[1]&0xff)])^aes_ROTL8(uint32(aes_fbsub[int((p[2]>>8)&0xff)]))^aes_ROTL16(uint32(aes_fbsub[int((p[3]>>16)&0xff)]))^aes_ROTL24(uint32(aes_fbsub[int((p[0]>>24)&0xff)]))
    
        q[2]=A.fkey[k+2]^uint32(aes_fbsub[int(p[2]&0xff)])^aes_ROTL8(uint32(aes_fbsub[int((p[3]>>8)&0xff)]))^aes_ROTL16(uint32(aes_fbsub[int((p[0]>>16)&0xff)]))^aes_ROTL24(uint32(aes_fbsub[int((p[1]>>24)&0xff)]))
    
        q[3]=A.fkey[k+3]^uint32(aes_fbsub[int(p[3]&0xff)])^aes_ROTL8(uint32(aes_fbsub[int((p[0]>>8)&0xff)]))^aes_ROTL16(uint32(aes_fbsub[int((p[1]>>16)&0xff)]))^aes_ROTL24(uint32(aes_fbsub[int((p[2]>>24)&0xff)]))
    
        j=0
        for i:=0;i<4;i++ {
            b=aes_unpack(q[i])
            for k=0;k<4;k++ {buff[j+k]=b[k]}
            j+=4
        }
}
    
    /* Decrypt a single block */
func (A *AES)  ecb_decrypt(buff []byte) {
        var b [4]byte
        var p [4]uint32
        var q [4]uint32
    
        j:=0
        for i:=0;i<4;i++ {
            for k:=0;k<4;k++ {b[k]=buff[j+k]}
            p[i]=aes_pack(b)
            p[i]^=A.rkey[i]
            j+=4
        }
    
        k:=4
    
    /* State alternates between p and q */
        for i:=1;i<A.Nr;i++ {
            
            q[0]=A.rkey[k]^aes_rtable[int(p[0]&0xff)]^aes_ROTL8(aes_rtable[int((p[3]>>8)&0xff)])^aes_ROTL16(aes_rtable[int((p[2]>>16)&0xff)])^aes_ROTL24(aes_rtable[int((p[1]>>24)&0xff)])
            
            q[1]=A.rkey[k+1]^aes_rtable[int(p[1]&0xff)]^aes_ROTL8(aes_rtable[int((p[0]>>8)&0xff)])^aes_ROTL16(aes_rtable[int((p[3]>>16)&0xff)])^aes_ROTL24(aes_rtable[int((p[2]>>24)&0xff)])
            
        
            q[2]=A.rkey[k+2]^aes_rtable[int(p[2]&0xff)]^aes_ROTL8(aes_rtable[int((p[1]>>8)&0xff)])^aes_ROTL16(aes_rtable[int((p[0]>>16)&0xff)])^aes_ROTL24(aes_rtable[int((p[3]>>24)&0xff)])
       
            q[3]=A.rkey[k+3]^aes_rtable[int(p[3]&0xff)]^aes_ROTL8(aes_rtable[int((p[2]>>8)&0xff)])^aes_ROTL16(aes_rtable[int((p[1]>>16)&0xff)])^aes_ROTL24(aes_rtable[int((p[0]>>24)&0xff)])
            
    
            k+=4;
            for j:=0;j<4;j++ {
			t:=p[j]; p[j]=q[j]; q[j]=t
            }
        }
    
    /* Last Round */
        
        q[0]=A.rkey[k]^uint32(aes_rbsub[int(p[0]&0xff)])^aes_ROTL8(uint32(aes_rbsub[int((p[3]>>8)&0xff)]))^aes_ROTL16(uint32(aes_rbsub[int((p[2]>>16)&0xff)]))^aes_ROTL24(uint32(aes_rbsub[int((p[1]>>24)&0xff)]))
        
        q[1]=A.rkey[k+1]^uint32(aes_rbsub[int(p[1]&0xff)])^aes_ROTL8(uint32(aes_rbsub[int((p[0]>>8)&0xff)]))^aes_ROTL16(uint32(aes_rbsub[int((p[3]>>16)&0xff)]))^aes_ROTL24(uint32(aes_rbsub[int((p[2]>>24)&0xff)]))
        
        
        q[2]=A.rkey[k+2]^uint32(aes_rbsub[int(p[2]&0xff)])^aes_ROTL8(uint32(aes_rbsub[int((p[1]>>8)&0xff)]))^aes_ROTL16(uint32(aes_rbsub[int((p[0]>>16)&0xff)]))^aes_ROTL24(uint32(aes_rbsub[int((p[3]>>24)&0xff)]))

        q[3]=A.rkey[k+3]^uint32(aes_rbsub[int((p[3])&0xff)])^aes_ROTL8(uint32(aes_rbsub[int((p[2]>>8)&0xff)]))^aes_ROTL16(uint32(aes_rbsub[int((p[1]>>16)&0xff)]))^aes_ROTL24(uint32(aes_rbsub[int((p[0]>>24)&0xff)]))
    
        j=0
        for i:=0;i<4;i++ {
            b=aes_unpack(q[i]);
            for k:=0;k<4;k++ {buff[j+k]=b[k]}
            j+=4
        }
}

/* Encrypt using selected mode of operation */
func (A *AES) Encrypt(buff []byte) uint32 {
	var st [16]byte
    
    // Supported Modes of Operation
    
        var fell_off uint32=0
        switch A.mode {
        case aes_ECB:
            A.ecb_encrypt(buff)
            return 0
        case aes_CBC:
            for j:=0;j<16;j++ {buff[j]^=A.f[j]}
            A.ecb_encrypt(buff)
            for j:=0;j<16;j++ {A.f[j]=buff[j]}
            return 0
    
        case aes_CFB1:
            fallthrough
        case aes_CFB2:
            fallthrough
        case aes_CFB4:
            bytes:=A.mode-aes_CFB1+1
            for j:=0;j<bytes;j++ {fell_off=(fell_off<<8)|uint32(A.f[j])}
            for j:=0;j<16;j++ {st[j]=A.f[j]}
            for j:=bytes;j<16;j++ {A.f[j-bytes]=A.f[j]}
            A.ecb_encrypt(st[:])
            for j:=0;j<bytes;j++ {
		buff[j]^=st[j]
		A.f[16-bytes+j]=buff[j]
            }
            return fell_off
    
        case aes_OFB1:
            fallthrough
        case aes_OFB2:
            fallthrough
        case aes_OFB4:
            fallthrough
        case aes_OFB8:
            fallthrough
        case aes_OFB16:
    
            bytes:=A.mode-aes_OFB1+1
            A.ecb_encrypt(A.f[:])
            for j:=0;j<bytes;j++ {buff[j]^=A.f[j]}
            return 0;
    
	case aes_CTR1:
	    fallthrough
	case aes_CTR2:
	    fallthrough
	case aes_CTR4:
	    fallthrough
	case aes_CTR8:
	    fallthrough
	case aes_CTR16:
	    bytes:=A.mode-aes_CTR1+1
	    for j:=0;j<16;j++ {st[j]=A.f[j]}
	    A.ecb_encrypt(st[:])
	    for j:=0;j<bytes;j++ {buff[j]^=st[j]}
	    aes_increment(A.f[:])
	    return 0

        default:
            return 0
        }
}
    
    /* Decrypt using selected mode of operation */
func (A *AES) Decrypt(buff []byte) uint32 {

	var st [16]byte
        
        // Supported Modes of Operation
        
        var fell_off uint32=0
        switch A.mode {
        case aes_ECB:
            A.ecb_decrypt(buff);
            return 0;
        case aes_CBC:
            for j:=0;j<16;j++ {
		st[j]=A.f[j];
		A.f[j]=buff[j];
            }
            A.ecb_decrypt(buff);
            for j:=0;j<16;j++ {
		buff[j]^=st[j];
		st[j]=0
            }
            return 0
        case aes_CFB1:
            fallthrough
        case aes_CFB2:
            fallthrough
        case aes_CFB4:
            bytes:=A.mode-aes_CFB1+1;
            for j:=0;j<bytes;j++ {fell_off=(fell_off<<8)|uint32(A.f[j])}
            for j:=0;j<16;j++ {st[j]=A.f[j]}
            for j:=bytes;j<16;j++ {A.f[j-bytes]=A.f[j]}
            A.ecb_encrypt(st[:])
            for j:=0;j<bytes;j++ {
		A.f[16-bytes+j]=buff[j]
		buff[j]^=st[j]
            }
            return fell_off
        case aes_OFB1:
            fallthrough
        case aes_OFB2:
            fallthrough
        case aes_OFB4:
            fallthrough
        case aes_OFB8:
            fallthrough
        case aes_OFB16:
            bytes:=A.mode-aes_OFB1+1
            A.ecb_encrypt(A.f[:]);
            for j:=0;j<bytes;j++ {buff[j]^=A.f[j]}
            return 0

	case aes_CTR1:
	    fallthrough
	case aes_CTR2:
	    fallthrough
	case aes_CTR4:
	    fallthrough
	case aes_CTR8:
	    fallthrough
	case aes_CTR16:
	    bytes:=A.mode-aes_CTR1+1
	    for j:=0;j<16;j++ {st[j]=A.f[j]}
	    A.ecb_encrypt(st[:])
	    for j:=0;j<bytes;j++ {buff[j]^=st[j]}
	    aes_increment(A.f[:])
	    return 0

        default:
            return 0;
        }
    } 
    
/* Clean up and delete left-overs */
func (A *AES) End() { // clean up
    for i:=0;i<4*(A.Nr+1);i++ {A.fkey[i]=0; A.rkey[i]=0}
    for i:=0;i<16;i++ {A.f[i]=0}
}
/*
func main() {
	var key [32]byte
	var block [16]byte
	var iv [16]byte

	for i:=0;i<32;i++ {key[i]=0}
	key[0]=1
	for i:=0;i<16;i++ {iv[i]=byte(i)}
	for i:=0;i<16;i++ {block[i]=byte(i)}

	a:=NewAES()

	a.Init(aes_CTR16,32,key[:],iv[:])
	fmt.Printf("Plain= \n")
	for i:=0;i<16;i++  {fmt.Printf("%02X ", block[i]&0xff)}
	fmt.Printf("\n")

	a.Encrypt(block[:])

	fmt.Printf("Encrypt= \n") 
	for i:=0;i<16;i++  {fmt.Printf("%02X ", block[i]&0xff)}
	fmt.Printf("\n")

	a.Reset(aes_CTR16,iv[:])
	a.Decrypt(block[:])

	fmt.Printf("Decrypt= \n") 
	for i:=0;i<16;i++  {fmt.Printf("%02X ", block[i]&0xff)}
	fmt.Printf("\n")

	a.End();
}
*/
