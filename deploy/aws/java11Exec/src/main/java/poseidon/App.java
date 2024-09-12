package poseidon;

import com.amazonaws.client.builder.AwsClientBuilder;
import com.amazonaws.services.apigatewaymanagementapi.AmazonApiGatewayManagementApi;
import com.amazonaws.services.apigatewaymanagementapi.AmazonApiGatewayManagementApiClientBuilder;
import com.amazonaws.services.apigatewaymanagementapi.model.PostToConnectionRequest;
import com.amazonaws.services.lambda.runtime.Context;
import com.amazonaws.services.lambda.runtime.RequestHandler;
import com.amazonaws.services.lambda.runtime.events.APIGatewayProxyResponseEvent;
import com.amazonaws.services.lambda.runtime.events.APIGatewayV2WebSocketEvent;
import com.google.gson.Gson;
import com.google.gson.JsonObject;

import java.io.*;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.Base64;
import java.util.Map;

// AwsFunctionRequest contains the java files that needs to be executed.
class AwsFunctionRequest {
    String[] cmd;
    Map<String, String> files;
}

// WebSocketMessageType are the types of messages that are being sent back over the WebSocket connection.
enum WebSocketMessageType {
    WebSocketOutputStdout("stdout"),
    WebSocketOutputStderr("stderr"),
    WebSocketOutputError("error"),
    WebSocketExit("exit");

    private final String typeName;

    WebSocketMessageType(String name) {
        this.typeName = name;
    }

    public String toString() {
        return typeName;
    }
}

/**
 * Handler for requests to Lambda function.
 * This Lambda function executes the passed command with the provided files in an isolated Java environment.
 */
public class App implements RequestHandler<APIGatewayV2WebSocketEvent, APIGatewayProxyResponseEvent> {

    // gson helps parse the json objects.
    private static final Gson gson = new Gson();

    // gwClient is used to send messages back via the WebSocket connection.
    private AmazonApiGatewayManagementApi gwClient;

    // connectionID helps to identify the WebSocket connection that has called this function.
    private String connectionID;

    public static final String disableOutputHeaderKey = "disableOutput";

    // disableOutput: If set to true, no output will be sent over the WebSocket connection.
    private boolean disableOutput = false;

    // Unwrapps the passed command. We expect a "sh -c" wrapped command.
    public static String unwrapCommand(String[] cmd) {
        return cmd[cmd.length - 1];
    }

    // Replaces the last element of the command with the new shell command.
    public static String[] wrapCommand(String[] originalCommand, String shellCommand) {
        originalCommand[originalCommand.length - 1] = shellCommand;
        return originalCommand;
    }

    public APIGatewayProxyResponseEvent handleRequest(final APIGatewayV2WebSocketEvent input, final Context context) {
        APIGatewayV2WebSocketEvent.RequestContext ctx = input.getRequestContext();
        String[] domains = ctx.getDomainName().split("\\.");
        String region = domains[domains.length-3];
        this.gwClient = AmazonApiGatewayManagementApiClientBuilder.standard()
                .withEndpointConfiguration(new AwsClientBuilder.EndpointConfiguration("https://" + ctx.getDomainName() + "/" + ctx.getStage(), region))
                .build();
        this.connectionID = ctx.getConnectionId();
        this.disableOutput = input.getHeaders() != null && input.getHeaders().containsKey(disableOutputHeaderKey) && Boolean.parseBoolean(input.getHeaders().get(disableOutputHeaderKey));
        AwsFunctionRequest execution = gson.fromJson(input.getBody(), AwsFunctionRequest.class);

        try {
            File workingDirectory = this.writeFS(execution.files);

            String[] cmd = execution.cmd;
            try {
                SimpleMakefile make = new SimpleMakefile(execution.files);
                cmd = wrapCommand(cmd, make.parseCommand(unwrapCommand(execution.cmd)));
            } catch (NoMakefileFoundException | NoMakeCommandException | InvalidMakefileException ignored) {}

            ProcessBuilder pb = new ProcessBuilder(cmd);
            pb.directory(workingDirectory);
            Map<String, String> env = pb.environment();
            env.put("CLASSPATH", ".:/var/task/lib/org.hamcrest.hamcrest-3.0.jar:/var/task/lib/junit.junit-4.13.2.jar:" + env.get("CLASSPATH"));
            Process p = pb.start();
            InputStream stdout = p.getInputStream(), stderr = p.getErrorStream();
            this.forwardOutput(p, stdout, stderr);
            p.destroy();
            return new APIGatewayProxyResponseEvent().withStatusCode(200);
        } catch (Exception e) {
            this.sendMessage(WebSocketMessageType.WebSocketOutputError, e.toString(), null);
            return new APIGatewayProxyResponseEvent().withBody(e.toString()).withStatusCode(500);
        }
    }

    // writeFS writes the files to the local filesystem.
    private File writeFS(Map<String, String> files) throws IOException {
        File workspace = Files.createTempDirectory("workspace").toFile();
        for (Map.Entry<String, String> entry : files.entrySet()) {
            try {
                File f = new File(entry.getKey());

                if (!f.isAbsolute()) {
                    f = new File(workspace, entry.getKey());
                }

                f.getParentFile().mkdirs();
                if (!f.getParentFile().exists()) {
                    throw new IOException("Cannot create parent directories.");
                }

                f.createNewFile();
                if (!f.exists()) {
                    throw new IOException("Cannot create file.");
                }

                Files.write(f.toPath(), Base64.getDecoder().decode(entry.getValue()));
            } catch (IOException e) {
                this.sendMessage(WebSocketMessageType.WebSocketOutputError, e.toString(), null);
            }
        }
        return workspace;
    }

    // forwardOutput sends the output of the process to the WebSocket connection.
    private void forwardOutput(Process p, InputStream stdout, InputStream stderr) throws InterruptedException {
        Thread output = new Thread(() -> scanForOutput(p, stdout, WebSocketMessageType.WebSocketOutputStdout));
        Thread error = new Thread(() -> scanForOutput(p, stderr, WebSocketMessageType.WebSocketOutputStderr));
        output.start();
        error.start();

        output.join();
        error.join();
        this.sendMessage(WebSocketMessageType.WebSocketExit, null, p.waitFor());
    }

    // scanForOutput reads the passed stream and forwards it via the WebSocket connection.
    private void scanForOutput(Process p, InputStream stream, WebSocketMessageType type) {
        BufferedReader br = new BufferedReader(new InputStreamReader(stream));
        StringBuilder s = new StringBuilder();
        int nextByte;

        try {
            while ((nextByte = br.read()) != -1) {
                char c = (char) nextByte;
                s.append(c);

                if (c == '\n') {
                    this.sendMessage(type, s.toString(), null);
                    s = new StringBuilder();
                }
            }
        } catch (IOException ignored) {}

        if (!s.toString().isEmpty()) {
            this.sendMessage(type, s.toString(), null);
        }
    }

    // sendMessage sends WebSocketMessage objects back to the requester of this Lambda function.
    private void sendMessage(WebSocketMessageType type, String data, Integer exitCode) {
        JsonObject msg = new JsonObject();
        msg.addProperty("type", type.toString());
        if (type == WebSocketMessageType.WebSocketExit) {
            msg.addProperty("data", exitCode);
        } else {
            msg.addProperty("data", data);
        }

        if (this.disableOutput) {
            System.out.println(gson.toJson(msg));
        } else {
            this.gwClient.postToConnection(new PostToConnectionRequest()
                    .withConnectionId(this.connectionID)
                    .withData(ByteBuffer.wrap(gson.toJson(msg).getBytes(StandardCharsets.UTF_8))));
        }
    }
}
