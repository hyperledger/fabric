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

package bddtests

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DATA-DOG/godog"
	"github.com/DATA-DOG/godog/gherkin"
	"github.com/hyperledger/fabric/common/util"
)

// BDDContext represents the current context for the executing scenario.  Commensurate concept of 'context' from behave testing.
type BDDContext struct {
	grpcClientPort            int
	composition               *Composition
	godogSuite                *godog.Suite
	scenarioOrScenarioOutline interface{}
	users                     map[string]*UserRegistration
}

func (b *BDDContext) getScenarioDefinition() *gherkin.ScenarioDefinition {
	if b.scenarioOrScenarioOutline == nil {
		return nil
	}
	switch t := b.scenarioOrScenarioOutline.(type) {
	case *gherkin.Scenario:
		return &(t.ScenarioDefinition)
	case *gherkin.ScenarioOutline:
		return &(t.ScenarioDefinition)
	}
	return nil
}

func (b *BDDContext) hasTag(tagName string) bool {
	if b.scenarioOrScenarioOutline == nil {
		return false
	}
	hasTagInner := func(tags []*gherkin.Tag) bool {
		for _, t := range tags {
			if t.Name == tagName {
				return true
			}
		}
		return false
	}

	switch t := b.scenarioOrScenarioOutline.(type) {
	case *gherkin.Scenario:
		return hasTagInner(t.Tags)
	case *gherkin.ScenarioOutline:
		return hasTagInner(t.Tags)
	}
	return false
}

// GetArgsForUser will return an arg slice of string allowing for replacement of parameterized values based upon tags for userRegistration
func (b *BDDContext) GetArgsForUser(cells []*gherkin.TableCell, userRegistration *UserRegistration) (args []string, err error) {
	regExp := regexp.MustCompile("\\{(.*?)\\}+")
	// Loop through cells and replace with user tag values if found
	for _, cell := range cells {
		var arg = cell.Value
		for _, tagNameToFind := range regExp.FindAllStringSubmatch(cell.Value, -1) {
			println("toFind = ", tagNameToFind[0], " to replace = ", tagNameToFind[1])
			var tagValue interface{}
			tagValue, err = userRegistration.GetTagValue(tagNameToFind[1])
			if err != nil {
				return nil, fmt.Errorf("Error getting args for user '%s': %s", userRegistration.enrollID, err)
			}
			arg = strings.Replace(arg, tagNameToFind[0], fmt.Sprintf("%v", tagValue), 1)
		}
		args = append(args, arg)
	}
	return args, nil
}

func (b *BDDContext) weCompose(composeFiles string) error {
	if b.composition != nil {
		return fmt.Errorf("Already have composition in BDD context (%s)", b.composition.projectName)
	}
	// Need a unique name, but docker does not allow '-' in names
	composeProjectName := strings.Replace(util.GenerateUUID(), "-", "", -1)
	newComposition, err := NewComposition(composeProjectName, composeFiles)
	if err != nil {
		return fmt.Errorf("Error composing system in BDD context:  %s", err)
	}
	b.composition = newComposition
	return nil
}

func (b *BDDContext) beforeScenario(scenarioOrScenarioOutline interface{}) {
	b.scenarioOrScenarioOutline = scenarioOrScenarioOutline
	//switch t := scenarioOrScenarioOutline.(type) {
	//case *gherkin.Scenario:
	//	fmt.Printf("Scenario received %v", t)
	//case *gherkin.ScenarioOutline:
	//	fmt.Printf("ScenarioOutline received %v", t)
	//}
}

func (b *BDDContext) afterScenarioDecompose(interface{}, error) {
	if b.hasTag("@doNotDecompose") == true {
		fmt.Printf("Not decomposing:  %s", b.getScenarioDefinition().Name)
	} else {
		if b.composition != nil {
			b.composition.Decompose()
		}
	}
	// Now clear the users
	b.composition = nil
	b.users = make(map[string]*UserRegistration)
}
