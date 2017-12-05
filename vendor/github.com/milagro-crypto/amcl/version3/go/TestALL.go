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

/* test driver and function exerciser for ECDH/ECIES/ECDSA API Functions */


package main

import "fmt"
import "amcl"
import "amcl/ED25519"
import "amcl/BN254"
import "amcl/RSA2048"


func printBinary(array []byte) {
	for i:=0;i<len(array);i++ {
		fmt.Printf("%02x", array[i])
	}
	fmt.Printf("\n")
}  


func ecdh_ED25519(rng *amcl.RAND) {
//	j:=0
	pp:="M0ng00se"
	res:=0

	var sha=ED25519.ECDH_HASH_TYPE

	var S1 [ED25519.EGS]byte
	var W0 [2*ED25519.EFS+1]byte
	var W1 [2*ED25519.EFS+1]byte
	var Z0 [ED25519.EFS]byte
	var Z1 [ED25519.EFS]byte
	var SALT [8]byte
	var P1 [3]byte
	var P2 [4]byte
	var V [2*ED25519.EFS+1]byte
	var M [17]byte
	var T [12]byte
	var CS [ED25519.EGS]byte
	var DS [ED25519.EGS]byte

	for i:=0;i<8;i++ {SALT[i]=byte(i+1)}  // set Salt

	fmt.Printf("\nTesting ECDH/ECDSA/ECIES\n")
	fmt.Printf("Alice's Passphrase= "+pp)
	fmt.Printf("\n");
	PW:=[]byte(pp)

/* private key S0 of size MGS bytes derived from Password and Salt */

	S0:=ED25519.ECDH_PBKDF2(sha,PW,SALT[:],1000,ED25519.EGS)

	fmt.Printf("Alice's private key= 0x")
	printBinary(S0)

/* Generate Key pair S/W */
	ED25519.ECDH_KEY_PAIR_GENERATE(nil,S0,W0[:])

	fmt.Printf("Alice's public key= 0x")
	printBinary(W0[:]);

	res=ED25519.ECDH_PUBLIC_KEY_VALIDATE(W0[:])
	if res!=0 {
		fmt.Printf("ECP Public Key is invalid!\n")
		return
	}

/* Random private key for other party */
	ED25519.ECDH_KEY_PAIR_GENERATE(rng,S1[:],W1[:])

	fmt.Printf("Servers private key= 0x");
	printBinary(S1[:])

	fmt.Printf("Servers public key= 0x")
	printBinary(W1[:])


	res=ED25519.ECDH_PUBLIC_KEY_VALIDATE(W1[:])
	if res!=0 {
		fmt.Printf("ECP Public Key is invalid!\n")
		return
	}
/* Calculate common key using DH - IEEE 1363 method */

	ED25519.ECDH_ECPSVDP_DH(S0,W1[:],Z0[:])
	ED25519.ECDH_ECPSVDP_DH(S1[:],W0[:],Z1[:])

	same:=true
	for i:=0;i<ED25519.EFS;i++ {
		if Z0[i]!=Z1[i] {same=false}
	}

	if !same {
		fmt.Printf("*** ECPSVDP-DH Failed\n");
		return
	}

	KEY:=ED25519.ECDH_KDF2(sha,Z0[:],nil,ED25519.EAS);

	fmt.Printf("Alice's DH Key=  0x"); printBinary(KEY)
	fmt.Printf("Servers DH Key=  0x"); printBinary(KEY)
	
	if ED25519.CURVETYPE!=ED25519.MONTGOMERY {
		fmt.Printf("Testing ECIES\n");

		P1[0]=0x0; P1[1]=0x1; P1[2]=0x2
		P2[0]=0x0; P2[1]=0x1; P2[2]=0x2; P2[3]=0x3

		for i:=0;i<=16;i++ {M[i]=byte(i)} 

		C:=ED25519.ECDH_ECIES_ENCRYPT(sha,P1[:],P2[:],rng,W1[:],M[:],V[:],T[:])

		fmt.Printf("Ciphertext= \n")
		fmt.Printf("V= 0x"); printBinary(V[:])
		fmt.Printf("C= 0x"); printBinary(C)
		fmt.Printf("T= 0x"); printBinary(T[:])


		RM:=ED25519.ECDH_ECIES_DECRYPT(sha,P1[:],P2[:],V[:],C,T[:],S1[:])
		if RM==nil {
			fmt.Printf("*** ECIES Decryption Failed\n")
			return
		} else {fmt.Printf("Decryption succeeded\n")}

		fmt.Printf("Message is 0x"); printBinary(RM)

		fmt.Printf("Testing ECDSA\n");

		if ED25519.ECDH_ECPSP_DSA(sha,rng,S0,M[:],CS[:],DS[:])!=0 {
			fmt.Printf("***ECDSA Signature Failed\n")
			return
		}
		fmt.Printf("Signature= \n")
		fmt.Printf("C= 0x"); printBinary(CS[:])
		fmt.Printf("D= 0x"); printBinary(DS[:])

		if ED25519.ECDH_ECPVP_DSA(sha,W0[:],M[:],CS[:],DS[:])!=0 {
			fmt.Printf("***ECDSA Verification Failed\n")
			return
		} else {fmt.Printf("ECDSA Signature/Verification succeeded \n")}
	}
}

/* Configure mode of operation */

const PERMITS bool=true
const PINERROR bool=true
const FULL bool=true
const SINGLE_PASS bool=false


func mpin_BN254(rng *amcl.RAND) {

	var sha=BN254.MPIN_HASH_TYPE

	const MGS=BN254.MGS
	const MFS=BN254.MFS
	const G1S=2*MFS+1 /* Group 1 Size */
	const G2S=4*MFS; /* Group 2 Size */
	const EAS int=16

	var S [MGS]byte
	var SST [G2S]byte
	var TOKEN [G1S]byte
	var PERMIT [G1S]byte
	var SEC [G1S]byte
	var xID [G1S]byte
	var xCID [G1S]byte
	var X [MGS]byte
	var Y [MGS]byte
	var E [12*MFS]byte
	var F [12*MFS]byte
	var HID [G1S]byte
	var HTID [G1S]byte

	var G1 [12*MFS]byte
	var G2 [12*MFS]byte
	var R [MGS]byte
	var Z [G1S]byte
	var W [MGS]byte
	var T [G1S]byte
	var CK [EAS]byte
	var SK [EAS]byte

	var HSID []byte


/* Trusted Authority set-up */

	fmt.Printf("\nTesting MPIN\n")
	BN254.MPIN_RANDOM_GENERATE(rng,S[:])
	fmt.Printf("Master Secret s: 0x");  printBinary(S[:])

 /* Create Client Identity */
 	IDstr:= "testUser@miracl.com"
	CLIENT_ID:=[]byte(IDstr)

	HCID:=BN254.MPIN_HASH_ID(sha,CLIENT_ID)  /* Either Client or TA calculates Hash(ID) - you decide! */
		
	fmt.Printf("Client ID= "); printBinary(CLIENT_ID)
	fmt.Printf("\n")

/* Client and Server are issued secrets by DTA */
	BN254.MPIN_GET_SERVER_SECRET(S[:],SST[:])
	fmt.Printf("Server Secret SS: 0x");  printBinary(SST[:])

	BN254.MPIN_GET_CLIENT_SECRET(S[:],HCID,TOKEN[:])
	fmt.Printf("Client Secret CS: 0x");        
	printBinary(TOKEN[:])

/* Client extracts PIN from secret to create Token */
	pin:=1234
	fmt.Printf("Client extracts PIN= %d",pin)
	fmt.Printf("\n")
	rtn:=BN254.MPIN_EXTRACT_PIN(sha,CLIENT_ID,pin,TOKEN[:])
	if rtn != 0 {
		fmt.Printf("FAILURE: EXTRACT_PIN rtn: %d",rtn);
		fmt.Printf("\n")
	}

	fmt.Printf("Client Token TK: 0x")       
	printBinary(TOKEN[:]); 

	if FULL {
		BN254.MPIN_PRECOMPUTE(TOKEN[:],HCID,G1[:],G2[:])
	}

	date:=0
	if PERMITS {
		date=BN254.Today()
/* Client gets "Time Token" permit from DTA */ 
		BN254.MPIN_GET_CLIENT_PERMIT(sha,date,S[:],HCID,PERMIT[:])
		fmt.Printf("Time Permit TP: 0x");  printBinary(PERMIT[:])

/* This encoding makes Time permit look random - Elligator squared */
		BN254.MPIN_ENCODING(rng,PERMIT[:])
		fmt.Printf("Encoded Time Permit TP: 0x");  printBinary(PERMIT[:])
		BN254.MPIN_DECODING(PERMIT[:])
		fmt.Printf("Decoded Time Permit TP: 0x");  printBinary(PERMIT[:])
	}
	pin=0
	fmt.Printf("\nPIN= ")
	fmt.Scanf("%d",&pin)

	pxID:=xID[:]
	pxCID:=xCID[:]
	pHID:=HID[:]
	pHTID:=HTID[:]
	pE:=E[:]
	pF:=F[:]
	pPERMIT:=PERMIT[:]
	var prHID []byte

	if date!=0 {
		prHID=pHTID;
		if !PINERROR {
			pxID=nil
			// pHID=nil
		}
	} else {
		prHID=pHID
		pPERMIT=nil
		pxCID=nil
		pHTID=nil
	}
	if !PINERROR {
		pE=nil
		pF=nil
	}

	if SINGLE_PASS {
		fmt.Printf("MPIN Single Pass\n")
		timeValue:= BN254.MPIN_GET_TIME()
		rtn=BN254.MPIN_CLIENT(sha,date,CLIENT_ID,rng,X[:],pin,TOKEN[:],SEC[:],pxID,pxCID,pPERMIT,timeValue,Y[:])
		if rtn != 0 {
			fmt.Printf("FAILURE: CLIENT rtn: %d\n",rtn)
		}

		if FULL {
			HCID=BN254.MPIN_HASH_ID(sha,CLIENT_ID)
			BN254.MPIN_GET_G1_MULTIPLE(rng,1,R[:],HCID,Z[:])  /* Also Send Z=r.ID to Server, remember random r */
		}

		rtn=BN254.MPIN_SERVER(sha,date,pHID,pHTID,Y[:],SST[:],pxID,pxCID,SEC[:],pE,pF,CLIENT_ID,timeValue)
		if rtn != 0 {
  		    fmt.Printf("FAILURE: SERVER rtn: %d\n",rtn)
		}

		if FULL {
			HSID=BN254.MPIN_HASH_ID(sha,CLIENT_ID);
			BN254.MPIN_GET_G1_MULTIPLE(rng,0,W[:],prHID,T[:]);  /* Also send T=w.ID to client, remember random w  */
		}
	} else {
		fmt.Printf("MPIN Multi Pass\n")
        /* Send U=x.ID to server, and recreate secret from token and pin */
		rtn=BN254.MPIN_CLIENT_1(sha,date,CLIENT_ID,rng,X[:],pin,TOKEN[:],SEC[:],pxID,pxCID,pPERMIT);
		if rtn != 0 {
			fmt.Printf("FAILURE: CLIENT_1 rtn: %d\n",rtn)
		}
  
		if FULL {
			HCID=BN254.MPIN_HASH_ID(sha,CLIENT_ID)
			BN254.MPIN_GET_G1_MULTIPLE(rng,1,R[:],HCID,Z[:])  /* Also Send Z=r.ID to Server, remember random r */
		}
  
        /* Server calculates H(ID) and H(T|H(ID)) (if time permits enabled), and maps them to points on the curve HID and HTID resp. */
		BN254.MPIN_SERVER_1(sha,date,CLIENT_ID,pHID,pHTID)
  
        /* Server generates Random number Y and sends it to Client */
		BN254.MPIN_RANDOM_GENERATE(rng,Y[:]);
  
		if FULL {
			HSID=BN254.MPIN_HASH_ID(sha,CLIENT_ID);
			BN254.MPIN_GET_G1_MULTIPLE(rng,0,W[:],prHID,T[:])  /* Also send T=w.ID to client, remember random w  */
		}
  
       /* Client Second Pass: Inputs Client secret SEC, x and y. Outputs -(x+y)*SEC */
		rtn=BN254.MPIN_CLIENT_2(X[:],Y[:],SEC[:])
		if rtn != 0 {
			fmt.Printf("FAILURE: CLIENT_2 rtn: %d\n",rtn)
		}
  
       /* Server Second pass. Inputs hashed client id, random Y, -(x+y)*SEC, xID and xCID and Server secret SST. E and F help kangaroos to find error. */
       /* If PIN error not required, set E and F = null */
  
		rtn=BN254.MPIN_SERVER_2(date,pHID,pHTID,Y[:],SST[:],pxID,pxCID,SEC[:],pE,pF)
  
		if rtn!=0 {
			fmt.Printf("FAILURE: SERVER_1 rtn: %d\n",rtn)
		}
  
		if rtn == BN254.BAD_PIN {
			fmt.Printf("Server says - Bad Pin. I don't know you. Feck off.\n")
			if PINERROR {
				err:=BN254.MPIN_KANGAROO(E[:],F[:])
				if err!=0 {fmt.Printf("(Client PIN is out by %d)\n",err)}
			}
			return
		} else {
			fmt.Printf("Server says - PIN is good! You really are "+IDstr)
			fmt.Printf("\n")
		}

		if  FULL {
			H:=BN254.MPIN_HASH_ALL(sha,HCID[:],pxID,pxCID,SEC[:],Y[:],Z[:],T[:]);
			BN254.MPIN_CLIENT_KEY(sha,G1[:],G2[:],pin,R[:],X[:],H[:],T[:],CK[:])
			fmt.Printf("Client Key =  0x");  printBinary(CK[:])

			H=BN254.MPIN_HASH_ALL(sha,HSID[:],pxID,pxCID,SEC[:],Y[:],Z[:],T[:]);			
			BN254.MPIN_SERVER_KEY(sha,Z[:],SST[:],W[:],H[:],pHID,pxID,pxCID,SK[:])
			fmt.Printf("Server Key =  0x");  printBinary(SK[:])
		}
	}
}

func rsa_2048(rng *amcl.RAND) {
	var sha=RSA2048.RSA_HASH_TYPE
	message:="Hello World\n"

	pub:=RSA2048.New_public_key(RSA2048.FFLEN)
	priv:=RSA2048.New_private_key(RSA2048.HFLEN)

	var ML [RSA2048.RFS]byte
	var C [RSA2048.RFS]byte
	var S [RSA2048.RFS]byte

	fmt.Printf("\nTesting RSA 2048-bit\n")
	fmt.Printf("Generating public/private key pair\n")
	RSA2048.RSA_KEY_PAIR(rng,65537,priv,pub)

	M:=[]byte(message)

	fmt.Printf("Encrypting test string\n")
	E:=RSA2048.RSA_OAEP_ENCODE(sha,M,rng,nil) /* OAEP encode message M to E  */

	RSA2048.RSA_ENCRYPT(pub,E,C[:])    /* encrypt encoded message */
	fmt.Printf("Ciphertext= 0x"); printBinary(C[:])

	fmt.Printf("Decrypting test string\n");
	RSA2048.RSA_DECRYPT(priv,C[:],ML[:])
	MS:=RSA2048.RSA_OAEP_DECODE(sha,nil,ML[:]) /* OAEP decode message  */

	message=string(MS)
	fmt.Printf(message)

	fmt.Printf("Signing message\n")
	RSA2048.RSA_PKCS15(sha,M,C[:]); 

	RSA2048.RSA_DECRYPT(priv,C[:],S[:])  /* create signature in S */ 

	fmt.Printf("Signature= 0x"); printBinary(S[:])

	RSA2048.RSA_ENCRYPT(pub,S[:],ML[:])

	cmp:=true
	if len(C)!=len(ML) {
		cmp=false
	} else {
		for j:=0;j<len(C);j++ {
			if C[j]!=ML[j] {cmp=false}
		}
	}
	if cmp {
		fmt.Printf("Signature is valid\n")
	} else {
		fmt.Printf("Signature is INVALID\n")
	}

}

func main() {
	rng:=amcl.NewRAND()
	var raw [100]byte
	for i:=0;i<100;i++ {raw[i]=byte(i+1)}
	rng.Seed(100,raw[:])

	mpin_BN254(rng)
	ecdh_ED25519(rng)
	rsa_2048(rng)
}
