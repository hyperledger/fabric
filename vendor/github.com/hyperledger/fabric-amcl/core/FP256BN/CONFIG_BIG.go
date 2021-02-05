/*
 * Copyright (c) 2012-2020 MIRACL UK Ltd.
 *
 * This file is part of MIRACL Core
 * (see https://github.com/miracl/core).
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package FP256BN

// BIG length in bytes and number base
const MODBYTES uint = 32
const BASEBITS uint = 56

// BIG lengths and Masks
const NLEN int = int((1 + ((8*MODBYTES - 1) / BASEBITS)))
const DNLEN int = 2 * NLEN
const BMASK Chunk = ((Chunk(1) << BASEBITS) - 1)
const HBITS uint = (BASEBITS / 2)
const HMASK Chunk = ((Chunk(1) << HBITS) - 1)
const NEXCESS int = (1 << (uint(CHUNK) - BASEBITS - 1))

const BIGBITS int = int(MODBYTES * 8)

