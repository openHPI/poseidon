# Poseidon AWS Executors

This project contains source code and supporting files for a serverless application that you can deploy with the SAM CLI. It includes the following functions.

- java11ExecFunction - Code for the application's Lambda function. It can execute Java files.
- events - Invocation events that you can use to invoke the function.
- template.yaml - A template that defines the application's AWS resources.

The application uses several AWS resources, including Lambda functions and an API Gateway API. These resources are defined in the `template.yaml` file in this project. You can update the template to add AWS resources through the same deployment process that updates your application code.

See the [AWS SAM developer guide](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/what-is-sam.html) for deployment, usage and an introduction to SAM specification, the SAM CLI, and serverless application concepts.
