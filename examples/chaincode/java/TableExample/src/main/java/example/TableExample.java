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
package example;

import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;
import org.hyperledger.java.shim.ChaincodeBase;
import org.hyperledger.java.shim.ChaincodeStub;
import org.hyperledger.protos.TableProto;

import java.util.ArrayList;
import java.util.List;


public class TableExample extends ChaincodeBase {
    private static String tableName = "MyNewTable1";
    private static Log log = LogFactory.getLog(TableExample.class);
    @java.lang.Override
    public String run(ChaincodeStub stub, String function, String[] args) {
        log.info("In run, function:"+function);
        switch (function) {

            case "init":
                init(stub, function, args);
                break;
            case "insert":
                insertRow(stub, args, false);
                break;
            case "update":
                insertRow(stub, args, true);
                break;
            case "delete":
                delete(stub, args);
                break;
            default:
                log.error("No matching case for function:"+function);

        }
        return null;
    }

    private void insertRow(ChaincodeStub stub, String[] args, boolean update) {

        int fieldID = 0;

        try {
            fieldID = Integer.parseInt(args[0]);
        }catch (NumberFormatException e){
            log.error("Illegal field id -" + e.getMessage());
            return;
        }

        TableProto.Column col1 =
                TableProto.Column.newBuilder()
                        .setUint32(fieldID).build();
        TableProto.Column col2 =
                TableProto.Column.newBuilder()
                        .setString(args[1]).build();
        List<TableProto.Column> cols = new ArrayList<TableProto.Column>();
        cols.add(col1);
        cols.add(col2);

        TableProto.Row row = TableProto.Row.newBuilder()
                .addAllColumns(cols)
                .build();
        try {

            boolean success = false;
            if(update){
                success = stub.replaceRow(tableName,row);
            }else
            {
                success = stub.insertRow(tableName, row);
            }
            if (success){
                log.info("Row successfully inserted");
            }
        } catch (Exception e) {
            e.printStackTrace();
        }
    }


    public String init(ChaincodeStub stub, String function, String[] args) {
        List<TableProto.ColumnDefinition> cols = new ArrayList<TableProto.ColumnDefinition>();

        cols.add(TableProto.ColumnDefinition.newBuilder()
                .setName("ID")
                .setKey(true)
                .setType(TableProto.ColumnDefinition.Type.UINT32)
                .build()
        );

        cols.add(TableProto.ColumnDefinition.newBuilder()
                .setName("Name")
                .setKey(false)
                .setType(TableProto.ColumnDefinition.Type.STRING)
                .build()
        );


        try {
            try {
                stub.deleteTable(tableName);
            } catch (Exception e) {
                e.printStackTrace();
            }
            stub.createTable(tableName,cols);
        } catch (Exception e) {
            e.printStackTrace();
        }
        return null;
    }

    private boolean delete(ChaincodeStub stub, String[] args){
        int fieldID = 0;

        try {
            fieldID = Integer.parseInt(args[0]);
        }catch (NumberFormatException e){
            log.error("Illegal field id -" + e.getMessage());
            return false;
        }


        TableProto.Column queryCol =
                TableProto.Column.newBuilder()
                        .setUint32(fieldID).build();
        List<TableProto.Column> key = new ArrayList<>();
        key.add(queryCol);
        return stub.deleteRow(tableName, key);
    }

    @java.lang.Override
    public String query(ChaincodeStub stub, String function, String[] args) {
        log.info("query");
        int fieldID = 0;

        try {
            fieldID = Integer.parseInt(args[0]);
        }catch (NumberFormatException e){
            log.error("Illegal field id -" + e.getMessage());
            return "ERROR querying ";
        }
        TableProto.Column queryCol =
                TableProto.Column.newBuilder()
                        .setUint32(fieldID).build();
        List<TableProto.Column> key = new ArrayList<>();
        key.add(queryCol);
        switch (function){
            case "get": {
                try {
                    TableProto.Row tableRow = stub.getRow(tableName,key);
                    if (tableRow.getSerializedSize() > 0) {
                        return tableRow.getColumns(1).getString();
                    }else
                    {
                        return "No record found !";
                    }
                } catch (Exception invalidProtocolBufferException) {
                    invalidProtocolBufferException.printStackTrace();
                }
            }
            default:
                log.error("No matching case for function:"+function);
                return "";
        }

    }

    @java.lang.Override
    public String getChaincodeID() {
        return "TableExample";
    }

    public static void main(String[] args) throws Exception {
        log.info("starting");
        new TableExample().start(args);
    }

}
