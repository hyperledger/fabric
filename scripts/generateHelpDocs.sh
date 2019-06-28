#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
export FABRIC_CFG_PATH=./sampleconfig

DOC=docs/source/commands/peerversion.md
cat docs/wrappers/peer_version_preamble.md > $DOC

for x in "peer version"; do
  echo "" >> $DOC
  echo "\`\`\`" >> $DOC
  .build/bin/${x} --help 1>> $DOC 2>/dev/null
  echo "\`\`\`" >> $DOC
  echo "" >> $DOC
done
cat docs/wrappers/license_postscript.md >> $DOC

DOC=docs/source/commands/peerchaincode.md
cat docs/wrappers/peer_chaincode_preamble.md > $DOC

for x in "peer chaincode install" "peer chaincode instantiate" "peer chaincode invoke" "peer chaincode list" "peer chaincode package" "peer chaincode query" "peer chaincode signpackage" "peer chaincode upgrade"; do
  echo "" >> $DOC
  echo "##" $x >> $DOC
  echo "\`\`\`" >> $DOC
  .build/bin/${x} --help 1>> $DOC 2>/dev/null
  echo "\`\`\`" >> $DOC
  echo "" >> $DOC
done
cat docs/wrappers/peer_chaincode_postscript.md >> $DOC

DOC=docs/source/commands/peerchannel.md
cat docs/wrappers/peer_channel_preamble.md > $DOC

for x in "peer channel" "peer channel create" "peer channel fetch" "peer channel getinfo" "peer channel join" "peer channel list" "peer channel signconfigtx" "peer channel update"; do
  echo "" >> $DOC
  echo "##" $x >> $DOC
  echo "\`\`\`" >> $DOC
  .build/bin/${x} --help 1>> $DOC 2>/dev/null
  echo "\`\`\`" >> $DOC
  echo "" >> $DOC
done
cat docs/wrappers/peer_channel_postscript.md >> $DOC

DOC=docs/source/commands/peerlogging.md
cat docs/wrappers/peer_logging_preamble.md > $DOC

for x in "peer logging" "peer logging getlevel" "peer logging revertlevels" "peer logging setlevel"; do
  echo "" >> $DOC
  echo "##" $x >> $DOC
  echo "\`\`\`" >> $DOC
  .build/bin/${x} --help 1>> $DOC 2>/dev/null
  echo "\`\`\`" >> $DOC
  echo "" >> $DOC
done
cat docs/wrappers/peer_logging_postscript.md >> $DOC

DOC=docs/source/commands/peernode.md
cat docs/wrappers/peer_node_preamble.md > $DOC

for x in "peer node start" "peer node status" "peer node reset" "peer node rollback"; do
  echo "" >> $DOC
  echo "##" $x >> $DOC
  echo "\`\`\`" >> $DOC
  .build/bin/${x} --help 1>> $DOC 2>/dev/null
  echo "\`\`\`" >> $DOC
  echo "" >> $DOC
done
cat docs/wrappers/peer_node_postscript.md >> $DOC

DOC=${PWD}/docs/source/commands/configtxgen.md
cat docs/wrappers/configtxgen_preamble.md > $DOC

for x in "configtxgen"; do
  echo "" >> $DOC
  echo "##" $x >> $DOC
  echo "\`\`\`" >> $DOC
  (cd .build/bin && PATH=./:${PATH} ${x} --help 2>> $DOC)
  echo "\`\`\`" >> $DOC
  echo "" >> $DOC
done

cat docs/wrappers/configtxgen_postscript.md >> $DOC
grep -v 'WARN 001' $DOC > tmp && mv tmp $DOC

DOC=docs/source/commands/cryptogen.md
cat docs/wrappers/cryptogen_preamble.md > $DOC

echo "" >> $DOC

for x in "cryptogen help" "cryptogen generate" "cryptogen showtemplate" "cryptogen extend" "cryptogen version"; do
  echo "" >> $DOC
  echo "##" $x >> $DOC
  echo "\`\`\`" >> $DOC
  .build/bin/${x} --help 2>> $DOC
  echo "\`\`\`" >> $DOC
  echo "" >> $DOC
done
cat docs/wrappers/cryptogen_postscript.md >> $DOC

DOC=docs/source/commands/configtxlator.md

cat docs/wrappers/configtxlator_preamble.md > $DOC

for x in "configtxlator start" "configtxlator proto_encode" "configtxlator proto_decode" "configtxlator compute_update" "configtxlator version"; do
  echo "" >> $DOC
  echo "##" $x >> $DOC
  echo "\`\`\`" >> $DOC
  .build/bin/${x} --help 2>> $DOC
  echo "\`\`\`" >> $DOC
  echo "" >> $DOC
done
cat docs/wrappers/configtxlator_postscript.md >> $DOC

exit
