/*
Copyright IBM Corp. 2017 All Rights Reserved.

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
package pkcs11

import (
	"fmt"
	"github.com/miekg/pkcs11"
)

var (
	ctx      *pkcs11.Ctx
	sessions = make(chan pkcs11.SessionHandle, 2000)
	slot     uint
)

func initPKCS11(lib, pin, label string) error {
	return loadLib(lib, pin, label)
}

func loadLib(lib, pin, label string) error {
	logger.Debugf("Loading pkcs11 library [%s]\n", lib)
	if lib == "" {
		return fmt.Errorf("No PKCS11 library default")
	}

	ctx = pkcs11.New(lib)
	if ctx == nil {
		return fmt.Errorf("Instantiate failed [%s]", lib)
	}

	ctx.Initialize()
	slots, err := ctx.GetSlotList(true)
	if err != nil {
		return err
	}
	found := false
	for _, s := range slots {
		info, err := ctx.GetTokenInfo(s)
		if err != nil {
			continue
		}
		if label == info.Label {
			found = true
			slot = s
			break
		}
	}
	if !found {
		return fmt.Errorf("Could not find token with label %s", label)
	}
	session := getSession()
	defer returnSession(session)

	if pin == "" {
		return fmt.Errorf("No PIN set\n")
	}
	err = ctx.Login(session, pkcs11.CKU_USER, pin)
	if err != nil {
		return fmt.Errorf("Login failed [%s]\n", err)
	}

	return nil
}

func getSession() (session pkcs11.SessionHandle) {
	select {
	case session = <-sessions:
		logger.Debugf("Reusing existing pkcs11 session %x on slot %d\n", session, slot)

	default:
		// create one
		var s pkcs11.SessionHandle
		var err error = nil
		for i := 0; i < 10; i++ {
			s, err = ctx.OpenSession(slot, pkcs11.CKF_SERIAL_SESSION|pkcs11.CKF_RW_SESSION)
			if err != nil {
				logger.Warningf("OpenSession failed, retrying [%s]\n", err)
			} else {
				break
			}
		}
		if err != nil {
			logger.Fatalf("OpenSession [%s]\n", err)
		}
		logger.Debugf("Created new pkcs11 session %x on slot %d\n", session, slot)
		session = s
	}
	return session
}

func returnSession(session pkcs11.SessionHandle) {
	sessions <- session
}
