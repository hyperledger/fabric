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

package discovery

import "testing"

func TestZeroNodes(t *testing.T) {
	disc := NewDiscoveryImpl()
	nodes := disc.GetAllNodes()
	if len(nodes) != 0 {
		t.Fatalf("Expected empty list, got a list of size %d instead", len(nodes))
	}
}

func TestAddFindNode(t *testing.T) {
	disc := NewDiscoveryImpl()
	res := disc.AddNode("foo")
	if !res || !disc.FindNode("foo") {
		t.Fatal("Unable to add a node to the discovery list")
	}
}

func TestRemoveNode(t *testing.T) {
	disc := NewDiscoveryImpl()
	_ = disc.AddNode("foo")
	if !disc.RemoveNode("foo") || len(disc.GetAllNodes()) != 0 {
		t.Fatalf("Unable to remove a node from the discovery list")
	}
	if disc.RemoveNode("bar") {
		t.Fatalf("Remove operation should have failed, element is not present in the list")
	}
}

func TestGetAllNodes(t *testing.T) {
	initList := []string{"a", "b", "c", "d"}
	disc := NewDiscoveryImpl()
	for i := range initList {
		_ = disc.AddNode(initList[i])
	}
	nodes := disc.GetAllNodes()

	expected := len(initList)
	actual := len(nodes)
	if actual != expected {
		t.Fatalf("Nodes list length should have been %d but is %d", expected, actual)
		return
	}

	for _, node := range nodes {
		if !inArray(node, initList) {
			t.Fatalf("%s is found in the discovery list but not in the initial list %v", node, initList)
		}
	}
}

func TestRandomNodes(t *testing.T) {
	initList := []string{"a", "b", "c", "d", "e"}
	disc := NewDiscoveryImpl()
	for i := range initList {
		_ = disc.AddNode(initList[i])
	}
	expectedCount := 2
	randomSet := disc.GetRandomNodes(expectedCount)
	actualCount := len(randomSet)
	if actualCount != expectedCount {
		t.Fatalf("Expected %d random nodes, got %d instead", expectedCount, actualCount)
	}
	for _, node := range randomSet {
		if !inArray(node, initList) {
			t.Fatalf("%s was randomly picked from the discovery list but is not in the initial list %v", node, initList)
		}
	}

	// Does the random array contain duplicate values? And does it pick nodes that were previously removed?
	removedElement := "d"
	_ = disc.RemoveNode(removedElement)
	for i := 0; i < 5; i++ {
		trackDuplicates := make(map[string]bool)
		anotherRandomSet := disc.GetRandomNodes(expectedCount)
		if inArray(removedElement, anotherRandomSet) {
			t.Fatalf("Random array %v contains element %v that was removed", anotherRandomSet, removedElement)
		}
		for _, v := range anotherRandomSet {
			if _, ok := trackDuplicates[v]; ok {
				t.Fatalf("Random array contains duplicate values: %v", anotherRandomSet)
			}
			trackDuplicates[v] = true
		}
	}

	// Do we get a random element?
	for i := 0; i < 100; i++ {
		if anotherSet := disc.GetRandomNodes(1); anotherSet[0] != randomSet[0] {
			return
		}
	}
	t.Fatalf("Random returned value is always %s", randomSet[0])
}
