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
	"encoding/asn1"
	"errors"
	"strings"
	"time"

	"crypto/x509"

	"database/sql"

	"github.com/hyperledger/fabric/flogging"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"google.golang.org/grpc"

	"github.com/golang/protobuf/ptypes/timestamp"
	pb "github.com/hyperledger/fabric/membersrvc/protos"
)

var acaLogger = logging.MustGetLogger("aca")

var (
	//ACAAttribute is the base OID to the attributes extensions.
	ACAAttribute = asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6, 10}
)

// ACA is the attribute certificate authority.
type ACA struct {
	*CA
	gRPCServer *grpc.Server
}

// ACAA serves the administrator GRPC interface of the ACA.
//
type ACAA struct {
	aca *ACA
}

//IsAttributeOID returns if the oid passed as parameter is or not linked with an attribute
func IsAttributeOID(oid asn1.ObjectIdentifier) bool {
	l := len(oid)
	if len(ACAAttribute) != l {
		return false
	}
	for i := 0; i < l-1; i++ {
		if ACAAttribute[i] != oid[i] {
			return false
		}
	}

	return ACAAttribute[l-1] < oid[l-1]
}

func initializeACATables(db *sql.DB) error {
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS Attributes (row INTEGER PRIMARY KEY, id VARCHAR(64), affiliation VARCHAR(64), attributeName VARCHAR(64), validFrom DATETIME, validTo DATETIME,  attributeValue BLOB)"); err != nil {
		return err
	}
	return nil
}

//AttributeOwner is the struct that contains the data related with the user who owns the attribute.
type AttributeOwner struct {
	id          string
	affiliation string
}

//AttributePair is an struct that store the relation between an owner (user who owns the attribute), attributeName (name of the attribute), attributeValue (value of the attribute),
//validFrom (time since the attribute is valid) and validTo (time until the attribute will be valid).
type AttributePair struct {
	owner          *AttributeOwner
	attributeName  string
	attributeValue []byte
	validFrom      time.Time
	validTo        time.Time
}

//NewAttributePair creates a new attribute pair associated with <attrOwner>.
func NewAttributePair(attributeVals []string, attrOwner *AttributeOwner) (*AttributePair, error) {
	if len(attributeVals) < 6 {
		return nil, errors.New("Invalid attribute entry")
	}
	var attrPair = *new(AttributePair)
	if attrOwner != nil {
		attrPair.SetOwner(attrOwner)
	} else {
		attrPair.SetOwner(&AttributeOwner{strings.TrimSpace(attributeVals[0]), strings.TrimSpace(attributeVals[1])})
	}
	attrPair.SetAttributeName(strings.TrimSpace(attributeVals[2]))
	attrPair.SetAttributeValue([]byte(strings.TrimSpace(attributeVals[3])))
	//Reading validFrom date
	dateStr := strings.TrimSpace(attributeVals[4])
	if dateStr != "" {
		var t time.Time
		var err error
		if t, err = time.Parse(time.RFC3339, dateStr); err != nil {
			return nil, err
		}
		attrPair.SetValidFrom(t)
	}
	//Reading validTo date
	dateStr = strings.TrimSpace(attributeVals[5])
	if dateStr != "" {
		var t time.Time
		var err error
		if t, err = time.Parse(time.RFC3339, dateStr); err != nil {
			return nil, err
		}
		attrPair.SetValidTo(t)
	}
	return &attrPair, nil
}

//GetID returns the id of the attributeOwner.
func (attrOwner *AttributeOwner) GetID() string {
	return attrOwner.id
}

//GetAffiliation returns the affiliation related with the owner.
func (attrOwner *AttributeOwner) GetAffiliation() string {
	return attrOwner.affiliation
}

//GetOwner returns the owner of the attribute pair.
func (attrPair *AttributePair) GetOwner() *AttributeOwner {
	return attrPair.owner
}

//SetOwner sets the owner of the attributes.
func (attrPair *AttributePair) SetOwner(owner *AttributeOwner) {
	attrPair.owner = owner
}

//GetID returns the id of the attributePair.
func (attrPair *AttributePair) GetID() string {
	return attrPair.owner.GetID()
}

//GetAffiliation gets the affilition of the attribute pair.
func (attrPair *AttributePair) GetAffiliation() string {
	return attrPair.owner.GetAffiliation()
}

//GetAttributeName gets the attribute name related with the attribute pair.
func (attrPair *AttributePair) GetAttributeName() string {
	return attrPair.attributeName
}

//SetAttributeName sets the name related with the attribute pair.
func (attrPair *AttributePair) SetAttributeName(name string) {
	attrPair.attributeName = name
}

//GetAttributeValue returns the value of the pair.
func (attrPair *AttributePair) GetAttributeValue() []byte {
	return attrPair.attributeValue
}

//SetAttributeValue sets the value of the pair.
func (attrPair *AttributePair) SetAttributeValue(val []byte) {
	attrPair.attributeValue = val
}

//IsValidFor returns if the pair is valid for date.
func (attrPair *AttributePair) IsValidFor(date time.Time) bool {
	return (attrPair.validFrom.Before(date) || attrPair.validFrom.Equal(date)) && (attrPair.validTo.IsZero() || attrPair.validTo.After(date))
}

//GetValidFrom returns time which is valid from the pair.
func (attrPair *AttributePair) GetValidFrom() time.Time {
	return attrPair.validFrom
}

//SetValidFrom returns time which is valid from the pair.
func (attrPair *AttributePair) SetValidFrom(date time.Time) {
	attrPair.validFrom = date
}

//GetValidTo returns time which is valid to the pair.
func (attrPair *AttributePair) GetValidTo() time.Time {
	return attrPair.validTo
}

//SetValidTo returns time which is valid to the pair.
func (attrPair *AttributePair) SetValidTo(date time.Time) {
	attrPair.validTo = date
}

//ToACAAttribute converts the receiver to the protobuf format.
func (attrPair *AttributePair) ToACAAttribute() *pb.ACAAttribute {
	var from, to *timestamp.Timestamp
	if attrPair.validFrom.IsZero() {
		from = nil
	} else {
		from = &timestamp.Timestamp{Seconds: attrPair.validFrom.Unix(), Nanos: int32(attrPair.validFrom.UnixNano())}
	}
	if attrPair.validTo.IsZero() {
		to = nil
	} else {
		to = &timestamp.Timestamp{Seconds: attrPair.validTo.Unix(), Nanos: int32(attrPair.validTo.UnixNano())}

	}
	return &pb.ACAAttribute{AttributeName: attrPair.attributeName, AttributeValue: attrPair.attributeValue, ValidFrom: from, ValidTo: to}
}

// NewACA sets up a new ACA.
func NewACA() *ACA {
	aca := &ACA{CA: NewCA("aca", initializeACATables)}
	flogging.LoggingInit("aca")
	return aca
}

func (aca *ACA) getECACertificate() (*x509.Certificate, error) {
	raw, err := aca.readCACertificate("eca")
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(raw)
}

func (aca *ACA) getTCACertificate() (*x509.Certificate, error) {
	raw, err := aca.readCACertificate("tca")
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(raw)
}

func (aca *ACA) fetchAttributes(id, affiliation string) ([]*AttributePair, error) {
	// TODO this attributes should be readed from the outside world in place of configuration file.
	var attributes = make([]*AttributePair, 0)
	attrs := viper.GetStringMapString("aca.attributes")

	for _, flds := range attrs {
		vals := strings.Fields(flds)
		if len(vals) >= 1 {
			val := ""
			for _, eachVal := range vals {
				val = val + " " + eachVal
			}
			attributeVals := strings.Split(val, ";")
			if len(attributeVals) >= 6 {
				attrPair, err := NewAttributePair(attributeVals, nil)
				if err != nil {
					return nil, errors.New("Invalid attribute entry " + val + " " + err.Error())
				}
				if attrPair.GetID() != id || attrPair.GetAffiliation() != affiliation {
					continue
				}
				attributes = append(attributes, attrPair)
			} else {
				acaLogger.Errorf("Invalid attribute entry '%v'", vals[0])
			}
		}
	}

	acaLogger.Debugf("%v %v", id, attributes)

	return attributes, nil
}

func (aca *ACA) PopulateAttributes(attrs []*AttributePair) error {

	acaLogger.Debugf("PopulateAttributes: %+v", attrs)

	mutex.Lock()
	defer mutex.Unlock()

	tx, dberr := aca.db.Begin()
	if dberr != nil {
		return dberr
	}
	for _, attr := range attrs {
		acaLogger.Debugf("attr: %+v", attr)
		if err := aca.populateAttribute(tx, attr); err != nil {
			dberr = tx.Rollback()
			if dberr != nil {
				return dberr
			}
			return err
		}
	}
	dberr = tx.Commit()
	if dberr != nil {
		return dberr
	}
	return nil
}

func (aca *ACA) populateAttribute(tx *sql.Tx, attr *AttributePair) error {
	var count int
	err := tx.QueryRow("SELECT count(row) AS cant FROM Attributes WHERE id=? AND affiliation =? AND attributeName =?",
		attr.GetID(), attr.GetAffiliation(), attr.GetAttributeName()).Scan(&count)

	if err != nil {
		return err
	}

	if count > 0 {
		_, err = tx.Exec("UPDATE Attributes SET validFrom = ?, validTo = ?,  attributeValue = ? WHERE  id=? AND affiliation =? AND attributeName =? AND validFrom < ?",
			attr.GetValidFrom(), attr.GetValidTo(), attr.GetAttributeValue(), attr.GetID(), attr.GetAffiliation(), attr.GetAttributeName(), attr.GetValidFrom())
		if err != nil {
			return err
		}
	} else {
		_, err = tx.Exec("INSERT INTO Attributes (validFrom , validTo,  attributeValue, id, affiliation, attributeName) VALUES (?,?,?,?,?,?)",
			attr.GetValidFrom(), attr.GetValidTo(), attr.GetAttributeValue(), attr.GetID(), attr.GetAffiliation(), attr.GetAttributeName())
		if err != nil {
			return err
		}
	}
	return nil
}

func (aca *ACA) fetchAndPopulateAttributes(id, affiliation string) error {
	var attrs []*AttributePair
	attrs, err := aca.fetchAttributes(id, affiliation)
	if err != nil {
		return err
	}
	err = aca.PopulateAttributes(attrs)
	if err != nil {
		return err
	}
	return nil
}

func (aca *ACA) findAttribute(owner *AttributeOwner, attributeName string) (*AttributePair, error) {
	var count int

	mutex.RLock()
	defer mutex.RUnlock()

	err := aca.db.QueryRow("SELECT count(row) AS cant FROM Attributes WHERE id=? AND affiliation =? AND attributeName =?",
		owner.GetID(), owner.GetAffiliation(), attributeName).Scan(&count)
	if err != nil {
		return nil, err
	}

	if count == 0 {
		return nil, nil
	}

	var attName string
	var attValue []byte
	var validFrom, validTo time.Time
	err = aca.db.QueryRow("SELECT attributeName, attributeValue, validFrom, validTo AS cant FROM Attributes WHERE id=? AND affiliation =? AND attributeName =?",
		owner.GetID(), owner.GetAffiliation(), attributeName).Scan(&attName, &attValue, &validFrom, &validTo)
	if err != nil {
		return nil, err
	}

	return &AttributePair{owner, attName, attValue, validFrom, validTo}, nil
}

func (aca *ACA) startACAP(srv *grpc.Server) {
	pb.RegisterACAPServer(srv, &ACAP{aca})
	acaLogger.Info("ACA PUBLIC gRPC API server started")
}

// Start starts the ACA.
func (aca *ACA) Start(srv *grpc.Server) {
	acaLogger.Info("Staring ACA services...")
	aca.startACAP(srv)
	aca.gRPCServer = srv
	acaLogger.Info("ACA services started")
}

// Stop stops the ACA
func (aca *ACA) Stop() error {
	acaLogger.Info("Stopping the ACA services...")
	if aca.gRPCServer != nil {
		aca.gRPCServer.Stop()
	}
	err := aca.CA.Stop()
	if err != nil {
		acaLogger.Errorf("Error stopping the ACA services: %s ", err)
	} else {
		acaLogger.Info("ACA services stopped")
	}
	return err
}
