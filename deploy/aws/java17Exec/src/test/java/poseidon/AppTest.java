package poseidon;

import com.amazonaws.services.lambda.runtime.events.APIGatewayProxyResponseEvent;
import com.amazonaws.services.lambda.runtime.events.APIGatewayV2WebSocketEvent;
import org.junit.Test;

import java.io.ByteArrayOutputStream;
import java.io.PrintStream;
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

  static final String MultilineMathContent = Base64.getEncoder().encodeToString(
          ("package org.example;\n" +
          "\n" +
          "public class RecursiveMath {\n" +
          "\n" +
          "    public static void main(String[] args) {\n" +
          "        System.out.println(\"Mein Text\");\n" +
          "        System.out.println(\"Mein Text\");\n" +
          "    }\n" +
          "}").getBytes(StandardCharsets.UTF_8));

  static final String MathContentWithoutTrailingNewline = Base64.getEncoder().encodeToString(
          ("package org.example;\n" +
          "\n" +
          "public class RecursiveMath {\n" +
          "\n" +
          "    public static void main(String[] args) {\n" +
          "        System.out.print(\"Mein Text\");\n" +
          "    }\n" +
          "}").getBytes(StandardCharsets.UTF_8));

  @Test
  public void successfulResponse() {
    APIGatewayProxyResponseEvent result = getApiGatewayProxyResponseRecursiveMath(RecursiveMathContent);
    assertEquals(200, result.getStatusCode().intValue());
  }

  // Scanner.nextLine() consumes the new line. Therefore, we need to add it again.
  @Test
  public void successfulMultilineResponse() {
    ByteArrayOutputStream out = setupStdOutLogs();
    APIGatewayProxyResponseEvent result = getApiGatewayProxyResponseRecursiveMath(MultilineMathContent);
    restoreStdOutLogs();

    assertEquals(200, result.getStatusCode().intValue());
    String expectedOutput =
            "{\"type\":\"stdout\",\"data\":\"Mein Text\\n\"}\n" +
            "{\"type\":\"stdout\",\"data\":\"Mein Text\\n\"}\n" +
            "{\"type\":\"exit\",\"data\":0}\n";
    assertEquals(expectedOutput, out.toString());
  }

  @Test
  public void outputWithoutTrailingNewline() {
    ByteArrayOutputStream out = setupStdOutLogs();
    APIGatewayProxyResponseEvent result = getApiGatewayProxyResponseRecursiveMath(MathContentWithoutTrailingNewline);
    restoreStdOutLogs();

    assertEquals(200, result.getStatusCode().intValue());
    String expectedOutput =
            "{\"type\":\"stdout\",\"data\":\"Mein Text\"}\n" +
            "{\"type\":\"exit\",\"data\":0}\n";
    assertEquals(expectedOutput, out.toString());
  }

  @Test
  public void makefileJustReplacesShellCommand() {
    ByteArrayOutputStream out = setupStdOutLogs();
    APIGatewayProxyResponseEvent result = getApiGatewayProxyResponse("{\"action\":\"java11Exec\"," +
            "\"cmd\":[\"env\", \"TEST_VAR=42\", \"sh\",\"-c\",\"make run\"]," +
            "\"files\":{\"Makefile\":\"" + Base64.getEncoder().encodeToString(("run:\n\t@echo $TEST_VAR\n").getBytes(StandardCharsets.UTF_8)) + "\"}}");
    restoreStdOutLogs();

    assertEquals(200, result.getStatusCode().intValue());
    String expectedOutput =
            "{\"type\":\"stdout\",\"data\":\"42\\n\"}\n" +
                    "{\"type\":\"exit\",\"data\":0}\n";
    assertEquals(expectedOutput, out.toString());
  }


  private PrintStream originalOut;
  private ByteArrayOutputStream setupStdOutLogs() {
    originalOut = System.out;
    ByteArrayOutputStream out = new ByteArrayOutputStream();
    System.setOut(new PrintStream(out));
    return out;
  }

  private void restoreStdOutLogs() {
    System.setOut(originalOut);
  }

  private APIGatewayProxyResponseEvent getApiGatewayProxyResponse(String body) {
    App app = new App();
    APIGatewayV2WebSocketEvent input = new APIGatewayV2WebSocketEvent();
    APIGatewayV2WebSocketEvent.RequestContext ctx = new APIGatewayV2WebSocketEvent.RequestContext();
    ctx.setDomainName("abcdef1234.execute-api.eu-central-1.amazonaws.com");
    ctx.setConnectionId("myUUID");
    input.setRequestContext(ctx);
    Map<String, String> headers = new HashMap<>();
    headers.put(App.disableOutputHeaderKey, "True");
    input.setHeaders(headers);
    input.setBody(body);
    return app.handleRequest(input, null);
  }

  private APIGatewayProxyResponseEvent getApiGatewayProxyResponseRecursiveMath(String content) {
    return getApiGatewayProxyResponse("{\"action\":\"java11Exec\",\"cmd\":[\"sh\",\"-c\",\"javac org/example/RecursiveMath.java && java org/example/RecursiveMath\"]," +
            "\"files\":{\"org/example/RecursiveMath.java\":\"" + content + "\"}}");
  }
}
