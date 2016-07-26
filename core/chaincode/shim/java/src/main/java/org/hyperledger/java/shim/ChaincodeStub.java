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

public class ChaincodeStub {

	private final String uuid;
	private final Handler handler;

	public ChaincodeStub(String uuid, Handler handler) {
		this.uuid = uuid;
		this.handler = handler;
	}
	
	/**
	 * Gets the UUID of this stub
	 * @return the id used to identify this communication channel
	 */
	public String getUuid() {
		return uuid;
	}
	
	/**
	 * Get the state of the provided key from the ledger, and returns is as a string
	 * @param key the key of the desired state
	 * @return the String value of the requested state
	 */
	public String getState(String key) {
		return handler.handleGetState(key, uuid).toStringUtf8();
	}

	/**
	 * Puts the given state into a ledger, automatically wrapping it in a ByteString
	 * @param key reference key
	 * @param value value to be put
	 */
	public void putState(String key, String value) {
		handler.handlePutState(key, ByteString.copyFromUtf8(value), uuid);
	}

	/**
	 * Deletes the state of the given key from the ledger
	 * @param key key of the state to be deleted
	 */
	public void delState(String key) {
		handler.handleDeleteState(key, uuid);
	}

	/**
	 * 
	 * @param startKey
	 * @param endKey
	 * @param limit
	 * @return
	 */
//	public HashMap<String, String> rangeQueryState(String startKey, String endKey, int limit) {
//		HashMap<String, String> map = new HashMap<>();
//		for (RangeQueryStateKeyValue mapping : handler.handleRangeQueryState(
//				startKey, endKey, limit, uuid).getKeysAndValuesList()) {
//			map.put(mapping.getKey(), mapping.getValue().toStringUtf8());
//		}
//		return map;
//	}

	/**
	 * 
	 * @param chaincodeName
	 * @param function
	 * @param args
	 * @return
	 */
	public String invokeChaincode(String chaincodeName, String function, String[] args) {
		return handler.handleInvokeChaincode(chaincodeName, function, args, uuid).toStringUtf8();
	}
	
	/**
	 * 
	 * @param chaincodeName
	 * @param function
	 * @param args
	 * @return
	 */
	public String queryChaincode(String chaincodeName, String function, String[] args) {
		return handler.handleQueryChaincode(chaincodeName, function, args, uuid).toStringUtf8();
	}

	//------RAW CALLS------
	
	/**
	 * 
	 * @param key
	 * @return
	 */
	public ByteString getRawState(String key) {
		return handler.handleGetState(key, uuid);
	}

	/**
	 * 
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
	 * 
	 * @param chaincodeName
	 * @param function
	 * @param args
	 * @return
	 */
	public ByteString queryRawChaincode(String chaincodeName, String function, String[] args) {
		return handler.handleQueryChaincode(chaincodeName, function, args, uuid);
	}
	
	/**
	 * Invokes the provided chaincode with the given function and arguments, and returns the
	 * raw ByteString value that invocation generated. 
	 * @param chaincodeName The name of the chaincode to invoke
	 * @param function the function parameter to pass to the chaincode
	 * @param args the arguments to be provided in the chaincode call
	 * @return the value returned by the chaincode call
	 */
	public ByteString invokeRawChaincode(String chaincodeName, String function, String[] args) {
		return handler.handleInvokeChaincode(chaincodeName, function, args, uuid);
	}
	
	
//	//TODO Table calls
	public void createTable(String name) {
		
//		if (getTable(name) != null)
//			throw new RuntimeException("CreateTable operation failed. Table %s already exists.");
		
//		if err != ErrTableNotFound {
//			return fmt.Errorf("CreateTable operation failed. %s", err)
//		}
//
//		if columnDefinitions == nil || len(columnDefinitions) == 0 {
//			return errors.New("Invalid column definitions. Tables must contain at least one column.")
//		}
//
//		hasKey := false
//		nameMap := make(map[string]bool)
//		for i, definition := range columnDefinitions {
//
//			// Check name
//			if definition == nil {
//				return fmt.Errorf("Column definition %d is invalid. Definition must not be nil.", i)
//			}
//			if len(definition.Name) == 0 {
//				return fmt.Errorf("Column definition %d is invalid. Name must be 1 or more characters.", i)
//			}
//			if _, exists := nameMap[definition.Name]; exists {
//				return fmt.Errorf("Invalid table. Table contains duplicate column name '%s'.", definition.Name)
//			}
//			nameMap[definition.Name] = true
//
//			// Check type
//			switch definition.Type {
//			case ColumnDefinition_STRING:
//			case ColumnDefinition_INT32:
//			case ColumnDefinition_INT64:
//			case ColumnDefinition_UINT32:
//			case ColumnDefinition_UINT64:
//			case ColumnDefinition_BYTES:
//			case ColumnDefinition_BOOL:
//			default:
//				return fmt.Errorf("Column definition %s does not have a valid type.", definition.Name)
//			}
//
//			if definition.Key {
//				hasKey = true
//			}
//		}
//
//		if !hasKey {
//			return errors.New("Inavlid table. One or more columns must be a key.")
//		}
//
//		table := &Table{name, columnDefinitions}
//		tableBytes, err := proto.Marshal(table)
//		if err != nil {
//			return fmt.Errorf("Error marshalling table: %s", err)
//		}
//		tableNameKey, err := getTableNameKey(name)
//		if err != nil {
//			return fmt.Errorf("Error creating table key: %s", err)
//		}
//		try {
//			stub.PutState(tableNameKey, tableBytes)
//		} catch (Exception e) {
//			throw new RuntimeException("Error inserting table in state: " + e.getMessage());			
//		}
//		return;
	}
//
//	// GetTable returns the table for the specified table name or ErrTableNotFound
//	// if the table does not exist.
//	func (stub *ChaincodeStub) GetTable(tableName string) (*Table, error) {
//		return stub.getTable(tableName)
//	}
//
//	// DeleteTable deletes and entire table and all associated row
//	func (stub *ChaincodeStub) DeleteTable(tableName string) error {
//		tableNameKey, err := getTableNameKey(tableName)
//		if err != nil {
//			return err
//		}
//
//		// Delete rows
//		iter, err := stub.RangeQueryState(tableNameKey+"1", tableNameKey+":")
//		if err != nil {
//			return fmt.Errorf("Error deleting table: %s", err)
//		}
//		defer iter.Close()
//		for iter.HasNext() {
//			key, _, err := iter.Next()
//			if err != nil {
//				return fmt.Errorf("Error deleting table: %s", err)
//			}
//			err = stub.DelState(key)
//			if err != nil {
//				return fmt.Errorf("Error deleting table: %s", err)
//			}
//		}
//
//		return stub.DelState(tableNameKey)
//	}
//
//	// InsertRow inserts a new row into the specified table.
//	// Returns -
//	// true and no error if the row is successfully inserted.
//	// false and no error if a row already exists for the given key.
//	// false and a TableNotFoundError if the specified table name does not exist.
//	// false and an error if there is an unexpected error condition.
//	func (stub *ChaincodeStub) InsertRow(tableName string, row Row) (bool, error) {
//		return stub.insertRowInternal(tableName, row, false)
//	}
//
//	// ReplaceRow updates the row in the specified table.
//	// Returns -
//	// true and no error if the row is successfully updated.
//	// false and no error if a row does not exist the given key.
//	// flase and a TableNotFoundError if the specified table name does not exist.
//	// false and an error if there is an unexpected error condition.
//	func (stub *ChaincodeStub) ReplaceRow(tableName string, row Row) (bool, error) {
//		return stub.insertRowInternal(tableName, row, true)
//	}
//
//	// GetRow fetches a row from the specified table for the given key.
//	func (stub *ChaincodeStub) GetRow(tableName string, key []Column) (Row, error) {
//
//		var row Row
//
//		keyString, err := buildKeyString(tableName, key)
//		if err != nil {
//			return row, err
//		}
//
//		rowBytes, err := stub.GetState(keyString)
//		if err != nil {
//			return row, fmt.Errorf("Error fetching row from DB: %s", err)
//		}
//
//		err = proto.Unmarshal(rowBytes, &row)
//		if err != nil {
//			return row, fmt.Errorf("Error unmarshalling row: %s", err)
//		}
//
//		return row, nil
//
//	}
//
//	// GetRows returns multiple rows based on a partial key. For example, given table
//	// | A | B | C | D |
//	// where A, C and D are keys, GetRows can be called with [A, C] to return
//	// all rows that have A, C and any value for D as their key. GetRows could
//	// also be called with A only to return all rows that have A and any value
//	// for C and D as their key.
//	func (stub *ChaincodeStub) GetRows(tableName string, key []Column) (<-chan Row, error) {
//
//		keyString, err := buildKeyString(tableName, key)
//		if err != nil {
//			return nil, err
//		}
//
//		iter, err := stub.RangeQueryState(keyString+"1", keyString+":")
//		if err != nil {
//			return nil, fmt.Errorf("Error fetching rows: %s", err)
//		}
//		defer iter.Close()
//
//		rows := make(chan Row)
//
//		go func() {
//			for iter.HasNext() {
//				_, rowBytes, err := iter.Next()
//				if err != nil {
//					close(rows)
//				}
//
//				var row Row
//				err = proto.Unmarshal(rowBytes, &row)
//				if err != nil {
//					close(rows)
//				}
//
//				rows <- row
//
//			}
//			close(rows)
//		}()
//
//		return rows, nil
//
//	}
//
//	// DeleteRow deletes the row for the given key from the specified table.
//	func (stub *ChaincodeStub) DeleteRow(tableName string, key []Column) error {
//
//		keyString, err := buildKeyString(tableName, key)
//		if err != nil {
//			return err
//		}
//
//		err = stub.DelState(keyString)
//		if err != nil {
//			return fmt.Errorf("DeleteRow operation error. Error deleting row: %s", err)
//		}
//
//		return nil
//	}
//
//	// VerifySignature ...
//	func (stub *ChaincodeStub) VerifySignature(certificate, signature, message []byte) (bool, error) {
//		// Instantiate a new SignatureVerifier
//		sv := ecdsa.NewX509ECDSASignatureVerifier()
//
//		// Verify the signature
//		return sv.Verify(certificate, signature, message)
//	}
//
//	// GetCallerCertificate returns caller certificate
//	func (stub *ChaincodeStub) GetCallerCertificate() ([]byte, error) {
//		return stub.securityContext.CallerCert, nil
//	}
//
//	// GetCallerMetadata returns caller metadata
//	func (stub *ChaincodeStub) GetCallerMetadata() ([]byte, error) {
//		return stub.securityContext.Metadata, nil
//	}
//
//	// GetBinding returns tx binding
//	func (stub *ChaincodeStub) GetBinding() ([]byte, error) {
//		return stub.securityContext.Binding, nil
//	}
//
//	// GetPayload returns tx payload
//	func (stub *ChaincodeStub) GetPayload() ([]byte, error) {
//		return stub.securityContext.Payload, nil
//	}
//
//	func (stub *ChaincodeStub) getTable(tableName string) (*Table, error) {
//
//		tableName, err := getTableNameKey(tableName)
//		if err != nil {
//			return nil, err
//		}
//
//		tableBytes, err := stub.GetState(tableName)
//		if tableBytes == nil {
//			return nil, ErrTableNotFound
//		}
//		if err != nil {
//			return nil, fmt.Errorf("Error fetching table: %s", err)
//		}
//		table := &Table{}
//		err = proto.Unmarshal(tableBytes, table)
//		if err != nil {
//			return nil, fmt.Errorf("Error unmarshalling table: %s", err)
//		}
//
//		return table, nil
//	}
//
//	func validateTableName(name string) error {
//		if len(name) == 0 {
//			return errors.New("Inavlid table name. Table name must be 1 or more characters.")
//		}
//
//		return nil
//	}
//
//	func getTableNameKey(name string) (string, error) {
//		err := validateTableName(name)
//		if err != nil {
//			return "", err
//		}
//
//		return strconv.Itoa(len(name)) + name, nil
//	}
//
//	func buildKeyString(tableName string, keys []Column) (string, error) {
//
//		var keyBuffer bytes.Buffer
//
//		tableNameKey, err := getTableNameKey(tableName)
//		if err != nil {
//			return "", err
//		}
//
//		keyBuffer.WriteString(tableNameKey)
//
//		for _, key := range keys {
//
//			var keyString string
//			switch key.Value.(type) {
//			case *Column_String_:
//				keyString = key.GetString_()
//			case *Column_Int32:
//				// b := make([]byte, 4)
//				// binary.LittleEndian.PutUint32(b, uint32(key.GetInt32()))
//				// keyBuffer.Write(b)
//				keyString = strconv.FormatInt(int64(key.GetInt32()), 10)
//			case *Column_Int64:
//				keyString = strconv.FormatInt(key.GetInt64(), 10)
//			case *Column_Uint32:
//				keyString = strconv.FormatUint(uint64(key.GetInt32()), 10)
//			case *Column_Uint64:
//				keyString = strconv.FormatUint(key.GetUint64(), 10)
//			case *Column_Bytes:
//				keyString = string(key.GetBytes())
//			case *Column_Bool:
//				keyString = strconv.FormatBool(key.GetBool())
//			}
//
//			keyBuffer.WriteString(strconv.Itoa(len(keyString)))
//			keyBuffer.WriteString(keyString)
//		}
//
//		return keyBuffer.String(), nil
//	}
//
//	func getKeyAndVerifyRow(table Table, row Row) ([]Column, error) {
//
//		var keys []Column
//
//		if row.Columns == nil || len(row.Columns) != len(table.ColumnDefinitions) {
//			return keys, fmt.Errorf("Table '%s' defines %d columns, but row has %d columns.",
//				table.Name, len(table.ColumnDefinitions), len(row.Columns))
//		}
//
//		for i, column := range row.Columns {
//
//			// Check types
//			var expectedType bool
//			switch column.Value.(type) {
//			case *Column_String_:
//				expectedType = table.ColumnDefinitions[i].Type == ColumnDefinition_STRING
//			case *Column_Int32:
//				expectedType = table.ColumnDefinitions[i].Type == ColumnDefinition_INT32
//			case *Column_Int64:
//				expectedType = table.ColumnDefinitions[i].Type == ColumnDefinition_INT64
//			case *Column_Uint32:
//				expectedType = table.ColumnDefinitions[i].Type == ColumnDefinition_UINT32
//			case *Column_Uint64:
//				expectedType = table.ColumnDefinitions[i].Type == ColumnDefinition_UINT64
//			case *Column_Bytes:
//				expectedType = table.ColumnDefinitions[i].Type == ColumnDefinition_BYTES
//			case *Column_Bool:
//				expectedType = table.ColumnDefinitions[i].Type == ColumnDefinition_BOOL
//			default:
//				expectedType = false
//			}
//			if !expectedType {
//				return keys, fmt.Errorf("The type for table '%s', column '%s' is '%s', but the column in the row does not match.",
//					table.Name, table.ColumnDefinitions[i].Name, table.ColumnDefinitions[i].Type)
//			}
//
//			if table.ColumnDefinitions[i].Key {
//				keys = append(keys, *column)
//			}
//
//		}
//
//		return keys, nil
//	}
//
//	func (stub *ChaincodeStub) isRowPrsent(tableName string, key []Column) (bool, error) {
//		keyString, err := buildKeyString(tableName, key)
//		if err != nil {
//			return false, err
//		}
//		rowBytes, err := stub.GetState(keyString)
//		if err != nil {
//			return false, fmt.Errorf("Error fetching row for key %s: %s", keyString, err)
//		}
//		if rowBytes != nil {
//			return true, nil
//		}
//		return false, nil
//	}
//
//	// insertRowInternal inserts a new row into the specified table.
//	// Returns -
//	// true and no error if the row is successfully inserted.
//	// false and no error if a row already exists for the given key.
//	// flase and a TableNotFoundError if the specified table name does not exist.
//	// false and an error if there is an unexpected error condition.
//	func (stub *ChaincodeStub) insertRowInternal(tableName string, row Row, update bool) (bool, error) {
//
//		table, err := stub.getTable(tableName)
//		if err != nil {
//			return false, err
//		}
//
//		key, err := getKeyAndVerifyRow(*table, row)
//		if err != nil {
//			return false, err
//		}
//
//		present, err := stub.isRowPrsent(tableName, key)
//		if err != nil {
//			return false, err
//		}
//		if (present && !update) || (!present && update) {
//			return false, nil
//		}
//
//		rowBytes, err := proto.Marshal(&row)
//		if err != nil {
//			return false, fmt.Errorf("Error marshalling row: %s", err)
//		}
//
//		keyString, err := buildKeyString(tableName, key)
//		if err != nil {
//			return false, err
//		}
//		err = stub.PutState(keyString, rowBytes)
//		if err != nil {
//			return false, fmt.Errorf("Error inserting row in table %s: %s", tableName, err)
//		}
//
//		return true, nil
	
	
}
