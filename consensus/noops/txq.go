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

package noops

import (
	pb "github.com/hyperledger/fabric/protos"
)

type txq struct {
	i int
	q []*pb.Transaction
}

func newTXQ(size int) *txq {
	o := &txq{}
	o.i = 0
	if size < 1 {
		size = 1
	}
	o.q = make([]*pb.Transaction, size)
	return o
}

func (o *txq) append(tx *pb.Transaction) {
	if cap(o.q) > o.i {
		o.q[o.i] = tx
		o.i++
	}
}

func (o *txq) getTXs() []*pb.Transaction {
	length := o.i
	o.i = 0
	return o.q[:length]
}

func (o *txq) isFull() bool {
	if cap(o.q) == o.i {
		return true
	}
	return false
}

func (o *txq) size() int {
	return o.i
}

func (o *txq) reset() {
	o.i = 0
}
