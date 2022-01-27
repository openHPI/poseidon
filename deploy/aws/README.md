# Poseidon AWS Executors

This project contains source code and supporting files for a serverless application that you can deploy with the SAM CLI. It includes the following functions.

- java11ExecFunction - Code for the application's Lambda function. It can execute Java files with JDK 11.
- events - Invocation events that you can use to invoke the function.
- template.yaml - A template that defines the application's AWS resources.

The application uses several AWS resources, including Lambda functions and an API Gateway API. These resources are defined in the `template.yaml` file in this project. You can update the template to add AWS resources through the same deployment process that updates your application code.

See the [AWS SAM developer guide](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/what-is-sam.html) for deployment, usage and an introduction to SAM specification, the SAM CLI, and serverless application concepts.

## Interface

You can establish a WebSocket connection to the WebSocketURI generated by the deployment. With this connection you can send requests to the lambda functions following this interface:

```
action:
  description: The name of the requested function.
  type: string
cmd:
  description: The command that should be executed.
  type: []string
files:
  description: The files that will be copied before the execution.
  type: map[string]string
```

So for example:
```
{
  "action": "java11Exec",
  "cmd": [
    "sh",
    "-c",
    "javac org/example/RecursiveMath.java && java org/example/RecursiveMath"
  ],
  "files": {
    "org/example/RecursiveMath.java":"cGFja2FnZSBvcmcuZXhhbXBsZTsKCnB1YmxpYyBjbGFzcyBSZWN1cnNpdmVNYXRoIHsKCiAgICBwdWJsaWMgc3RhdGljIHZvaWQgbWFpbihTdHJpbmdbXSBhcmdzKSB7CiAgICAgICAgU3lzdGVtLm91dC5wcmludGxuKCJNZWluIFRleHQiKTsKICAgIH0KCiAgICBwdWJsaWMgc3RhdGljIGRvdWJsZSBwb3dlcihpbnQgYmFzZSwgaW50IGV4cG9uZW50KSB7CiAgICAgICAgcmV0dXJuIDQyOwogICAgfQp9Cgo="
  }
}
```

The messages sent by the function use the [WebSocket Schema](../../api/websocket.schema.json).