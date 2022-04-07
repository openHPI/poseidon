package poseidon;

import com.amazonaws.services.lambda.runtime.events.APIGatewayProxyResponseEvent;
import com.amazonaws.services.lambda.runtime.events.APIGatewayV2WebSocketEvent;
import org.junit.Test;

import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.util.HashMap;
import java.util.Map;

import static org.junit.Assert.assertEquals;


public class AppTest {

  static final String RecursiveMathContent = Base64.getEncoder().encodeToString(
          ("package org.example;\n" +
          "\n" +
          "public class RecursiveMath {\n" +
          "\n" +
          "    public static void main(String[] args) {\n" +
          "        System.out.println(\"Mein Text\");\n" +
          "    }\n" +
          "\n" +
          "    public static double power(int base, int exponent) {\n" +
          "        return 42;\n" +
          "    }\n" +
          "}").getBytes(StandardCharsets.UTF_8));

  @Test
  public void successfulResponse() {
    App app = new App();
    APIGatewayV2WebSocketEvent input = new APIGatewayV2WebSocketEvent();
    APIGatewayV2WebSocketEvent.RequestContext ctx = new APIGatewayV2WebSocketEvent.RequestContext();
    ctx.setDomainName("abcdef1234.execute-api.eu-central-1.amazonaws.com");
    ctx.setConnectionId("myUUID");
    input.setRequestContext(ctx);
    Map<String, String> headers = new HashMap<>();
    headers.put(App.disableOutputHeaderKey, "True");
    input.setHeaders(headers);
    input.setBody("{\"action\":\"java11Exec\",\"cmd\":[\"sh\",\"-c\",\"javac org/example/RecursiveMath.java && java org/example/RecursiveMath\"]," +
            "\"files\":{\"org/example/RecursiveMath.java\":\"" + RecursiveMathContent + "\"}}");
    APIGatewayProxyResponseEvent result = app.handleRequest(input, null);
    assertEquals(200, result.getStatusCode().intValue());
  }
}
