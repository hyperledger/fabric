/*
# Copyright State Street Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
*/

package ccmetadata

import (
	"github.com/hyperledger/fabric/common/flogging"
)

//logger used by this package
var logger = flogging.MustGetLogger("chaincode-metadata")

//MetadataProvider is implemented by each platform in a platform specific manner.
//It can process metadata stored in ChaincodeDeploymentSpec in different formats.
//The common format is targz. Currently users expect the metadata to be presented
//as tar file entries (directly extracted from chaincode stored in targz format).
//In future, we would like provide better abstraction by extending the interface
type MetadataProvider interface {
	GetMetadataAsTarEntries() ([]byte, error)
}
