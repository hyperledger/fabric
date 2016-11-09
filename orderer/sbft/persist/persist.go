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

package persist

import (
	"io/ioutil"
	"os"
	"strings"
)

type Persist struct {
	dir string
}

func New(dir string) *Persist {
	p := &Persist{
		dir: dir,
	}
	os.MkdirAll(dir, 0755)
	return p
}

func (p *Persist) path(key string) string {
	return p.dir + "/" + key
}

//
func (p *Persist) StoreState(key string, value []byte) error {
	return ioutil.WriteFile(p.path(key), value, 0640)
}

func (p *Persist) ReadState(key string) ([]byte, error) {
	return ioutil.ReadFile(p.path(key))
}

func (p *Persist) ReadStateSet(prefix string) (map[string][]byte, error) {
	files, err := ioutil.ReadDir(p.dir)
	if err != nil {
		return nil, err
	}
	r := make(map[string][]byte)
	for _, fi := range files {
		if fi.Mode()&os.ModeType != 0 {
			continue
		}
		if strings.Index(fi.Name(), prefix) != 0 {
			continue
		}
		data, err := ioutil.ReadFile(p.path(fi.Name()))
		if err != nil {
			return nil, err
		}
		r[fi.Name()] = data
	}
	return r, nil
}

func (p *Persist) DelState(key string) {
	os.Remove(p.path(key))
}
