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

package crypto

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger/fabric/core/crypto/utils"
)

func (peer *peerImpl) initKeyStore() error {
	// create tables
	peer.Debugf("Create Table [%s] if not exists", "Certificates")
	if _, err := peer.ks.sqlDB.Exec("CREATE TABLE IF NOT EXISTS Certificates (id VARCHAR, certsign BLOB, certenc BLOB, PRIMARY KEY (id))"); err != nil {
		peer.Errorf("Failed creating table [%s].", err.Error())
		return err
	}

	return nil
}

func (ks *keyStore) GetSignEnrollmentCert(id []byte, certFetcher func(id []byte) ([]byte, []byte, error)) ([]byte, error) {
	if len(id) == 0 {
		return nil, fmt.Errorf("Invalid peer id. It is empty.")
	}

	ks.m.Lock()
	defer ks.m.Unlock()

	sid := utils.EncodeBase64(id)

	certSign, certEnc, err := ks.selectSignEnrollmentCert(sid)
	if err != nil {
		ks.node.Errorf("Failed selecting enrollment cert [%s].", err.Error())

		return nil, err
	}

	if certSign == nil {
		ks.node.Debugf("Cert for [%s] not available. Fetching from ECA....", sid)

		// If No cert is available, fetch from ECA

		// 1. Fetch
		ks.node.Debug("Fectch Enrollment Certificate from ECA...")
		certSign, certEnc, err = certFetcher(id)
		if err != nil {
			return nil, err
		}

		// 2. Store
		ks.node.Debug("Store certificate...")
		tx, err := ks.sqlDB.Begin()
		if err != nil {
			ks.node.Errorf("Failed beginning transaction [%s].", err.Error())

			return nil, err
		}

		ks.node.Debugf("Insert id [%s].", sid)
		ks.node.Debugf("Insert cert [% x].", certSign)

		_, err = tx.Exec("INSERT INTO Certificates (id, certsign, certenc) VALUES (?, ?, ?)", sid, certSign, certEnc)

		if err != nil {
			ks.node.Errorf("Failed inserting cert [%s].", err.Error())

			tx.Rollback()

			return nil, err
		}

		err = tx.Commit()
		if err != nil {
			ks.node.Errorf("Failed committing transaction [%s].", err.Error())

			tx.Rollback()

			return nil, err
		}

		ks.node.Debug("Fectch Enrollment Certificate from ECA...done!")

		certSign, certEnc, err = ks.selectSignEnrollmentCert(sid)
		if err != nil {
			ks.node.Errorf("Failed selecting next TCert after fetching [%s].", err.Error())

			return nil, err
		}
	}

	ks.node.Debugf("Cert for [%s] = [% x]", sid, certSign)

	return certSign, nil
}

func (ks *keyStore) selectSignEnrollmentCert(id string) ([]byte, []byte, error) {
	ks.node.Debugf("Select Sign Enrollment Cert for id [%s]", id)

	// Get the first row available
	var cert []byte
	row := ks.sqlDB.QueryRow("SELECT certsign FROM Certificates where id = ?", id)
	err := row.Scan(&cert)

	if err == sql.ErrNoRows {
		return nil, nil, nil
	} else if err != nil {
		ks.node.Errorf("Error during select [%s].", err.Error())

		return nil, nil, err
	}

	ks.node.Debugf("Cert [% x].", cert)

	ks.node.Debug("Select Enrollment Cert...done!")

	return cert, nil, nil
}
