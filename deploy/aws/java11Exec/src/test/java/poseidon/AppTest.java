package poseidon;

import com.amazonaws.services.lambda.runtime.events.APIGatewayProxyResponseEvent;
import com.amazonaws.services.lambda.runtime.events.APIGatewayV2WebSocketEvent;
import org.junit.Test;

import java.util.HashMap;
import java.util.Map;

import static org.junit.Assert.assertEquals;

public class AppTest {
  @Test
  public void successfulResponse() {
    App app = new App();
    APIGatewayV2WebSocketEvent input = new APIGatewayV2WebSocketEvent();
    APIGatewayV2WebSocketEvent.RequestContext ctx = new APIGatewayV2WebSocketEvent.RequestContext();
    ctx.setDomainName("domain.eu");
    ctx.setConnectionId("myUUID");
    input.setRequestContext(ctx);
    Map<String, String> headers = new HashMap<>();
    headers.put(App.disableOutputHeaderKey, "True");
    input.setHeaders(headers);
    input.setBody("{\n    \"action\": \"java11Exec\",\n    \"cmd\": [\n        \"sh\",\n        \"-c\",\n        \"javac org/example/RecursiveMath.java && java org/example/RecursiveMath\"\n    ],\n    \"files\": {\n        \"org/example/RecursiveMath.java\": \"cGFja2FnZSBvcmcuZXhhbXBsZTsKCnB1YmxpYyBjbGFzcyBSZWN1cnNpdmVNYXRoIHsKCiAgICBwdWJsaWMgc3RhdGljIHZvaWQgbWFpbihTdHJpbmdbXSBhcmdzKSB7CiAgICAgICAgU3lzdGVtLm91dC5wcmludGxuKCJNZWluIFRleHQiKTsKICAgIH0KCiAgICBwdWJsaWMgc3RhdGljIGRvdWJsZSBwb3dlcihpbnQgYmFzZSwgaW50IGV4cG9uZW50KSB7CiAgICAgICAgcmV0dXJuIDQyOwogICAgfQp9Cgo=\"\n    }\n}");
    APIGatewayProxyResponseEvent result = app.handleRequest(input, null);
    assertEquals(200, result.getStatusCode().intValue());
  }
}
