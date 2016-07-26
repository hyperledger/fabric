/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ca

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"database/sql"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/hyperledger/fabric/core/crypto/primitives"
	pb "github.com/hyperledger/fabric/membersrvc/protos"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

var (
	// ECertSubjectRole is the ASN1 object identifier of the subject's role.
	//
	ECertSubjectRole = asn1.ObjectIdentifier{2, 1, 3, 4, 5, 6, 7}
)

// ECA is the enrollment certificate authority.
//
type ECA struct {
	*CA
	obcKey          []byte
	obcPriv, obcPub []byte
	gRPCServer      *grpc.Server
}

func initializeECATables(db *sql.DB) error {
	return initializeCommonTables(db)
}

// NewECA sets up a new ECA.
//
func NewECA() *ECA {
	eca := &ECA{CA: NewCA("eca", initializeECATables)}

	{
		// read or create global symmetric encryption key
		var cooked string

		raw, err := ioutil.ReadFile(eca.path + "/obc.aes")
		if err != nil {
			rand := rand.Reader
			key := make([]byte, 32) // AES-256
			rand.Read(key)
			cooked = base64.StdEncoding.EncodeToString(key)

			err = ioutil.WriteFile(eca.path+"/obc.aes", []byte(cooked), 0644)
			if err != nil {
				Panic.Panicln(err)
			}
		} else {
			cooked = string(raw)
		}

		eca.obcKey, err = base64.StdEncoding.DecodeString(cooked)
		if err != nil {
			Panic.Panicln(err)
		}
	}

	{
		// read or create global ECDSA key pair for ECIES
		var priv *ecdsa.PrivateKey
		cooked, err := ioutil.ReadFile(eca.path + "/obc.ecies")
		if err == nil {
			block, _ := pem.Decode(cooked)
			priv, err = x509.ParseECPrivateKey(block.Bytes)
			if err != nil {
				Panic.Panicln(err)
			}
		} else {
			priv, err = ecdsa.GenerateKey(primitives.GetDefaultCurve(), rand.Reader)
			if err != nil {
				Panic.Panicln(err)
			}

			raw, _ := x509.MarshalECPrivateKey(priv)
			cooked = pem.EncodeToMemory(
				&pem.Block{
					Type:  "ECDSA PRIVATE KEY",
					Bytes: raw,
				})
			err := ioutil.WriteFile(eca.path+"/obc.ecies", cooked, 0644)
			if err != nil {
				Panic.Panicln(err)
			}
		}

		eca.obcPriv = cooked
		raw, _ := x509.MarshalPKIXPublicKey(&priv.PublicKey)
		eca.obcPub = pem.EncodeToMemory(
			&pem.Block{
				Type:  "ECDSA PUBLIC KEY",
				Bytes: raw,
			})
	}

	eca.populateAffiliationGroupsTable()
	eca.populateUsersTable()
	return eca
}

// populateUsersTable populates the users table.
//
func (eca *ECA) populateUsersTable() {
	// populate user table
	users := viper.GetStringMapString("eca.users")
	for id, flds := range users {
		vals := strings.Fields(flds)
		role, err := strconv.Atoi(vals[0])
		if err != nil {
			Panic.Panicln(err)
		}
		var affiliation, memberMetadata, registrar string
		if len(vals) >= 3 {
			affiliation = vals[2]
			if len(vals) >= 4 {
				memberMetadata = vals[3]
				if len(vals) >= 5 {
					registrar = vals[4]
				}
			}
		}
		eca.registerUser(id, affiliation, pb.Role(role), registrar, memberMetadata, vals[1])
	}
}

// populateAffiliationGroup populates the affiliation groups table.
//
func (eca *ECA) populateAffiliationGroup(name, parent, key string, level int) {
	eca.registerAffiliationGroup(name, parent)
	newKey := key + "." + name

	if level == 0 {
		affiliationGroups := viper.GetStringSlice(newKey)
		for ci := range affiliationGroups {
			eca.registerAffiliationGroup(affiliationGroups[ci], name)
		}
	} else {
		affiliationGroups := viper.GetStringMapString(newKey)
		for childName := range affiliationGroups {
			eca.populateAffiliationGroup(childName, name, newKey, level-1)
		}
	}
}

// populateAffiliationGroupsTable populates affiliation groups table.
//
func (eca *ECA) populateAffiliationGroupsTable() {
	key := "eca.affiliations"
	affiliationGroups := viper.GetStringMapString(key)
	for name := range affiliationGroups {
		eca.populateAffiliationGroup(name, "", key, 1)
	}
}

// Start starts the ECA.
//
func (eca *ECA) Start(srv *grpc.Server) {
	Info.Println("Starting ECA...")

	eca.startECAP(srv)
	eca.startECAA(srv)
	eca.gRPCServer = srv

	Info.Println("ECA started.")
}

// Stop stops the ECA services.
func (eca *ECA) Stop() {
	Info.Println("Stopping ECA services...")
	if eca.gRPCServer != nil {
		eca.gRPCServer.Stop()
	}
	err := eca.CA.Stop()
	if err != nil {
		Error.Println("ECA Error stopping services ", err)
	} else {
		Info.Println("ECA stopped")
	}
}

func (eca *ECA) startECAP(srv *grpc.Server) {
	pb.RegisterECAPServer(srv, &ECAP{eca})
	Info.Println("ECA PUBLIC gRPC API server started")
}

func (eca *ECA) startECAA(srv *grpc.Server) {
	pb.RegisterECAAServer(srv, &ECAA{eca})
	Info.Println("ECA ADMIN gRPC API server started")
}
