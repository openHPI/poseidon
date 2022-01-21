package poseidon;

import java.io.*;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.Base64;
import java.util.Map;

import com.amazonaws.client.builder.AwsClientBuilder;
import com.amazonaws.regions.Regions;
import com.amazonaws.services.apigatewaymanagementapi.AmazonApiGatewayManagementApi;
import com.amazonaws.services.apigatewaymanagementapi.AmazonApiGatewayManagementApiClientBuilder;
import com.amazonaws.services.apigatewaymanagementapi.model.PostToConnectionRequest;
import com.amazonaws.services.lambda.runtime.Context;
import com.amazonaws.services.lambda.runtime.RequestHandler;
import com.amazonaws.services.lambda.runtime.events.APIGatewayProxyResponseEvent;
import com.amazonaws.services.lambda.runtime.events.APIGatewayV2WebSocketEvent;
import com.google.gson.Gson;

// AwsFunctionRequest contains the java files that needs to be executed.
class AwsFunctionRequest {
    String[] cmd;
    Map<String, String> files;
}

// WebSocketMessageType are the types of messages that are being send back over the WebSocket connection.
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

// WebSocketMessage is the object that is beeing send over the WebSocket connection.
class WebSocketMessage {
    WebSocketMessageType type;
    String data;
    int exitCode;
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

    public APIGatewayProxyResponseEvent handleRequest(final APIGatewayV2WebSocketEvent input, final Context context) {
        APIGatewayV2WebSocketEvent.RequestContext ctx = input.getRequestContext();
        this.gwClient = AmazonApiGatewayManagementApiClientBuilder.standard()
                .withEndpointConfiguration(new AwsClientBuilder.EndpointConfiguration("https://" + ctx.getDomainName() + "/" + ctx.getStage(), Regions.EU_CENTRAL_1.getName()))
                .build();
        this.connectionID = ctx.getConnectionId();
        this.disableOutput = input.getHeaders() != null && input.getHeaders().containsKey(disableOutputHeaderKey) && Boolean.parseBoolean(input.getHeaders().get(disableOutputHeaderKey));
        AwsFunctionRequest execution = gson.fromJson(input.getBody(), AwsFunctionRequest.class);

        try {
            File workingDirectory = this.writeFS(execution.files);

            ProcessBuilder pb = new ProcessBuilder(execution.cmd).redirectErrorStream(true);
            pb.directory(workingDirectory);
            Process p = pb.start();
            InputStream stdout = p.getInputStream(), stderr = p.getErrorStream();
            this.forwardOutput(p, stdout, stderr);
            p.destroy();
            return new APIGatewayProxyResponseEvent().withStatusCode(200);
        } catch (Exception e) {
            this.sendMessage(e.toString().getBytes(StandardCharsets.UTF_8), WebSocketMessageType.WebSocketOutputError, 0);
            return new APIGatewayProxyResponseEvent().withBody(e.toString()).withStatusCode(500);
        }
    }

    // writeFS writes the files to the local filesystem.
    private File writeFS(Map<String, String> files) throws IOException {
        File workspace = Files.createTempDirectory("workspace").toFile();
        for (Map.Entry<String, String> entry : files.entrySet()) {
            File f = new File(workspace, entry.getKey());

            boolean ok = f.getParentFile().mkdirs();
            if (!ok) throw new IOException("Cannot create parent directories.");

            ok = f.createNewFile();
            if (!ok) throw new IOException("Cannot create file.");

            Files.write(f.toPath(), Base64.getDecoder().decode(entry.getValue()));
        }
        return workspace;
    }

    // forwardOutput sends the output of the process to the WebSocket connection.
    private void forwardOutput(Process p, InputStream stdout, InputStream stderr) throws InterruptedException, IOException {
        while (p.isAlive() || stdout.available()>0 || stderr.available()>0) {
            while(stdout.available()>0) this.sendMessage(stdout.readAllBytes(), WebSocketMessageType.WebSocketOutputStdout, 0);
            while(stderr.available()>0) this.sendMessage(stderr.readAllBytes(), WebSocketMessageType.WebSocketOutputStderr, 0);
            Thread.sleep(50);
        }
        this.sendMessage(new byte[0], WebSocketMessageType.WebSocketExit, p.exitValue());
    }

    // sendMessage sends WebSocketMessage objects back to the requester of this Lambda function.
    private void sendMessage(byte[] data, WebSocketMessageType type, int exitCode) {
        if (this.disableOutput) return;
        WebSocketMessage msg = new WebSocketMessage();
        msg.data = new String(data);
        msg.type = type;
        msg.exitCode = exitCode;

        this.gwClient.postToConnection(new PostToConnectionRequest()
                .withConnectionId(this.connectionID)
                .withData(ByteBuffer.wrap(gson.toJson(msg).getBytes(StandardCharsets.UTF_8))));
    }
}
