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

func (node *nodeImpl) prependPrefix(args []interface{}) []interface{} {
	return append([]interface{}{node.conf.logPrefix}, args...)
}

func (node *nodeImpl) Infof(format string, args ...interface{}) {
	log.Infof(node.conf.logPrefix+format, args...)
}

func (node *nodeImpl) Info(args ...interface{}) {
	log.Info(node.prependPrefix(args)...)
}

func (node *nodeImpl) Debugf(format string, args ...interface{}) {
	log.Debugf(node.conf.logPrefix+format, args...)
}

func (node *nodeImpl) Debug(args ...interface{}) {
	log.Debug(node.prependPrefix(args)...)
}

func (node *nodeImpl) Errorf(format string, args ...interface{}) {
	log.Errorf(node.conf.logPrefix+format, args...)
}

func (node *nodeImpl) Error(args ...interface{}) {
	log.Error(node.prependPrefix(args)...)
}

func (node *nodeImpl) Warningf(format string, args ...interface{}) {
	log.Warningf(node.conf.logPrefix+format, args...)
}

func (node *nodeImpl) Warning(args ...interface{}) {
	log.Warning(node.prependPrefix(args)...)
}
