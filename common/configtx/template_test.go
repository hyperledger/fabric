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

package configtx

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/util"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func verifyItemsResult(t *testing.T, template Template, count int) {
	newChainID := "foo"

	result, err := template.Items(newChainID)
	if err != nil {
		t.Fatalf("Should not have errored")
	}

	if len(result) != count {
		t.Errorf("Expected %d items, but got %d", count, len(result))
	}

	for i, signedItem := range result {
		item := utils.UnmarshalConfigurationItemOrPanic(signedItem.ConfigurationItem)
		expected := fmt.Sprintf("%d", i)
		assert.Equal(t, expected, string(item.Value), "Expected %s but got %s", expected, item.Value)
	}
}

func TestSimpleTemplate(t *testing.T) {
	simple := NewSimpleTemplate(
		&cb.ConfigurationItem{Value: []byte("0")},
		&cb.ConfigurationItem{Value: []byte("1")},
	)
	verifyItemsResult(t, simple, 2)
}

func TestCompositeTemplate(t *testing.T) {
	composite := NewCompositeTemplate(
		NewSimpleTemplate(
			&cb.ConfigurationItem{Value: []byte("0")},
			&cb.ConfigurationItem{Value: []byte("1")},
		),
		NewSimpleTemplate(
			&cb.ConfigurationItem{Value: []byte("2")},
		),
	)

	verifyItemsResult(t, composite, 3)
}

func TestNewChainTemplate(t *testing.T) {
	simple := NewSimpleTemplate(
		&cb.ConfigurationItem{Value: []byte("1")},
		&cb.ConfigurationItem{Value: []byte("2")},
	)

	creationPolicy := "Test"
	nct := NewChainCreationTemplate(creationPolicy, util.ComputeCryptoHash, simple)

	newChainID := "foo"
	items, err := nct.Items(newChainID)
	if err != nil {
		t.Fatalf("Error creation a chain creation configuration")
	}

	if expected := 3; len(items) != expected {
		t.Fatalf("Expected %d items, but got %d", expected, len(items))
	}

	for i, signedItem := range items {
		item := utils.UnmarshalConfigurationItemOrPanic(signedItem.ConfigurationItem)
		if i == 0 {
			if item.Key != CreationPolicyKey {
				t.Errorf("First item should have been the creation policy")
			}
		} else {
			if expected := fmt.Sprintf("%d", i); string(item.Value) != expected {
				t.Errorf("Expected %s but got %s", expected, item.Value)
			}
		}
	}
}
