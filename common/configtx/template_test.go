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

	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

func verifyItemsResult(t *testing.T, template Template, count int) {
	newChainID := "foo"

	configEnv, err := template.Envelope(newChainID)
	if err != nil {
		t.Fatalf("Should not have errored: %s", err)
	}

	configNext, err := UnmarshalConfigNext(configEnv.Config)
	if err != nil {
		t.Fatalf("Should not have errored: %s", err)
	}
	config := ConfigNextToConfig(configNext)

	if len(config.Items) != count {
		t.Errorf("Expected %d items, but got %d", count, len(config.Items))
	}

	for i, _ := range config.Items {
		count := 0
		for _, item := range config.Items {
			key := fmt.Sprintf("%d", i)
			if key == item.Key {
				count++
			}
		}
		expected := 1
		assert.Equal(t, expected, count, "Expected %d but got %d for %d", expected, count, i)
	}
}

func TestSimpleTemplate(t *testing.T) {
	simple := NewSimpleTemplate(
		&cb.ConfigItem{Type: cb.ConfigItem_Orderer, Key: "0"},
		&cb.ConfigItem{Type: cb.ConfigItem_Orderer, Key: "1"},
	)
	verifyItemsResult(t, simple, 2)
}

func TestCompositeTemplate(t *testing.T) {
	composite := NewCompositeTemplate(
		NewSimpleTemplate(
			&cb.ConfigItem{Type: cb.ConfigItem_Orderer, Key: "0"},
			&cb.ConfigItem{Type: cb.ConfigItem_Orderer, Key: "1"},
		),
		NewSimpleTemplate(
			&cb.ConfigItem{Type: cb.ConfigItem_Orderer, Key: "2"},
		),
	)

	verifyItemsResult(t, composite, 3)
}

func TestNewChainTemplate(t *testing.T) {
	simple := NewSimpleTemplate(
		&cb.ConfigItem{Type: cb.ConfigItem_Orderer, Key: "0"},
		&cb.ConfigItem{Type: cb.ConfigItem_Orderer, Key: "1"},
	)

	creationPolicy := "Test"
	nct := NewChainCreationTemplate(creationPolicy, simple)

	newChainID := "foo"
	configEnv, err := nct.Envelope(newChainID)
	if err != nil {
		t.Fatalf("Error creation a chain creation config")
	}

	configNext, err := UnmarshalConfigNext(configEnv.Config)
	if err != nil {
		t.Fatalf("Should not have errored: %s", err)
	}
	config := ConfigNextToConfig(configNext)

	if expected := 3; len(config.Items) != expected {
		t.Fatalf("Expected %d items, but got %d", expected, len(config.Items))
	}

	for i, _ := range config.Items {
		if i == len(config.Items)-1 {
			break
		}
		count := 0
		for _, item := range config.Items {
			key := fmt.Sprintf("%d", i)
			if key == item.Key {
				count++
			}
		}
		expected := 1
		assert.Equal(t, expected, count, "Expected %d but got %d for %d", expected, count, i)
	}

	foundCreationPolicy := false
	for _, item := range config.Items {
		if item.Key == CreationPolicyKey {
			foundCreationPolicy = true
			continue
		}
	}

	if !foundCreationPolicy {
		t.Errorf("Should have found the creation policy")
	}
}
