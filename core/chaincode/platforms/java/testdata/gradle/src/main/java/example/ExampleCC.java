package example;

import io.netty.handler.ssl.OpenSsl;
import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;
import org.hyperledger.fabric.shim.ChaincodeBase;
import org.hyperledger.fabric.shim.ChaincodeStub;

public class ExampleCC extends ChaincodeBase {

    private static Log _logger = LogFactory.getLog(ExampleCC.class);

    @Override
    public Response init(ChaincodeStub stub) {
        _logger.info("Init java simple chaincode");
        return newSuccessResponse();
    }

    @Override
    public Response invoke(ChaincodeStub stub) {
        _logger.info("Invoke java simple chaincode");
        return newSuccessResponse();
    }
}