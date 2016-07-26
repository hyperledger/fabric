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

package buckettree

type byBucketNumber map[int]*bucketNode

type bucketTreeDelta struct {
	byLevel map[int]byBucketNumber
}

func newBucketTreeDelta() *bucketTreeDelta {
	return &bucketTreeDelta{make(map[int]byBucketNumber)}
}

func (bucketTreeDelta *bucketTreeDelta) getOrCreateBucketNode(bucketKey *bucketKey) *bucketNode {
	byBucketNumber := bucketTreeDelta.byLevel[bucketKey.level]
	if byBucketNumber == nil {
		byBucketNumber = make(map[int]*bucketNode)
		bucketTreeDelta.byLevel[bucketKey.level] = byBucketNumber
	}
	bucketNode := byBucketNumber[bucketKey.bucketNumber]
	if bucketNode == nil {
		bucketNode = newBucketNode(bucketKey)
		byBucketNumber[bucketKey.bucketNumber] = bucketNode
	}
	return bucketNode
}

func (bucketTreeDelta *bucketTreeDelta) isEmpty() bool {
	return bucketTreeDelta.byLevel == nil || len(bucketTreeDelta.byLevel) == 0
}

func (bucketTreeDelta *bucketTreeDelta) getBucketNodesAt(level int) []*bucketNode {
	bucketNodes := []*bucketNode{}
	byBucketNumber := bucketTreeDelta.byLevel[level]
	if byBucketNumber == nil {
		return nil
	}
	for _, bucketNode := range byBucketNumber {
		bucketNodes = append(bucketNodes, bucketNode)
	}
	return bucketNodes
}

func (bucketTreeDelta *bucketTreeDelta) getRootNode() *bucketNode {
	bucketNodes := bucketTreeDelta.getBucketNodesAt(0)
	if bucketNodes == nil || len(bucketNodes) == 0 {
		panic("This method should be called after processing is completed (i.e., the root node has been created)")
	}
	return bucketNodes[0]
}
