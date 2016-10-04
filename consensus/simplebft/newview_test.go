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

package simplebft

import (
	"reflect"
	"testing"
)

func TestXsetNoByz(t *testing.T) {
	s := &SBFT{config: Config{N: 4, F: 1}, seq: SeqView{3, 1}}
	vcs := []*ViewChange{
		&ViewChange{
			View: 3,
			Pset: nil,
			Qset: []*Subject{&Subject{&SeqView{1, 2}, []byte("val1")},
				&Subject{&SeqView{2, 2}, []byte("val2")}},
			Executed: 1,
		},
		&ViewChange{
			View:     3,
			Pset:     []*Subject{&Subject{&SeqView{1, 2}, []byte("val1")}},
			Qset:     []*Subject{&Subject{&SeqView{1, 2}, []byte("val1")}},
			Executed: 1,
		},
		&ViewChange{
			View: 3,
			Pset: []*Subject{&Subject{&SeqView{2, 2}, []byte("val2")}},
			Qset: []*Subject{&Subject{&SeqView{1, 2}, []byte("val1")},
				&Subject{&SeqView{2, 2}, []byte("val2")}},
			Executed: 1,
		},
	}

	xset, ok := s.makeXset(vcs)
	if !ok {
		t.Fatal("no xset")
	}

	if !reflect.DeepEqual(xset, &Subject{&SeqView{3, 2}, []byte("val2")}) {
		t.Error(xset)
	}
}

func TestXsetByz0(t *testing.T) {
	s := &SBFT{config: Config{N: 4, F: 1}, seq: SeqView{3, 1}}
	vcs := []*ViewChange{
		&ViewChange{
			View:     3,
			Pset:     nil,
			Qset:     nil,
			Executed: 1,
		},
		&ViewChange{
			View:     3,
			Pset:     []*Subject{&Subject{&SeqView{1, 2}, []byte("val1")}},
			Qset:     []*Subject{&Subject{&SeqView{1, 2}, []byte("val1")}},
			Executed: 1,
		},
		&ViewChange{
			View: 3,
			Pset: []*Subject{&Subject{&SeqView{2, 2}, []byte("val2")}},
			Qset: []*Subject{&Subject{&SeqView{1, 2}, []byte("val1")},
				&Subject{&SeqView{2, 2}, []byte("val2")}},
			Executed: 1,
		},
	}

	xset, ok := s.makeXset(vcs)
	if ok {
		t.Error("should not have received an xset")
	}

	vcs = append(vcs, &ViewChange{
		View: 3,
		Pset: []*Subject{&Subject{&SeqView{2, 2}, []byte("val2")}},
		Qset: []*Subject{&Subject{&SeqView{1, 2}, []byte("val1")},
			&Subject{&SeqView{2, 2}, []byte("val2")}},
		Executed: 2,
	})

	xset, ok = s.makeXset(vcs)
	if !ok {
		t.Error("no xset")
	}
	if !reflect.DeepEqual(xset, &Subject{&SeqView{3, 2}, []byte("val2")}) {
		t.Error(xset)
	}
}

func TestXsetByz2(t *testing.T) {
	s := &SBFT{config: Config{N: 4, F: 1}, seq: SeqView{3, 1}}
	vcs := []*ViewChange{
		&ViewChange{
			View:     3,
			Pset:     nil,
			Qset:     []*Subject{&Subject{&SeqView{1, 2}, []byte("val1")}},
			Executed: 1,
		},
		&ViewChange{
			View:     3,
			Pset:     []*Subject{&Subject{&SeqView{1, 2}, []byte("val1")}},
			Qset:     []*Subject{&Subject{&SeqView{1, 2}, []byte("val1")}},
			Executed: 1,
		},
		&ViewChange{
			View: 3,
			Pset: []*Subject{&Subject{&SeqView{2, 2}, []byte("val2")}},
			Qset: []*Subject{&Subject{&SeqView{1, 2}, []byte("val1")},
				&Subject{&SeqView{2, 2}, []byte("val2")}},
			Executed: 1,
		},
	}

	xset, ok := s.makeXset(vcs)
	if ok {
		t.Error("should not have received an xset")
	}

	vcs = append(vcs, &ViewChange{
		View:     3,
		Pset:     []*Subject{&Subject{&SeqView{1, 2}, []byte("val1")}},
		Qset:     []*Subject{&Subject{&SeqView{1, 2}, []byte("val1")}},
		Executed: 2,
	})

	xset, ok = s.makeXset(vcs)
	if !ok {
		t.Error("no xset")
	}
	if !reflect.DeepEqual(xset, &Subject{&SeqView{3, 2}, []byte("val1")}) {
		t.Error(xset)
	}
}
