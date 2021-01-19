/*
Copyright Suzhou Tongji Fintech Research Institute 2017 All Rights Reserved.
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
package gm

import (
	"errors"

	"github.com/tjfoc/hyperledger-fabric-gm/bccsp"
)

//定义国密 Key的驱动 ，实现 KeyDeriver 接口
type smPublicKeyKeyDeriver struct{}

func (kd *smPublicKeyKeyDeriver) KeyDeriv(k bccsp.Key, opts bccsp.KeyDerivOpts) (dk bccsp.Key, err error) {

	return nil, errors.New("Not implemented")

}
