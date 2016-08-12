/*
Copyright DTCC 2016 All Rights Reserved.

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

package org.hyperledger.java.shim;

import com.google.protobuf.ByteString;
import com.google.protobuf.InvalidProtocolBufferException;
import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;
import org.hyperledger.protos.Chaincode;
import org.hyperledger.protos.TableProto;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

import static org.hyperledger.protos.TableProto.ColumnDefinition.Type.STRING;

public class ChaincodeStub {
    private static Log logger = LogFactory.getLog(ChaincodeStub.class);
    private final String uuid;
    private final Handler handler;

    public ChaincodeStub(String uuid, Handler handler) {
        this.uuid = uuid;
        this.handler = handler;
    }

    /**
     * Gets the UUID of this stub
     *
     * @return the id used to identify this communication channel
     */
    public String getUuid() {
        return uuid;
    }

    /**
     * Get the state of the provided key from the ledger, and returns is as a string
     *
     * @param key the key of the desired state
     * @return the String value of the requested state
     */
    public String getState(String key) {
        return handler.handleGetState(key, uuid).toStringUtf8();
    }

    /**
     * Puts the given state into a ledger, automatically wrapping it in a ByteString
     *
     * @param key   reference key
     * @param value value to be put
     */
    public void putState(String key, String value) {
        handler.handlePutState(key, ByteString.copyFromUtf8(value), uuid);
    }

    /**
     * Deletes the state of the given key from the ledger
     *
     * @param key key of the state to be deleted
     */
    public void delState(String key) {
        handler.handleDeleteState(key, uuid);
    }

    /**
     * Given a start key and end key, this method returns a map of items with value converted to UTF-8 string.
     *
     * @param startKey
     * @param endKey
     * @return
     */
    public Map<String, String> rangeQueryState(String startKey, String endKey) {
        Map<String, String> retMap = new HashMap<>();
        for (Map.Entry<String, ByteString> item : rangeQueryRawState(startKey, endKey).entrySet()) {
            retMap.put(item.getKey(), item.getValue().toStringUtf8());
        }
        return retMap;
    }

    /**
     * This method is same as rangeQueryState, except it returns value in ByteString, useful in cases where
     * serialized object can be retrieved.
     *
     * @param startKey
     * @param endKey
     * @return
     */
    public Map<String, ByteString> rangeQueryRawState(String startKey, String endKey) {
        Map<String, ByteString> map = new HashMap<>();
        for (Chaincode.RangeQueryStateKeyValue mapping : handler.handleRangeQueryState(
                startKey, endKey, uuid).getKeysAndValuesList()) {
            map.put(mapping.getKey(), mapping.getValue());
        }
        return map;
    }

    /**
     * @param chaincodeName
     * @param function
     * @param args
     * @return
     */
    public String invokeChaincode(String chaincodeName, String function, List<ByteString> args) {
        return handler.handleInvokeChaincode(chaincodeName, function, args, uuid).toStringUtf8();
    }

    /**
     * @param chaincodeName
     * @param function
     * @param args
     * @return
     */
    public String queryChaincode(String chaincodeName, String function, List<ByteString> args) {
        return handler.handleQueryChaincode(chaincodeName, function, args, uuid).toStringUtf8();
    }

    //------RAW CALLS------

    /**
     * @param key
     * @return
     */
    public ByteString getRawState(String key) {
        return handler.handleGetState(key, uuid);
    }

    /**
     * @param key
     * @param value
     */
    public void putRawState(String key, ByteString value) {
        handler.handlePutState(key, value, uuid);
    }

    /**
     *
     * @param startKey
     * @param endKey
     * @param limit
     * @return
     */
//	public RangeQueryStateResponse rangeQueryRawState(String startKey, String endKey, int limit) {
//		return handler.handleRangeQueryState(startKey, endKey, limit, uuid);
//	}

    /**
     * @param chaincodeName
     * @param function
     * @param args
     * @return
     */
    public ByteString queryRawChaincode(String chaincodeName, String function, List<ByteString> args) {
        return handler.handleQueryChaincode(chaincodeName, function, args, uuid);
    }

    /**
     * Invokes the provided chaincode with the given function and arguments, and returns the
     * raw ByteString value that invocation generated.
     *
     * @param chaincodeName The name of the chaincode to invoke
     * @param function      the function parameter to pass to the chaincode
     * @param args          the arguments to be provided in the chaincode call
     * @return the value returned by the chaincode call
     */
    public ByteString invokeRawChaincode(String chaincodeName, String function, List<ByteString> args) {
        return handler.handleInvokeChaincode(chaincodeName, function, args, uuid);
    }

    public boolean createTable(String tableName, List<TableProto.ColumnDefinition> columnDefinitions)
            throws Exception    {
        if (validateTableName(tableName)) {
            logger.debug("Table name %s is valid, continue table creation");

            if (tableExist(tableName)) {
                logger.error("Table with tableName already exist, Create table operation failed");
                return false;//table exist
            } else {
                if (columnDefinitions != null && columnDefinitions.size() == 0) {
                    logger.error("Invalid column definitions. Table must contain at least one column");
                    return false;
                }
                Map<String, Boolean> nameMap = new HashMap<>();
                int idx = 0;
                boolean hasKey = false;
                logger.debug("Number of columns " + columnDefinitions.size());
                for (TableProto.ColumnDefinition colDef : columnDefinitions) {
                    logger.debug("Col information - " + colDef.getName()+ "=" + colDef.getType()+ "=" +colDef.isInitialized());

                    if (!colDef.isInitialized() || colDef.getName().length() == 0) {
                        logger.error("Column definition is invalid for index " + idx);
                    return false;
                    }

                    if (!nameMap.isEmpty() && nameMap.containsKey(colDef.getName())){
                        logger.error("Column already exist for colIdx " + idx + " with name " + colDef.getName());
                        return false;
                    }
                    nameMap.put(colDef.getName(), true);
                    switch (colDef.getType()) {
                        case STRING:
                            break;
                        case INT32:
                            break;
                        case INT64:
                            break;
                        case UINT32:
                            break;
                        case UINT64:
                            break;
                        case BYTES:
                            break;
                        case BOOL:
                            break;
                        default:
                            logger.error("Invalid column type for index " + idx + " given type " + colDef.getType());
//                            return false;

                    }

                    if (colDef.getKey()) hasKey = true;

                    idx++;
                }
                if (!hasKey) {
                    logger.error("Invalid table. One or more columns must be a key.");
                    return false;
                }
                TableProto.Table table = TableProto.Table.newBuilder()
                        .setName(tableName)
                        .addAllColumnDefinitions(columnDefinitions)
                        .build();
                String tableNameKey = getTableNameKey(tableName);
                putRawState(tableNameKey, table.toByteString());
                return true;
            }
        }
        return false;
    }
    public boolean deleteTable(String tableName) {
        String tableNameKey = getTableNameKey(tableName);
        rangeQueryState(tableNameKey + "1", tableNameKey + ":")
                .keySet().forEach(key -> delState(key));
        delState(tableNameKey);
        return true;
    }
    public boolean insertRow(String tableName, TableProto.Row row) throws Exception {
        try {
            return insertRowInternal(tableName, row, false);
         } catch (Exception e) {
            logger.error("Error while inserting row on table - " + tableName);
            logger.error(e.getMessage());

            throw e;
        }
    }

    public boolean replaceRow(String tableName, TableProto.Row row) throws Exception {
        try {
            return insertRowInternal(tableName, row, true);
        } catch (Exception e) {
            logger.error("Error while updating row on table - " + tableName);
            logger.error(e.getMessage());

            throw e;
        }
    }
    private List<TableProto.Column> getKeyAndVerifyRow(TableProto.Table table, TableProto.Row row) throws Exception {
        List keys  = new ArrayList();
        //logger.debug("Entering getKeyAndVerifyRow with tableName -" + table.getName() );
        //logger.debug("Entering getKeyAndVerifyRow with rowcount -" + row.getColumnsCount() );
        if ( !row.isInitialized() || row.getColumnsCount() != table.getColumnDefinitionsCount()){
            logger.error("Table " + table.getName() + " define "
            + table.getColumnDefinitionsCount() + " columns but row has "
            + row.getColumnsCount() + " columns");
            return keys;
        }
        int colIdx = 0;
        for (TableProto.Column col: row.getColumnsList()) {
            boolean expectedType;
            switch (col.getValueCase()){
                case STRING:
                    expectedType = table.getColumnDefinitions(colIdx).getType()
                            == STRING;
                    break;
                case INT32:
                    expectedType = table.getColumnDefinitions(colIdx).getType()
                            == TableProto.ColumnDefinition.Type.INT32;
                    break;
                case INT64:
                    expectedType = table.getColumnDefinitions(colIdx).getType()
                            == TableProto.ColumnDefinition.Type.INT64;
                    break;
                case UINT32:
                    expectedType = table.getColumnDefinitions(colIdx).getType()
                            == TableProto.ColumnDefinition.Type.UINT32;
                    break;
                case UINT64:
                    expectedType = table.getColumnDefinitions(colIdx).getType()
                            == TableProto.ColumnDefinition.Type.UINT64;
                    break;
                case BYTES:
                    expectedType = table.getColumnDefinitions(colIdx).getType()
                            == TableProto.ColumnDefinition.Type.BYTES;
                    break;
                case BOOL:
                    expectedType = table.getColumnDefinitions(colIdx).getType()
                            == TableProto.ColumnDefinition.Type.BOOL;
                    break;
                default:
                    expectedType = false;
            }
            if (!expectedType){
                logger.error("The type for table " + table.getName()
                        + " column " + table.getColumnDefinitions(colIdx).getName() + " is "
                        + table.getColumnDefinitions(colIdx).getType() +  " but the column in the row does not match" );
                throw new Exception();
            }
            if (table.getColumnDefinitions(colIdx).getKey()){
                keys.add(col);
            }

        colIdx++;
        }
        return keys;
    }
    private boolean isRowPresent(String tableName, List<TableProto.Column> keys){
        String keyString = buildKeyString(tableName, keys);
        ByteString rowBytes =   getRawState(keyString);
        return  !rowBytes.isEmpty();
    }

    private String buildKeyString(String tableName, List<TableProto.Column> keys){

        StringBuffer sb = new StringBuffer();
        String tableNameKey = getTableNameKey(tableName);

        sb.append(tableNameKey);
        String keyString="";
        for (TableProto.Column col: keys) {

            switch (col.getValueCase()){
                case STRING:
                    keyString = col.getString();
                    break;
                case INT32:
                    keyString = ""+col.getInt32();
                    break;
                case INT64:
                    keyString = ""+col.getInt64();
                    break;
                case UINT32:
                    keyString = ""+col.getUint32();
                    break;
                case UINT64:
                    keyString = ""+col.getUint64();
                    break;
                case BYTES:
                    keyString = col.getBytes().toString();
                    break;
                case BOOL:
                    keyString = ""+col.getBool();
                    break;
            }

            sb.append(keyString.length());
            sb.append(keyString);

        }
        return sb.toString();
    }
    public TableProto.Row getRow(String tableName, List<TableProto.Column> key) throws InvalidProtocolBufferException {

        String keyString = buildKeyString(tableName, key);
        try {
            return TableProto.Row.parseFrom(getRawState(keyString));
        } catch (InvalidProtocolBufferException e) {

            logger.error("Error while retrieving row on table -" + tableName);
            throw e;
        }
    }

    public boolean deleteRow(String tableName, List<TableProto.Column> key){
        String keyString = buildKeyString(tableName, key);
        delState(keyString);
        return true;
    }

    private boolean insertRowInternal(String tableName, TableProto.Row row, boolean update)
    throws  Exception{
        try {
            //logger.debug("inside insertRowInternal with tname " + tableName);
            TableProto.Table table = getTable(tableName);
            //logger.debug("inside insertRowInternal with tableName " + table.getName());
            List<TableProto.Column> keys = getKeyAndVerifyRow(table, row);
            Boolean present = isRowPresent(tableName, keys);
            if((present && !update) || (!present && update)){
                return false;
            }
            String keyString = buildKeyString(tableName, keys);
            putRawState(keyString, row.toByteString());
        } catch (Exception e) {
            logger.error("Unable to insert/update table -" + tableName);
            logger.error(e.getMessage());
            throw e;
        }

        return true;
    }

    private TableProto.Table getTable(String tableName) throws Exception {
logger.info("Inside get tbale");
        String tName = getTableNameKey(tableName);
        logger.debug("Table name key for getRawState - " + tName);
        ByteString tableBytes = getRawState(tName);
        logger.debug("Table after getrawState -" + tableBytes);
        return TableProto.Table.parseFrom(tableBytes);
    }

    private boolean tableExist(String tableName) throws Exception {
        boolean tableExist = false;
        //TODO Better way to check table existence ?
        if (getTable(tableName).getName().equals(tableName)) {
            tableExist = true;
        }
        return tableExist;
    }

    private String getTableNameKey(String name) {
        return name.length() + name;
    }

    public boolean validateTableName(String name) throws Exception {
        boolean validTableName = true;
        if (name.length() == 0) {
            validTableName = false;
        }
        return validTableName;
    }
}
