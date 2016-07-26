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

package pbft

import pb "github.com/golang/protobuf/proto"

type signable interface {
	getSignature() []byte
	setSignature(s []byte)
	getID() uint64
	setID(id uint64)
	serialize() ([]byte, error)
}

func (instance *pbftCore) sign(s signable) error {
	s.setSignature(nil)
	raw, err := s.serialize()
	if err != nil {
		return err
	}
	signedRaw, err := instance.consumer.sign(raw)
	if err != nil {
		return err
	}
	s.setSignature(signedRaw)

	return nil
}

func (instance *pbftCore) verify(s signable) error {
	origSig := s.getSignature()
	s.setSignature(nil)
	raw, err := s.serialize()
	s.setSignature(origSig)
	if err != nil {
		return err
	}
	return instance.consumer.verify(s.getID(), origSig, raw)
}

func (vc *ViewChange) getSignature() []byte {
	return vc.Signature
}

func (vc *ViewChange) setSignature(sig []byte) {
	vc.Signature = sig
}

func (vc *ViewChange) getID() uint64 {
	return vc.ReplicaId
}

func (vc *ViewChange) setID(id uint64) {
	vc.ReplicaId = id
}

func (vc *ViewChange) serialize() ([]byte, error) {
	return pb.Marshal(vc)
}
