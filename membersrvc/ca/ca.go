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
	"crypto/x509/pkix"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	gp "google/protobuf"

	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/flogging"
	pb "github.com/hyperledger/fabric/membersrvc/protos"
	_ "github.com/mattn/go-sqlite3" // This blank import is required to load sqlite3 driver
	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

var caLogger = logging.MustGetLogger("ca")

// CA is the base certificate authority.
type CA struct {
	db *sql.DB

	path string

	priv *ecdsa.PrivateKey
	cert *x509.Certificate
	raw  []byte
}

// CertificateSpec defines the parameter used to create a new certificate.
type CertificateSpec struct {
	id           string
	commonName   string
	serialNumber *big.Int
	pub          interface{}
	usage        x509.KeyUsage
	NotBefore    *time.Time
	NotAfter     *time.Time
	ext          *[]pkix.Extension
}

// AffiliationGroup struct
type AffiliationGroup struct {
	name     string
	parentID int64
	parent   *AffiliationGroup
	preKey   []byte
}

var (
	mutex          = &sync.RWMutex{}
	caOrganization string
	caCountry      string
	rootPath       string
	caDir          string
)

// NewCertificateSpec creates a new certificate spec
func NewCertificateSpec(id string, commonName string, serialNumber *big.Int, pub interface{}, usage x509.KeyUsage, notBefore *time.Time, notAfter *time.Time, opt ...pkix.Extension) *CertificateSpec {
	spec := new(CertificateSpec)
	spec.id = id
	spec.commonName = commonName
	spec.serialNumber = serialNumber
	spec.pub = pub
	spec.usage = usage
	spec.NotBefore = notBefore
	spec.NotAfter = notAfter
	spec.ext = &opt
	return spec
}

// NewDefaultPeriodCertificateSpec creates a new certificate spec with notBefore a minute ago and not after 90 days from notBefore.
//
func NewDefaultPeriodCertificateSpec(id string, serialNumber *big.Int, pub interface{}, usage x509.KeyUsage, opt ...pkix.Extension) *CertificateSpec {
	return NewDefaultPeriodCertificateSpecWithCommonName(id, id, serialNumber, pub, usage, opt...)
}

// NewDefaultPeriodCertificateSpecWithCommonName creates a new certificate spec with notBefore a minute ago and not after 90 days from notBefore and a specifc commonName.
//
func NewDefaultPeriodCertificateSpecWithCommonName(id string, commonName string, serialNumber *big.Int, pub interface{}, usage x509.KeyUsage, opt ...pkix.Extension) *CertificateSpec {
	notBefore := time.Now().Add(-1 * time.Minute)
	notAfter := notBefore.Add(time.Hour * 24 * 90)
	return NewCertificateSpec(id, commonName, serialNumber, pub, usage, &notBefore, &notAfter, opt...)
}

// NewDefaultCertificateSpec creates a new certificate spec with serialNumber = 1, notBefore a minute ago and not after 90 days from notBefore.
//
func NewDefaultCertificateSpec(id string, pub interface{}, usage x509.KeyUsage, opt ...pkix.Extension) *CertificateSpec {
	serialNumber := big.NewInt(1)
	return NewDefaultPeriodCertificateSpec(id, serialNumber, pub, usage, opt...)
}

// NewDefaultCertificateSpecWithCommonName creates a new certificate spec with serialNumber = 1, notBefore a minute ago and not after 90 days from notBefore and a specific commonName.
//
func NewDefaultCertificateSpecWithCommonName(id string, commonName string, pub interface{}, usage x509.KeyUsage, opt ...pkix.Extension) *CertificateSpec {
	serialNumber := big.NewInt(1)
	return NewDefaultPeriodCertificateSpecWithCommonName(id, commonName, serialNumber, pub, usage, opt...)
}

// CacheConfiguration caches the viper configuration
func CacheConfiguration() {
	caOrganization = viper.GetString("pki.ca.subject.organization")
	caCountry = viper.GetString("pki.ca.subject.country")
	rootPath = viper.GetString("server.rootpath")
	caDir = viper.GetString("server.cadir")
}

// GetID returns the spec's ID field/value
//
func (spec *CertificateSpec) GetID() string {
	return spec.id
}

// GetCommonName returns the spec's Common Name field/value
//
func (spec *CertificateSpec) GetCommonName() string {
	return spec.commonName
}

// GetSerialNumber returns the spec's Serial Number field/value
//
func (spec *CertificateSpec) GetSerialNumber() *big.Int {
	return spec.serialNumber
}

// GetPublicKey returns the spec's Public Key field/value
//
func (spec *CertificateSpec) GetPublicKey() interface{} {
	return spec.pub
}

// GetUsage returns the spec's usage (which is the x509.KeyUsage) field/value
//
func (spec *CertificateSpec) GetUsage() x509.KeyUsage {
	return spec.usage
}

// GetNotBefore returns the spec NotBefore (time.Time) field/value
//
func (spec *CertificateSpec) GetNotBefore() *time.Time {
	return spec.NotBefore
}

// GetNotAfter returns the spec NotAfter (time.Time) field/value
//
func (spec *CertificateSpec) GetNotAfter() *time.Time {
	return spec.NotAfter
}

// GetOrganization returns the spec's Organization field/value
//
func (spec *CertificateSpec) GetOrganization() string {
	return caOrganization
}

// GetCountry returns the spec's Country field/value
//
func (spec *CertificateSpec) GetCountry() string {
	return caCountry
}

// GetSubjectKeyID returns the spec's subject KeyID
//
func (spec *CertificateSpec) GetSubjectKeyID() *[]byte {
	return &[]byte{1, 2, 3, 4}
}

// GetSignatureAlgorithm returns the X509.SignatureAlgorithm field/value
//
func (spec *CertificateSpec) GetSignatureAlgorithm() x509.SignatureAlgorithm {
	return x509.ECDSAWithSHA384
}

// GetExtensions returns the sepc's extensions
//
func (spec *CertificateSpec) GetExtensions() *[]pkix.Extension {
	return spec.ext
}

// TableInitializer is a function type for table initialization
type TableInitializer func(*sql.DB) error

func initializeCommonTables(db *sql.DB) error {
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS Certificates (row INTEGER PRIMARY KEY, id VARCHAR(64), timestamp INTEGER, usage INTEGER, cert BLOB, hash BLOB, kdfkey BLOB)"); err != nil {
		return err
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS Users (row INTEGER PRIMARY KEY, id VARCHAR(64), enrollmentId VARCHAR(100), role INTEGER, metadata VARCHAR(256), token BLOB, state INTEGER, key BLOB)"); err != nil {
		return err
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS AffiliationGroups (row INTEGER PRIMARY KEY, name VARCHAR(64), parent INTEGER, FOREIGN KEY(parent) REFERENCES AffiliationGroups(row))"); err != nil {
		return err
	}
	return nil
}

// NewCA sets up a new CA.
func NewCA(name string, initTables TableInitializer) *CA {
	ca := new(CA)
	flogging.LoggingInit("ca")
	ca.path = filepath.Join(rootPath, caDir)

	if _, err := os.Stat(ca.path); err != nil {
		caLogger.Info("Fresh start; creating databases, key pairs, and certificates.")

		if err := os.MkdirAll(ca.path, 0755); err != nil {
			caLogger.Panic(err)
		}
	}

	// open or create certificate database
	db, err := sql.Open("sqlite3", ca.path+"/"+name+".db")
	if err != nil {
		caLogger.Panic(err)
	}

	if err = db.Ping(); err != nil {
		caLogger.Panic(err)
	}

	if err = initTables(db); err != nil {
		caLogger.Panic(err)
	}
	ca.db = db

	// read or create signing key pair
	priv, err := ca.readCAPrivateKey(name)
	if err != nil {
		priv = ca.createCAKeyPair(name)
	}
	ca.priv = priv

	// read CA certificate, or create a self-signed CA certificate
	raw, err := ca.readCACertificate(name)
	if err != nil {
		raw = ca.createCACertificate(name, &ca.priv.PublicKey)
	}
	cert, err := x509.ParseCertificate(raw)
	if err != nil {
		caLogger.Panic(err)
	}

	ca.raw = raw
	ca.cert = cert

	return ca
}

// Stop Close closes down the CA.
func (ca *CA) Stop() error {
	err := ca.db.Close()
	if err == nil {
		caLogger.Debug("Shutting down CA - Successfully")
	} else {
		caLogger.Debug(fmt.Sprintf("Shutting down CA - Error closing DB [%s]", err))
	}
	return err
}

func (ca *CA) createCAKeyPair(name string) *ecdsa.PrivateKey {
	caLogger.Debug("Creating CA key pair.")

	curve := primitives.GetDefaultCurve()

	priv, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err == nil {
		raw, _ := x509.MarshalECPrivateKey(priv)
		cooked := pem.EncodeToMemory(
			&pem.Block{
				Type:  "ECDSA PRIVATE KEY",
				Bytes: raw,
			})
		err = ioutil.WriteFile(ca.path+"/"+name+".priv", cooked, 0644)
		if err != nil {
			caLogger.Panic(err)
		}

		raw, _ = x509.MarshalPKIXPublicKey(&priv.PublicKey)
		cooked = pem.EncodeToMemory(
			&pem.Block{
				Type:  "ECDSA PUBLIC KEY",
				Bytes: raw,
			})
		err = ioutil.WriteFile(ca.path+"/"+name+".pub", cooked, 0644)
		if err != nil {
			caLogger.Panic(err)
		}
	}
	if err != nil {
		caLogger.Panic(err)
	}

	return priv
}

func (ca *CA) readCAPrivateKey(name string) (*ecdsa.PrivateKey, error) {
	caLogger.Debug("Reading CA private key.")

	cooked, err := ioutil.ReadFile(ca.path + "/" + name + ".priv")
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(cooked)
	return x509.ParseECPrivateKey(block.Bytes)
}

func (ca *CA) createCACertificate(name string, pub *ecdsa.PublicKey) []byte {
	caLogger.Debug("Creating CA certificate.")

	raw, err := ca.newCertificate(name, pub, x509.KeyUsageDigitalSignature|x509.KeyUsageCertSign, nil)
	if err != nil {
		caLogger.Panic(err)
	}

	cooked := pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: raw,
		})
	err = ioutil.WriteFile(ca.path+"/"+name+".cert", cooked, 0644)
	if err != nil {
		caLogger.Panic(err)
	}

	return raw
}

func (ca *CA) readCACertificate(name string) ([]byte, error) {
	caLogger.Debug("Reading CA certificate.")

	cooked, err := ioutil.ReadFile(ca.path + "/" + name + ".cert")
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(cooked)
	return block.Bytes, nil
}

func (ca *CA) createCertificate(id string, pub interface{}, usage x509.KeyUsage, timestamp int64, kdfKey []byte, opt ...pkix.Extension) ([]byte, error) {
	spec := NewDefaultCertificateSpec(id, pub, usage, opt...)
	return ca.createCertificateFromSpec(spec, timestamp, kdfKey, true)
}

func (ca *CA) createCertificateFromSpec(spec *CertificateSpec, timestamp int64, kdfKey []byte, persist bool) ([]byte, error) {
	caLogger.Debug("Creating certificate for " + spec.GetID() + ".")

	raw, err := ca.newCertificateFromSpec(spec)
	if err != nil {
		caLogger.Error(err)
		return nil, err
	}

	if persist {
		err = ca.persistCertificate(spec.GetID(), timestamp, spec.GetUsage(), raw, kdfKey)
	}

	return raw, err
}

func (ca *CA) persistCertificate(id string, timestamp int64, usage x509.KeyUsage, certRaw []byte, kdfKey []byte) error {
	mutex.Lock()
	defer mutex.Unlock()

	hash := primitives.NewHash()
	hash.Write(certRaw)
	var err error

	if _, err = ca.db.Exec("INSERT INTO Certificates (id, timestamp, usage, cert, hash, kdfkey) VALUES (?, ?, ?, ?, ?, ?)", id, timestamp, usage, certRaw, hash.Sum(nil), kdfKey); err != nil {
		caLogger.Error(err)
	}
	return err
}

func (ca *CA) newCertificate(id string, pub interface{}, usage x509.KeyUsage, ext []pkix.Extension) ([]byte, error) {
	spec := NewDefaultCertificateSpec(id, pub, usage, ext...)
	return ca.newCertificateFromSpec(spec)
}

func (ca *CA) newCertificateFromSpec(spec *CertificateSpec) ([]byte, error) {
	notBefore := spec.GetNotBefore()
	notAfter := spec.GetNotAfter()

	parent := ca.cert
	isCA := parent == nil

	tmpl := x509.Certificate{
		SerialNumber: spec.GetSerialNumber(),
		Subject: pkix.Name{
			CommonName:   spec.GetCommonName(),
			Organization: []string{spec.GetOrganization()},
			Country:      []string{spec.GetCountry()},
		},
		NotBefore: *notBefore,
		NotAfter:  *notAfter,

		SubjectKeyId:       *spec.GetSubjectKeyID(),
		SignatureAlgorithm: spec.GetSignatureAlgorithm(),
		KeyUsage:           spec.GetUsage(),

		BasicConstraintsValid: true,
		IsCA: isCA,
	}

	if len(*spec.GetExtensions()) > 0 {
		tmpl.Extensions = *spec.GetExtensions()
		tmpl.ExtraExtensions = *spec.GetExtensions()
	}
	if isCA {
		parent = &tmpl
	}

	raw, err := x509.CreateCertificate(
		rand.Reader,
		&tmpl,
		parent,
		spec.GetPublicKey(),
		ca.priv,
	)
	if isCA && err != nil {
		caLogger.Panic(err)
	}

	return raw, err
}

func (ca *CA) readCertificateByKeyUsage(id string, usage x509.KeyUsage) ([]byte, error) {
	caLogger.Debugf("Reading certificate for %s and usage %v", id, usage)

	mutex.RLock()
	defer mutex.RUnlock()

	var raw []byte
	err := ca.db.QueryRow("SELECT cert FROM Certificates WHERE id=? AND usage=?", id, usage).Scan(&raw)

	if err != nil {
		caLogger.Debugf("readCertificateByKeyUsage() Error: %v", err)
	}

	return raw, err
}

func (ca *CA) readCertificateByTimestamp(id string, ts int64) ([]byte, error) {
	caLogger.Debug("Reading certificate for " + id + ".")

	mutex.RLock()
	defer mutex.RUnlock()

	var raw []byte
	err := ca.db.QueryRow("SELECT cert FROM Certificates WHERE id=? AND timestamp=?", id, ts).Scan(&raw)

	return raw, err
}

func (ca *CA) readCertificates(id string, opt ...int64) (*sql.Rows, error) {
	caLogger.Debug("Reading certificatess for " + id + ".")

	mutex.RLock()
	defer mutex.RUnlock()

	if len(opt) > 0 && opt[0] != 0 {
		return ca.db.Query("SELECT cert, kdfkey FROM Certificates WHERE id=? AND timestamp=? ORDER BY usage", id, opt[0])
	}

	return ca.db.Query("SELECT cert, kdfkey FROM Certificates WHERE id=?", id)
}

func (ca *CA) readCertificateSets(id string, start, end int64) (*sql.Rows, error) {
	caLogger.Debug("Reading certificate sets for " + id + ".")

	mutex.RLock()
	defer mutex.RUnlock()

	return ca.db.Query("SELECT cert, kdfKey, timestamp FROM Certificates WHERE id=? AND timestamp BETWEEN ? AND ? ORDER BY timestamp", id, start, end)
}

func (ca *CA) readCertificateByHash(hash []byte) ([]byte, error) {
	caLogger.Debug("Reading certificate for hash " + string(hash) + ".")

	mutex.RLock()
	defer mutex.RUnlock()

	var raw []byte
	row := ca.db.QueryRow("SELECT cert FROM Certificates WHERE hash=?", hash)
	err := row.Scan(&raw)

	return raw, err
}

func (ca *CA) isValidAffiliation(affiliation string) (bool, error) {
	caLogger.Debug("Validating affiliation: " + affiliation)

	mutex.RLock()
	defer mutex.RUnlock()

	var count int
	var err error
	err = ca.db.QueryRow("SELECT count(row) FROM AffiliationGroups WHERE name=?", affiliation).Scan(&count)
	if err != nil {
		caLogger.Debug("Affiliation <" + affiliation + "> is INVALID.")

		return false, err
	}
	caLogger.Debug("Affiliation <" + affiliation + "> is VALID.")

	return count == 1, nil
}

//
// Determine if affiliation is required for a given registration request.
//
// Affiliation is required if the role is client or peer.
// Affiliation is not required if the role is validator or auditor.
// 1: client, 2: peer, 4: validator, 8: auditor
//

func (ca *CA) requireAffiliation(role pb.Role) bool {
	roleStr, _ := MemberRoleToString(role)
	caLogger.Debug("Assigned role is: " + roleStr + ".")

	return role != pb.Role_VALIDATOR && role != pb.Role_AUDITOR
}

// validateAndGenerateEnrollID validates the affiliation subject
func (ca *CA) validateAndGenerateEnrollID(id, affiliation string, role pb.Role) (string, error) {
	roleStr, _ := MemberRoleToString(role)
	caLogger.Debug("Validating and generating enrollID for user id: " + id + ", affiliation: " + affiliation + ", role: " + roleStr + ".")

	// Check whether the affiliation is required for the current user.
	//
	// Affiliation is required if the role is client or peer.
	// Affiliation is not required if the role is validator or auditor.
	if ca.requireAffiliation(role) {
		valid, err := ca.isValidAffiliation(affiliation)
		if err != nil {
			return "", err
		}

		if !valid {
			caLogger.Debug("Invalid affiliation group: ")
			return "", errors.New("Invalid affiliation group " + affiliation)
		}

		return ca.generateEnrollID(id, affiliation)
	}

	return "", nil
}

// registerUser registers a new member with the CA
//
func (ca *CA) registerUser(id, affiliation string, role pb.Role, attrs []*pb.Attribute, aca *ACA, registrar, memberMetadata string, opt ...string) (string, error) {
	memberMetadata = removeQuotes(memberMetadata)
	roleStr, _ := MemberRoleToString(role)
	caLogger.Debugf("Received request to register user with id: %s, affiliation: %s, role: %s, attrs: %+v, registrar: %s, memberMetadata: %s\n",
		id, affiliation, roleStr, attrs, registrar, memberMetadata)

	var enrollID, tok string
	var err error

	// There are two ways that registerUser can be called:
	// 1) At initialization time from eca.users in the YAML file
	//    In this case, 'registrar' may be nil but we still register the users from the YAML file
	// 2) At runtime via the GRPC ECA.RegisterUser handler (see RegisterUser in eca.go)
	//    In this case, 'registrar' must never be nil and furthermore the caller must have been authenticated
	//    to actually be the 'registrar' identity
	// This means we trust what is in the YAML file but not what comes over the network
	if registrar != "" {
		// Check the permission of member named 'registrar' to perform this registration
		err = ca.canRegister(registrar, role2String(int(role)), memberMetadata)
		if err != nil {
			return "", err
		}
	}

	enrollID, err = ca.validateAndGenerateEnrollID(id, affiliation, role)
	if err != nil {
		return "", err
	}

	tok, err = ca.registerUserWithEnrollID(id, enrollID, role, memberMetadata, opt...)
	if err != nil {
		return "", err
	}

	if attrs != nil && aca != nil {
		var pairs []*AttributePair
		pairs, err = toAttributePairs(id, affiliation, attrs)
		if err == nil {
			err = aca.PopulateAttributes(pairs)
		}
	}

	return tok, err
}

// registerUserWithEnrollID registers a new user and its enrollmentID, role and state
//
func (ca *CA) registerUserWithEnrollID(id string, enrollID string, role pb.Role, memberMetadata string, opt ...string) (string, error) {
	mutex.Lock()
	defer mutex.Unlock()

	roleStr, _ := MemberRoleToString(role)
	caLogger.Debugf("Registering user %s as %s with memberMetadata %s\n", id, roleStr, memberMetadata)

	var tok string
	if len(opt) > 0 && len(opt[0]) > 0 {
		tok = opt[0]
	} else {
		tok = randomString(12)
	}

	var row int
	err := ca.db.QueryRow("SELECT row FROM Users WHERE id=?", id).Scan(&row)
	if err == nil {
		return "", errors.New("User is already registered")
	}

	_, err = ca.db.Exec("INSERT INTO Users (id, enrollmentId, token, role, metadata, state) VALUES (?, ?, ?, ?, ?, ?)", id, enrollID, tok, role, memberMetadata, 0)

	if err != nil {
		caLogger.Error(err)
	}

	return tok, err
}

// registerAffiliationGroup registers a new affiliation group
//
func (ca *CA) registerAffiliationGroup(name string, parentName string) error {
	mutex.Lock()
	defer mutex.Unlock()

	caLogger.Debug("Registering affiliation group " + name + " parent " + parentName + ".")

	var parentID int
	var err error
	var count int
	err = ca.db.QueryRow("SELECT count(row) FROM AffiliationGroups WHERE name=?", name).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("Affiliation group is already registered")
	}

	if strings.Compare(parentName, "") != 0 {
		err = ca.db.QueryRow("SELECT row FROM AffiliationGroups WHERE name=?", parentName).Scan(&parentID)
		if err != nil {
			return err
		}
	}

	_, err = ca.db.Exec("INSERT INTO AffiliationGroups (name, parent) VALUES (?, ?)", name, parentID)

	if err != nil {
		caLogger.Error(err)
	}

	return err

}

// deleteUser deletes a user given a name
//
func (ca *CA) deleteUser(id string) error {
	caLogger.Debug("Deleting user " + id + ".")

	mutex.Lock()
	defer mutex.Unlock()

	var row int
	err := ca.db.QueryRow("SELECT row FROM Users WHERE id=?", id).Scan(&row)
	if err == nil {
		_, err = ca.db.Exec("DELETE FROM Certificates Where id=?", id)
		if err != nil {
			caLogger.Error(err)
		}

		_, err = ca.db.Exec("DELETE FROM Users WHERE row=?", row)
		if err != nil {
			caLogger.Error(err)
		}
	}

	return err
}

// readUser reads a token given an id
//
func (ca *CA) readUser(id string) *sql.Row {
	caLogger.Debug("Reading token for " + id + ".")

	mutex.RLock()
	defer mutex.RUnlock()

	return ca.db.QueryRow("SELECT role, token, state, key, enrollmentId FROM Users WHERE id=?", id)
}

// readUsers reads users of a given Role
//
func (ca *CA) readUsers(role int) (*sql.Rows, error) {
	caLogger.Debug("Reading users matching role " + strconv.FormatInt(int64(role), 2) + ".")

	return ca.db.Query("SELECT id, role FROM Users WHERE role&?!=0", role)
}

// readRole returns the user Role given a user id
//
func (ca *CA) readRole(id string) int {
	caLogger.Debug("Reading role for " + id + ".")

	mutex.RLock()
	defer mutex.RUnlock()

	var role int
	ca.db.QueryRow("SELECT role FROM Users WHERE id=?", id).Scan(&role)

	return role
}

func (ca *CA) readAffiliationGroups() ([]*AffiliationGroup, error) {
	caLogger.Debug("Reading affilition groups.")

	rows, err := ca.db.Query("SELECT row, name, parent FROM AffiliationGroups")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := make(map[int64]*AffiliationGroup)

	for rows.Next() {
		group := new(AffiliationGroup)
		var id int64
		if e := rows.Scan(&id, &group.name, &group.parentID); e != nil {
			return nil, err
		}
		groups[id] = group
	}

	groupList := make([]*AffiliationGroup, len(groups))
	idx := 0
	for _, eachGroup := range groups {
		eachGroup.parent = groups[eachGroup.parentID]
		groupList[idx] = eachGroup
		idx++
	}

	return groupList, nil
}

func (ca *CA) generateEnrollID(id string, affiliation string) (string, error) {
	if id == "" || affiliation == "" {
		return "", errors.New("Please provide all the input parameters, id and role")
	}

	if strings.Contains(id, "\\") || strings.Contains(affiliation, "\\") {
		return "", errors.New("Do not include the escape character \\ as part of the values")
	}

	return id + "\\" + affiliation, nil
}

func (ca *CA) parseEnrollID(enrollID string) (id string, affiliation string, err error) {

	if enrollID == "" {
		return "", "", errors.New("Input parameter missing")
	}

	enrollIDSections := strings.Split(enrollID, "\\")

	if len(enrollIDSections) != 2 {
		return "", "", errors.New("Either the userId or affiliation is missing from the enrollmentID. EnrollID was " + enrollID)
	}

	id = enrollIDSections[0]
	affiliation = enrollIDSections[1]
	err = nil
	return
}

// Check to see if member 'registrar' can register a new member of type 'newMemberRole'
// and with metadata associated with 'newMemberMetadataStr'
// Return nil if allowed, or an error if not allowed
func (ca *CA) canRegister(registrar string, newMemberRole string, newMemberMetadataStr string) error {
	mutex.RLock()
	defer mutex.RUnlock()

	// Read the user metadata associated with 'registrar'
	var registrarMetadataStr string
	err := ca.db.QueryRow("SELECT metadata FROM Users WHERE id=?", registrar).Scan(&registrarMetadataStr)
	if err != nil {
		caLogger.Debugf("CA.canRegister: db error: %s\n", err.Error())
		return err
	}
	caLogger.Debugf("CA.canRegister: registrar=%s, registrarMD=%s, newMemberRole=%s, newMemberMD=%s",
		registrar, registrarMetadataStr, newMemberRole, newMemberMetadataStr)
	// If isn't a registrar at all, then error
	if registrarMetadataStr == "" {
		caLogger.Debug("canRegister: member " + registrar + " is not a registrar")
		return errors.New("member " + registrar + " is not a registrar")
	}
	// Get the registrar's metadata
	caLogger.Debug("CA.canRegister: parsing registrar's metadata")
	registrarMetadata, err := newMemberMetadata(registrarMetadataStr)
	if err != nil {
		return err
	}
	// Convert the user's meta to an object
	caLogger.Debug("CA.canRegister: parsing new member's metadata")
	newMemberMetadata, err := newMemberMetadata(newMemberMetadataStr)
	if err != nil {
		return err
	}
	// See if the metadata to be registered is acceptable for the registrar
	return registrarMetadata.canRegister(registrar, newMemberRole, newMemberMetadata)
}

// Convert a string to a MemberMetadata
func newMemberMetadata(metadata string) (*MemberMetadata, error) {
	if metadata == "" {
		caLogger.Debug("newMemberMetadata: nil")
		return nil, nil
	}
	var mm MemberMetadata
	err := json.Unmarshal([]byte(metadata), &mm)
	if err != nil {
		caLogger.Debugf("newMemberMetadata: error: %s, metadata: %s\n", err.Error(), metadata)
	}
	caLogger.Debugf("newMemberMetadata: metadata=%s, object=%+v\n", metadata, mm)
	return &mm, err
}

// MemberMetadata Additional member metadata
type MemberMetadata struct {
	Registrar Registrar `json:"registrar"`
}

// Registrar metadata
type Registrar struct {
	Roles         []string `json:"roles"`
	DelegateRoles []string `json:"delegateRoles"`
}

// See if member 'registrar' can register a member of type 'newRole'
// with MemberMetadata of 'newMemberMetadata'
func (mm *MemberMetadata) canRegister(registrar string, newRole string, newMemberMetadata *MemberMetadata) error {
	// Can register a member of this type?
	caLogger.Debugf("MM.canRegister registrar=%s, newRole=%s\n", registrar, newRole)
	if !strContained(newRole, mm.Registrar.Roles) {
		caLogger.Debugf("MM.canRegister: role %s can't be registered by %s\n", newRole, registrar)
		return errors.New("member " + registrar + " may not register member of type " + newRole)
	}

	// The registrar privileges that are being registered must not be larger than the registrar's
	if newMemberMetadata == nil {
		// Not requesting registrar privileges for this member, so we are OK
		caLogger.Debug("MM.canRegister: not requesting registrar privileges")
		return nil
	}

	// Make sure this registrar is not delegating an invalid role
	err := checkDelegateRoles(newMemberMetadata.Registrar.Roles, mm.Registrar.DelegateRoles, registrar)
	if err != nil {
		caLogger.Debug("MM.canRegister: checkDelegateRoles failure")
		return err
	}

	// Can register OK
	caLogger.Debug("MM.canRegister: OK")
	return nil
}

// Return an error if all strings in 'strs1' are not contained in 'strs2'
func checkDelegateRoles(strs1 []string, strs2 []string, registrar string) error {
	caLogger.Debugf("CA.checkDelegateRoles: registrar=%s, strs1=%+v, strs2=%+v\n", registrar, strs1, strs2)
	for _, s := range strs1 {
		if !strContained(s, strs2) {
			caLogger.Debugf("CA.checkDelegateRoles: no: %s not in %+v\n", s, strs2)
			return errors.New("user " + registrar + " may not register delegateRoles " + s)
		}
	}
	caLogger.Debug("CA.checkDelegateRoles: ok")
	return nil
}

// Return true if 'str' is in 'strs'; otherwise return false
func strContained(str string, strs []string) bool {
	for _, s := range strs {
		if s == str {
			return true
		}
	}
	return false
}

// Return true if 'str' is prefixed by any string in 'strs'; otherwise return false
func isPrefixed(str string, strs []string) bool {
	for _, s := range strs {
		if strings.HasPrefix(str, s) {
			return true
		}
	}
	return false
}

// convert a role to a string
func role2String(role int) string {
	if role == int(pb.Role_CLIENT) {
		return "client"
	} else if role == int(pb.Role_PEER) {
		return "peer"
	} else if role == int(pb.Role_VALIDATOR) {
		return "validator"
	} else if role == int(pb.Role_AUDITOR) {
		return "auditor"
	}
	return ""
}

// Remove outer quotes from a string if necessary
func removeQuotes(str string) string {
	if str == "" {
		return str
	}
	if (strings.HasPrefix(str, "'") && strings.HasSuffix(str, "'")) ||
		(strings.HasPrefix(str, "\"") && strings.HasSuffix(str, "\"")) {
		str = str[1 : len(str)-1]
	}
	caLogger.Debugf("removeQuotes: %s\n", str)
	return str
}

// Convert the protobuf array of attributes to the AttributePair array format
// as required by the ACA code to populate the table
func toAttributePairs(id, affiliation string, attrs []*pb.Attribute) ([]*AttributePair, error) {
	var pairs = make([]*AttributePair, 0)
	for _, attr := range attrs {
		vals := []string{id, affiliation, attr.Name, attr.Value, attr.NotBefore, attr.NotAfter}
		pair, err := NewAttributePair(vals, nil)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, pair)
	}
	caLogger.Debugf("toAttributePairs: id=%s, affiliation=%s, attrs=%v, pairs=%v\n",
		id, affiliation, attrs, pairs)
	return pairs, nil
}

func convertTime(ts *gp.Timestamp) time.Time {
	var t time.Time
	if ts == nil {
		t = time.Unix(0, 0).UTC()
	} else {
		t = time.Unix(ts.Seconds, int64(ts.Nanos)).UTC()
	}
	return t
}
