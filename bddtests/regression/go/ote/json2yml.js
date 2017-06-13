//
// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//
var fs = require('fs');

var cfgFile = process.argv[2];
//var cfgFile = __dirname + "/" + "network-cfg.json";
var dFile = __dirname + "/" + "docker-compose.yml";
fs.createWriteStream(dFile);

var CA=0;
var CDB=0;
var KAFKA=0;

// Orderer environment var
var ord_env_name=[];
var ord_env_val=[];
if ( process.env.CONFIGTX_ORDERER_BATCHSIZE_MAXMESSAGECOUNT != null ) {
    console.log(' CONFIGTX_ORDERER_BATCHSIZE_MAXMESSAGECOUNT= ', process.env.CONFIGTX_ORDERER_BATCHSIZE_MAXMESSAGECOUNT);
    ord_env_name.push('CONFIGTX_ORDERER_BATCHSIZE_MAXMESSAGECOUNT');
    ord_env_val.push(process.env.CONFIGTX_ORDERER_BATCHSIZE_MAXMESSAGECOUNT);
}
if ( process.env.CONFIGTX_ORDERER_ORDERERTYPE != null ) {
    console.log(' CONFIGTX_ORDERER_ORDERERTYPE= ', process.env.CONFIGTX_ORDERER_ORDERERTYPE);
    ord_env_name.push('CONFIGTX_ORDERER_ORDERERTYPE');
    ord_env_val.push(process.env.CONFIGTX_ORDERER_ORDERERTYPE);
    if ( process.env.CONFIGTX_ORDERER_ORDERERTYPE == 'kafka' ) {
       KAFKA=1;
    }
}
if ( process.env.CONFIGTX_ORDERER_BATCHTIMEOUT != null ) {
    console.log(' CONFIGTX_ORDERER_BATCHTIMEOUT= ', process.env.CONFIGTX_ORDERER_BATCHTIMEOUT);
    ord_env_name.push('CONFIGTX_ORDERER_BATCHTIMEOUT');
    ord_env_val.push(process.env.CONFIGTX_ORDERER_BATCHTIMEOUT);
}

console.log('ord_env_name: ', ord_env_name.length, ord_env_name);
console.log('ord_env_val: ', ord_env_val.length, ord_env_val);

// Peer environment var
var peer_env_name=[];
var peer_env_val=[];
if ( process.env.CORE_LOGGING_LEVEL != null ) {
    console.log(' CORE_LOGGING_LEVEL= ', process.env.CORE_LOGGING_LEVEL);
    peer_env_name.push('CORE_LOGGING_LEVEL');
    peer_env_val.push(process.env.CORE_LOGGING_LEVEL);
}
if ( process.env.CORE_SECURITY_LEVEL != null ) {
    console.log(' CORE_SECURITY_LEVEL= ', process.env.CORE_SECURITY_LEVEL);
    peer_env_name.push('CORE_SECURITY_LEVEL');
    peer_env_val.push(process.env.CORE_SECURITY_LEVEL);
}
if ( process.env.CORE_SECURITY_HASHALGORITHM != null ) {
    console.log(' CORE_SECURITY_HASHALGORITHM= ', process.env.CORE_SECURITY_HASHALGORITHM);
    peer_env_name.push('CORE_SECURITY_HASHALGORITHM');
    peer_env_val.push(process.env.CORE_SECURITY_HASHALGORITHM);
}

console.log('peer_env_name: ', peer_env_name.length, peer_env_name);
console.log('peer_env_val: ', peer_env_val.length, peer_env_val);

//process.exit();
console.log('network cfg: ', cfgFile);
console.log('docker composer: ', dFile);

var addVP = parseInt(process.argv[3]);
console.log('add peer: ', addVP);

var VPName = 'peer' + addVP;

var addOrderer = parseInt(process.argv[4]);;
console.log('add Orderer: ', addOrderer);

var addBroker = parseInt(process.argv[5]);
console.log('add Kafka Broker: ', addBroker);

//console.log(' input argv length', process.argv.length);
var dbType = 'none';
if (process.argv.length == 7) {
   dbType = process.argv[6];
}
console.log('DB type: ', dbType);

var cfgContent = JSON.parse(fs.readFileSync(cfgFile, 'utf8'));

var top_key = Object.keys(cfgContent);

var lvl1_obj;
var lvl2_key;
var lvl2_obj;
var lvl3_key;
var tmp_name;
var tmp_port;
var ordererAddr;
var ordererPort;
var couchdbAddr;
var couchdbPort;
var vp0Addr;
var vp0Port;
var tmp;
var e;

if ( addBroker > 0 ) {
   KAFKA=1;
}

if ( (dbType == 'couchdb') || (dbType == 'goleveldb') ){
   CDB=1;
}

//header 0
for ( i0=0; i0<top_key.length; i0++ ) {
    var lvl0_obj = cfgContent[top_key[i0]];
    var lvl1_key = Object.keys(lvl0_obj);
    if ( top_key[i0] == 'ordererAddress' ) {
         ordererAddr = lvl0_obj;
         console.log('orderer address:', ordererAddr);
    } else if ( top_key[i0] == 'ordererPort' ) {
         ordererPort = parseInt(lvl0_obj);
         console.log('orderer Port:', ordererPort);
    } else if ( top_key[i0] == 'couchdbAddress' ) {
         couchdbAddr = lvl0_obj;
         console.log('couchdb address:', couchdbAddr);
    } else if ( top_key[i0] == 'couchdbPort' ) {
         couchdbPort = parseInt(lvl0_obj);
         console.log('couchdb Port:', couchdbPort);
    } else if ( top_key[i0] == 'vp0Address' ) {
         vp0Addr = lvl0_obj;
         console.log('peer0 address:', vp0Addr);
    } else if ( top_key[i0] == 'vp0Port' ) {
         vp0Port = parseInt(lvl0_obj);
         console.log('peer0 Port:', vp0Port);
    } else if ( top_key[i0] == 'version' ) {
         buff = top_key[i0] + ":" + " '" + lvl0_obj + "'" + "\n";
         fs.appendFileSync(dFile, buff);
    } else if ( top_key[i0] == 'networks' ) {
         buff = top_key[i0] + ":" + "\n";
         fs.appendFileSync(dFile, buff);
         buff = '    bridge:' + '\n';
         fs.appendFileSync(dFile, buff);
    } else if ( top_key[i0] == 'services' ) {
        buff = '\n';
        fs.appendFileSync(dFile, buff);
        buff = top_key[i0] + ':' + '\n';
        fs.appendFileSync(dFile, buff);
        //header 1
        for ( i=0; i<lvl1_key.length; i++ ) {
            lvl1_obj = lvl0_obj[lvl1_key[i]];
            lvl2_key = Object.keys(lvl1_obj);

            // header 2
             if (lvl1_key[i] == 'couchdb' ) {
                if (dbType == 'couchdb') {
                for ( v = 0; v < addVP; v++ ) {
                    tmp_name = lvl1_key[i] + v;
                    tmp_port = couchdbPort + v;
                    buff = '  ' + tmp_name +':' + '\n';
                    fs.appendFileSync(dFile, buff);

                    // header 3
                    for ( k=0; k<lvl2_key.length; k++ ) {
                        if ( lvl2_key[k] == 'environment' ) {
                            lvl2_obj = lvl1_obj[lvl2_key[k]];
                            lvl3_key = Object.keys(lvl2_obj);

                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                            fs.appendFileSync(dFile, buff);

                            // header 4
                            for ( m=0; m< lvl3_key.length; m++ ) {
                                if ( lvl3_key[m] == 'CORE_PEER_ID' ) {
                                    buff = '  ' + '    - ' + lvl3_key[m] + '=' + tmp_name + '\n';
                                } else if ( lvl3_key[m] == 'CORE_PEER_ADDRESS' ) {
                                    buff = '  ' + '    - ' + lvl3_key[m] + '=' + vp0Addr +':'+ tmp_port + '\n';
                                } else if ( lvl3_key[m] == 'CORE_LEDGER_STATE_COUCHDBCONFIG_COUCHDBADDRESS' ) {
                                    buff = '  ' + '    - ' + lvl3_key[m] + '=' + 'couchdb'+v +':'+ couchdbPort + '\n';
                                } else {
                                    buff = '  ' + '    - ' + lvl3_key[m] + '=' +lvl2_obj[lvl3_key[m]] + '\n';
                                }

                                fs.appendFileSync(dFile, buff);
                            }
                        } else if ( ( lvl2_key[k] == 'image' ) || ( lvl2_key[k] == 'command' ) || ( lvl2_key[k] == 'working_dir' )
                                    || ( lvl2_key[k] == 'restart') ) {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + lvl1_obj[lvl2_key[k]] + '\n';
                            fs.appendFileSync(dFile, buff);

                        } else if ( lvl2_key[k] == 'container_name' ) {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + tmp_name + '\n';
                            fs.appendFileSync(dFile, buff);

                        } else if ( lvl2_key[k] == 'ports' ) {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                            fs.appendFileSync(dFile, buff);

                            // header 4
                            buff = '  ' + '    - ' + tmp_port + ':' + couchdbPort + '\n';
                            fs.appendFileSync(dFile, buff);

                        } else if ( lvl2_key[k] == 'links' ) {
                            var lvl2_obj = lvl1_obj[lvl2_key[k]];
                            //console.log('lvl2_obj: %d ', lvl2_obj.length, lvl2_obj);

                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                            fs.appendFileSync(dFile, buff);
                            buff = '  ' + '    - ' + 'orderer' + '\n';
                            fs.appendFileSync(dFile, buff);
                            if ( CA == 1 ) {
                                buff = '  ' + '    - ' + 'ca' + '\n';
                                fs.appendFileSync(dFile, buff);
                            }
                            // header 4
                            for ( m=0; m< v; m++ ) {
                                buff = '  ' + '    - ' +'peer'+m + '\n';
                                fs.appendFileSync(dFile, buff);
                            }

                        } else if ( ( lvl2_key[k] == 'volumes' ) || ( lvl2_key[k] == 'depends_on' ) ){
                            var lvl2_obj = lvl1_obj[lvl2_key[k]];
                            //console.log('lvl2_obj: %d ', lvl2_obj.length, lvl2_obj);

                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                            fs.appendFileSync(dFile, buff);

                            // header 4
                            for ( m=0; m< lvl2_obj.length; m++ ) {
                                buff = '  ' + '    - ' +lvl2_obj[m] + '\n';
                                fs.appendFileSync(dFile, buff);

                            }

                        } else {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                            fs.appendFileSync(dFile, buff);

                            buff = '  ' + '    - ' + lvl1_obj[lvl2_key[k]] + '\n';
                            fs.appendFileSync(dFile, buff);

                        }
                    }
                    // add a blank line
                    buff = '\n';
                    fs.appendFileSync(dFile, buff);

                }
                }
             } else if (lvl1_key[i] == 'orderer' ) {
                for ( v = 0; v < addOrderer; v++ ) {
                    tmp_name = lvl1_key[i] + v;
                    buff = '  ' + tmp_name +':' + '\n';
                    fs.appendFileSync(dFile, buff);

                    // header 3
                    for ( k=0; k<lvl2_key.length; k++ ) {
                        if ( (lvl2_key[k] == 'environment') ) {
                            lvl2_obj = lvl1_obj[lvl2_key[k]];
                            lvl3_key = Object.keys(lvl2_obj);

                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                            fs.appendFileSync(dFile, buff);

                                // header 4
                                for ( m=0; m< lvl3_key.length; m++ ) {
                                    tmp = ord_env_name.indexOf( lvl3_key[m] );
                                    if ( tmp >= 0 ) {
                                        buff = '  ' + '    - ' + lvl3_key[m] + '=' + ord_env_val[tmp] + '\n';
                                        fs.appendFileSync(dFile, buff);
                                    } else if ( lvl3_key[m] == 'CONFIGTX_ORDERER_KAFKA_BROKERS' ) {
                                        if ( addBroker > 0 ) {
                                            buff = '  ' + '    - ' + lvl3_key[m] + '=[';
                                            for (n=0; n<addBroker; n++) {
                                                buff = buff + 'kafka' + n +':9092' ;
                                                if ( n < (addBroker-1) ) {
                                                     buff = buff + ',';
                                                }
                                            }
                                            buff = buff + ']' + '\n';
                                            fs.appendFileSync(dFile, buff);
                                        }
                                    } else if ( lvl3_key[m] == 'CONFIGTX_ORDERER_ORDERERTYPE' ) {
                                            if ( addBroker > 0 ) {
                                                buff = '  ' + '    - ' + lvl3_key[m] + '=' + 'kafka' + '\n';
                                                fs.appendFileSync(dFile, buff);
                                            } else {
                                                buff = '  ' + '    - ' + lvl3_key[m] + '=' + 'solo' + '\n';
                                                fs.appendFileSync(dFile, buff);
                                            }
                                    } else if ( lvl3_key[m] == 'ORDERER_GENERAL_LISTENPORT' ) {
                                            buff = '  ' + '    - ' + lvl3_key[m] + '=' + ordererPort + '\n';
                                            fs.appendFileSync(dFile, buff);
                                    } else if ( ( lvl3_key[m] == 'ORDERER_GENERAL_GENESISMETHOD' ) || ( lvl3_key[m] == 'ORDERER_GENERAL_GENESISFILE' ) ) {
                                        if ( addBroker > 0 ) {
                                            buff = '  ' + '    - ' + lvl3_key[m] + '=' + lvl2_obj[lvl3_key[m]] + '\n';
                                            fs.appendFileSync(dFile, buff);
                                        } else {
                                            //buff = '  ' + '    - ' + lvl3_key[m] + '=' + 'solo' + '\n';
                                            //fs.appendFileSync(dFile, buff);
                                        }
                                    } else {
                                        buff = '  ' + '    - ' + lvl3_key[m] + '=' +lvl2_obj[lvl3_key[m]] + '\n';
                                        fs.appendFileSync(dFile, buff);
                                    }

                                }
                        } else if ( ( lvl2_key[k] == 'image' ) || ( lvl2_key[k] == 'command' ) || ( lvl2_key[k] == 'working_dir' )
                                    || ( lvl2_key[k] == 'restart') ) {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + lvl1_obj[lvl2_key[k]] + '\n';
                            fs.appendFileSync(dFile, buff);
                        } else if ( lvl2_key[k] == 'container_name' ) {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + tmp_name + '\n';
                            fs.appendFileSync(dFile, buff);
                        } else if ( lvl2_key[k] == 'ports' ) {
                                lvl2_obj = lvl1_obj[lvl2_key[k]];
                                lvl3_key = Object.keys(lvl2_obj);

                                buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                                fs.appendFileSync(dFile, buff);
                                tmp_port = ordererPort + v;

                                buff = '  ' + '    - ' + tmp_port +':' + ordererPort + '\n' ;
                                fs.appendFileSync(dFile, buff);

                        } else if ( (lvl2_key[k] == 'depends_on') ) {
                            if ( addBroker > 0 ) {
                                lvl2_obj = lvl1_obj[lvl2_key[k]];
                                lvl3_key = Object.keys(lvl2_obj);

                                buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                                fs.appendFileSync(dFile, buff);

                                buff = '  ' + '    - ' + 'zookeeper' + '\n' ;
                                fs.appendFileSync(dFile, buff);
                                if ( KAFKA==1 ) {
                                    for (n=0; n<addBroker; n++) {
                                        buff = '  ' + '    - ' + 'kafka' + n + '\n' ;
                                        fs.appendFileSync(dFile, buff);
                                    }
                                }
                            }
                        } else {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                            fs.appendFileSync(dFile, buff);

                            buff = '  ' + '    - ' + lvl1_obj[lvl2_key[k]] + '\n';
                            fs.appendFileSync(dFile, buff);

                        }
                    }
                    // add a blank line
                    buff = '\n';
                    fs.appendFileSync(dFile, buff);

                }
             } else if (lvl1_key[i] == 'kafka' ) {
                for ( v = 0; v < addBroker; v++ ) {
                    tmp_name = lvl1_key[i] + v;
                    tmp_port = vp0Port + v;
                    buff = '  ' + tmp_name +':' + '\n';
                    fs.appendFileSync(dFile, buff);

                    // header 3
                    for ( k=0; k<lvl2_key.length; k++ ) {
                        if ( (lvl2_key[k] == 'environment') ) {
                                lvl2_obj = lvl1_obj[lvl2_key[k]];
                                lvl3_key = Object.keys(lvl2_obj);

                                buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                                fs.appendFileSync(dFile, buff);

                                // header 4
                                for ( m=0; m< lvl3_key.length; m++ ) {
                                    if ( lvl3_key[m] == 'KAFKA_BROKER_ID' ) {
                                        buff = '  ' + '    - ' + lvl3_key[m] + '=' + v + '\n';
                                        fs.appendFileSync(dFile, buff);
                                    } else if ( lvl3_key[m] == 'KAFKA_DEFAULT_REPLICATION_FACTOR' ) {
                                        buff = '  ' + '    - ' + lvl3_key[m] + '=' + addBroker + '\n';
                                        fs.appendFileSync(dFile, buff);
                                    } else {
                                        buff = '  ' + '    - ' + lvl3_key[m] + '=' +lvl2_obj[lvl3_key[m]] + '\n';
                                        fs.appendFileSync(dFile, buff);
                                    }

                                }
                        } else if ( ( lvl2_key[k] == 'image' ) || ( lvl2_key[k] == 'command' ) || ( lvl2_key[k] == 'working_dir' )
                                    || ( lvl2_key[k] == 'restart') ) {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + lvl1_obj[lvl2_key[k]] + '\n';
                            fs.appendFileSync(dFile, buff);
                        } else if ( lvl2_key[k] == 'container_name' ) {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + tmp_name + '\n';
                            fs.appendFileSync(dFile, buff);

                        } else {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                            fs.appendFileSync(dFile, buff);

                            buff = '  ' + '    - ' + lvl1_obj[lvl2_key[k]] + '\n';
                            fs.appendFileSync(dFile, buff);

                        }
                    }
                    // add a blank line
                    buff = '\n';
                    fs.appendFileSync(dFile, buff);

                }
             } else if (lvl1_key[i] == 'zookeeper' ) {
                if ( KAFKA == 1 ) {
                    tmp_name = lvl1_key[i];
                    buff = '  ' + tmp_name +':' + '\n';
                    fs.appendFileSync(dFile, buff);

                    // header 3
                    for ( k=0; k<lvl2_key.length; k++ ) {
                        if ( (lvl2_key[k] == 'environment') ) {
                                lvl2_obj = lvl1_obj[lvl2_key[k]];
                                lvl3_key = Object.keys(lvl2_obj);

                                buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                                fs.appendFileSync(dFile, buff);

                                // header 4
                                for ( m=0; m< lvl3_key.length; m++ ) {
                                        buff = '  ' + '    - ' + lvl3_key[m] + '=' +lvl2_obj[lvl3_key[m]] + '\n';
                                        fs.appendFileSync(dFile, buff);
                                }
                        } else if ( ( lvl2_key[k] == 'image' ) || ( lvl2_key[k] == 'command' ) || ( lvl2_key[k] == 'working_dir' )
                                    || ( lvl2_key[k] == 'restart') ) {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + lvl1_obj[lvl2_key[k]] + '\n';
                            fs.appendFileSync(dFile, buff);
                        } else if ( lvl2_key[k] == 'container_name' ) {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + tmp_name + '\n';
                            fs.appendFileSync(dFile, buff);

                        } else {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                            fs.appendFileSync(dFile, buff);

                            buff = '  ' + '    - ' + lvl1_obj[lvl2_key[k]] + '\n';
                            fs.appendFileSync(dFile, buff);

                        }
                    }
                    // add a blank line
                    buff = '\n';
                    fs.appendFileSync(dFile, buff);

                }
             } else if (lvl1_key[i] == 'peer' ) {
                for ( v = 0; v < addVP; v++ ) {
                    tmp_name = lvl1_key[i] + v;
                    tmp_port = vp0Port + v;
                    buff = '  ' + tmp_name +':' + '\n';
                    fs.appendFileSync(dFile, buff);

                    // header 3
                    for ( k=0; k<lvl2_key.length; k++ ) {
                        if ( (lvl2_key[k] == 'environment') ) {
                            lvl2_obj = lvl1_obj[lvl2_key[k]];
                            lvl3_key = Object.keys(lvl2_obj);

                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                            fs.appendFileSync(dFile, buff);

                                // header 4
                                for ( m=0; m< lvl3_key.length; m++ ) {
                                    tmp = peer_env_name.indexOf( lvl3_key[m] );
                                    if ( tmp >= 0 ) {
                                        buff = '  ' + '    - ' + lvl3_key[m] + '=' + peer_env_val[tmp] + '\n';
                                        fs.appendFileSync(dFile, buff);
                                    } else if ( lvl3_key[m] == 'CORE_PEER_ID' ) {
                                        buff = '  ' + '    - ' + lvl3_key[m] + '=' + tmp_name + '\n';
                                        fs.appendFileSync(dFile, buff);
                                    } else if ( lvl3_key[m] == 'CORE_PEER_NETWORKID' ) {
                                        buff = '  ' + '    - ' + lvl3_key[m] + '=' + tmp_name + '\n';
                                        fs.appendFileSync(dFile, buff);
                                    } else if ( lvl3_key[m] == 'CORE_PEER_ADDRESS' ) {
                                        buff = '  ' + '    - ' + lvl3_key[m] + '=' + vp0Addr +':'+ tmp_port + '\n';
                                        fs.appendFileSync(dFile, buff);
                                    } else if ( lvl3_key[m] == 'CORE_PEER_DISCOVERY_ROOTNODE' ) {
                                        if ( v != 0 ) {
                                            buff = '  ' + '    - ' + lvl3_key[m] + '=' + vp0Addr +':'+ vp0Port + '\n';
                                            fs.appendFileSync(dFile, buff);
                                        }
                                    } else if ( lvl3_key[m] == 'CORE_LEDGER_STATE_STATEDATABASE' ) {
                                        if (dbType == 'couchdb') {
                                            buff = '  ' + '    - ' + lvl3_key[m] + '=' + 'CouchDB' + '\n';
                                            fs.appendFileSync(dFile, buff);
                                        } else if (dbType == 'goleveldb') {
                                            buff = '  ' + '    - ' + lvl3_key[m] + '=' + 'goleveldb' + '\n';
                                            fs.appendFileSync(dFile, buff);
                                        }
                                    } else if ( lvl3_key[m] == 'CORE_LEDGER_STATE_COUCHDBCONFIG_COUCHDBADDRESS' ) {
                                        if (dbType == 'couchdb') {
                                            tmp = couchdbPort+v;
                                            buff = '  ' + '    - ' + lvl3_key[m] + '=' + 'couchdb'+v +':'+ tmp + '\n';
                                            fs.appendFileSync(dFile, buff);
                                        }
                                    } else {
                                        buff = '  ' + '    - ' + lvl3_key[m] + '=' +lvl2_obj[lvl3_key[m]] + '\n';
                                        fs.appendFileSync(dFile, buff);
                                    }

                                }
                        } else if ( ( lvl2_key[k] == 'image' ) || ( lvl2_key[k] == 'command' ) || ( lvl2_key[k] == 'working_dir' )
                                    || ( lvl2_key[k] == 'restart') ) {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + lvl1_obj[lvl2_key[k]] + '\n';
                            fs.appendFileSync(dFile, buff);

                        } else if ( lvl2_key[k] == 'container_name' ) {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + tmp_name + '\n';
                            fs.appendFileSync(dFile, buff);

                        } else if ( lvl2_key[k] == 'ports' ) {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                            fs.appendFileSync(dFile, buff);

                            // header 4
                            buff = '  ' + '    - ' + tmp_port + ':' + vp0Port + '\n';
                            fs.appendFileSync(dFile, buff);

                        } else if ( lvl2_key[k] == 'links' ) {
                            var lvl2_obj = lvl1_obj[lvl2_key[k]];
                            //console.log('lvl2_obj: %d ', lvl2_obj.length, lvl2_obj);

                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                            fs.appendFileSync(dFile, buff);
                            buff = '  ' + '    - ' + 'orderer0' + '\n';
                            fs.appendFileSync(dFile, buff);
                            if (dbType == 'couchdb') {
                                buff = '  ' + '    - ' + 'couchdb'+v + '\n';
                                fs.appendFileSync(dFile, buff);
                            }
                            if ( CA == 1 ) {
                                buff = '  ' + '    - ' + 'ca' + '\n';
                                fs.appendFileSync(dFile, buff);
                            }
                            // header 4
                            for ( m=0; m< v; m++ ) {
                                buff = '  ' + '    - ' +'peer'+m + '\n';
                                fs.appendFileSync(dFile, buff);
                            }

                        } else if ( lvl2_key[k] == 'volumes' ) {
                            var lvl2_obj = lvl1_obj[lvl2_key[k]];
                            //console.log('lvl2_obj: %d ', lvl2_obj.length, lvl2_obj);

                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                            fs.appendFileSync(dFile, buff);

                            // header 4
                            for ( m=0; m< lvl2_obj.length; m++ ) {
                                buff = '  ' + '    - ' +lvl2_obj[m] + '\n';
                                fs.appendFileSync(dFile, buff);

                            }

                        } else if ( lvl2_key[k] == 'depends_on'  ){
                            var lvl2_obj = lvl1_obj[lvl2_key[k]];

                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                            fs.appendFileSync(dFile, buff);

                            // header 4
                            for ( m=0; m< lvl2_obj.length; m++ ) {
                                buff = '  ' + '    - ' +lvl2_obj[m] + '\n';
                                fs.appendFileSync(dFile, buff);
                            }

                        } else {
                            buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                            fs.appendFileSync(dFile, buff);

                            buff = '  ' + '    - ' + lvl1_obj[lvl2_key[k]] + '\n';
                            fs.appendFileSync(dFile, buff);

                        }
                    }
                    // add a blank line
                    buff = '\n';
                    fs.appendFileSync(dFile, buff);

                }
             } else if (lvl1_key[i] == 'cli' ) {
                buff = '  ' + lvl1_key[i] +':' + '\n';
                fs.appendFileSync(dFile, buff);

                // header 3
                for ( k=0; k<lvl2_key.length; k++ ) {
                    if ( lvl2_key[k] == 'environment' ) {
                        lvl2_obj = lvl1_obj[lvl2_key[k]];
                        lvl3_key = Object.keys(lvl2_obj);
                        //console.log('lvl2_obj: ', lvl2_obj);
                        //console.log('lvl3_key: ', lvl3_key);

                        buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                        fs.appendFileSync(dFile, buff);

                        // header 4
                        for ( m=0; m< lvl3_key.length; m++ ) {
                            /*if ( lvl3_key[m] == 'CORE_PEER_COMMITTER_LEDGER_ORDERER' ) {
                                buff = '  ' + '    - ' + lvl3_key[m] + '=' + 'orderer0' +':'+ ordererPort + '\n';
                                fs.appendFileSync(dFile, buff);
                            } else {*/
                                buff = '  ' + '    - ' + lvl3_key[m] + '=' +lvl2_obj[lvl3_key[m]] + '\n';
                                fs.appendFileSync(dFile, buff);
                            //}

                        }
                    } else if ( ( lvl2_key[k] == 'image' ) || ( lvl2_key[k] == 'command' ) || ( lvl2_key[k] == 'working_dir' )
                            || ( lvl2_key[k] == 'restart') || ( lvl2_key[k] == 'container_name') || ( lvl2_key[k] == 'tty') ) {
                        buff = '  ' + '  ' + lvl2_key[k] + ': ' + lvl1_obj[lvl2_key[k]] + '\n';
                        fs.appendFileSync(dFile, buff);

                    } else if ( lvl2_key[k] == 'links' ) {
                        var lvl2_obj = lvl1_obj[lvl2_key[k]];
                        //console.log('lvl2_obj: %d ', lvl2_obj.length, lvl2_obj);

                        buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                        fs.appendFileSync(dFile, buff);

                        // header 4
                            buff = '  ' + '    - ' +'orderer0:orderer0' + '\n';
                            fs.appendFileSync(dFile, buff);
                            buff = '  ' + '    - ' +'peer0:peer0' + '\n';
                            fs.appendFileSync(dFile, buff);

                    } else if ( lvl2_key[k] == 'depends_on' ) {
                        var lvl2_obj = lvl1_obj[lvl2_key[k]];
                        //console.log('lvl2_obj: %d ', lvl2_obj.length, lvl2_obj);

                        buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                        fs.appendFileSync(dFile, buff);

                        // header 4
                            buff = '  ' + '    - ' +'orderer0' + '\n';
                            fs.appendFileSync(dFile, buff);
                            buff = '  ' + '    - ' +'peer0' + '\n';
                            fs.appendFileSync(dFile, buff);

                    } else if ( ( lvl2_key[k] == 'ports' ) || ( lvl2_key[k] == 'volumes' )  ){
                        var lvl2_obj = lvl1_obj[lvl2_key[k]];
                        //console.log('lvl2_obj: %d ', lvl2_obj.length, lvl2_obj);

                        buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                        fs.appendFileSync(dFile, buff);

                        // header 4
                        for ( m=0; m< lvl2_obj.length; m++ ) {
                            buff = '  ' + '    - ' +lvl2_obj[m] + '\n';
                            fs.appendFileSync(dFile, buff);

                        }

                    } else {
                        buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                        fs.appendFileSync(dFile, buff);

                        buff = '  ' + '    - ' + lvl1_obj[lvl2_key[k]] + '\n';
                        fs.appendFileSync(dFile, buff);

                    }
                }
                // add a blank line
                buff = '\n';
                fs.appendFileSync(dFile, buff);

             } else {
                if (lvl1_key[i] == 'ca' ) {
                    CA=1;
                }
                buff = '  ' + lvl1_key[i] +':' + '\n';
                fs.appendFileSync(dFile, buff);

                // header 3
                for ( k=0; k<lvl2_key.length; k++ ) {
                    if ( lvl2_key[k] == 'environment' ) {
                        lvl2_obj = lvl1_obj[lvl2_key[k]];
                        lvl3_key = Object.keys(lvl2_obj);
                        //console.log('lvl2_obj: ', lvl2_obj);
                        //console.log('lvl3_key: ', lvl3_key);

                        buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                        fs.appendFileSync(dFile, buff);

                        // header 4
                        for ( m=0; m< lvl3_key.length; m++ ) {
                            buff = '  ' + '    - ' + lvl3_key[m] + '=' +lvl2_obj[lvl3_key[m]] + '\n';
                            fs.appendFileSync(dFile, buff);

                        }
                    } else if ( ( lvl2_key[k] == 'image' ) || ( lvl2_key[k] == 'command' ) || ( lvl2_key[k] == 'working_dir' )
                            || ( lvl2_key[k] == 'restart') || ( lvl2_key[k] == 'container_name') || ( lvl2_key[k] == 'tty') ) {
                        buff = '  ' + '  ' + lvl2_key[k] + ': ' + lvl1_obj[lvl2_key[k]] + '\n';
                        fs.appendFileSync(dFile, buff);

                    } else if ( ( lvl2_key[k] == 'ports' ) || ( lvl2_key[k] == 'links' ) || ( lvl2_key[k] == 'volumes' )
                            || ( lvl2_key[k] == 'depends_on' ) ){
                        var lvl2_obj = lvl1_obj[lvl2_key[k]];
                        //console.log('lvl2_obj: %d ', lvl2_obj.length, lvl2_obj);

                        buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                        fs.appendFileSync(dFile, buff);

                        // header 4
                        for ( m=0; m< lvl2_obj.length; m++ ) {
                            buff = '  ' + '    - ' +lvl2_obj[m] + '\n';
                            fs.appendFileSync(dFile, buff);

                        }

                    } else {
                        buff = '  ' + '  ' + lvl2_key[k] + ': ' + '\n';
                        fs.appendFileSync(dFile, buff);

                        buff = '  ' + '    - ' + lvl1_obj[lvl2_key[k]] + '\n';
                        fs.appendFileSync(dFile, buff);

                    }
                }
                // add a blank line
                buff = '\n';
                fs.appendFileSync(dFile, buff);

            }
        }
    }
}
