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

  static final String SuccessfulMakefile = Base64.getEncoder().encodeToString(
          ("run:\n" +
                  "\tjavac org/example/RecursiveMath.java\n" +
                  "\tjava org/example/RecursiveMath\n" +
                  "\n" +
          "test:\n" +
                  "\techo Hi\n"
          ).getBytes(StandardCharsets.UTF_8));

  static final String NotSupportedMakefile = Base64.getEncoder().encodeToString(
          ("run: test\n" +
                  "\tjavac org/example/RecursiveMath.java\n" +
                  "\tjava org/example/RecursiveMath\n" +
                  "\n" +
          "test:\n" +
                  "\techo Hi\n"
          ).getBytes(StandardCharsets.UTF_8));


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

  @Test
  public void sucessfullMake() {
    Map<String, String> files = new HashMap<>();
    files.put("Makefile", SuccessfulMakefile);
    files.put("org/example/RecursiveMath.java", RecursiveMathContent);

    App app = new App();
    String replacedCommand = app.simpleMakefileReplacement("make run", files);

    assertEquals("javac org/example/RecursiveMath.java && java org/example/RecursiveMath", replacedCommand);
  }

  @Test
  public void withoutMake() {
    Map<String, String> files = new HashMap<>();
    files.put("Makefile", SuccessfulMakefile);
    files.put("org/example/RecursiveMath.java", RecursiveMathContent);

    App app = new App();
    String command = "javac org/example/RecursiveMath.java";
    String replacedCommand = app.simpleMakefileReplacement(command, files);

    assertEquals(command, replacedCommand);
  }

  @Test
  public void withNotSupportedMakefile() {
    Map<String, String> files = new HashMap<>();
    files.put("Makefile", NotSupportedMakefile);
    files.put("org/example/RecursiveMath.java", RecursiveMathContent);

    App app = new App();
    String command = "make run";
    String replacedCommand = app.simpleMakefileReplacement(command, files);

    assertEquals(command, replacedCommand);
  }
}
