# AWS Deployment

The AWS lambda function included in `deploy/aws` is deployed using the AWS SAM CLI within GitHub Actions. The deployment is triggered by a push to the `main` branch.

The following environment variables are required for the deployment:
- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`
- `AWS_REGION`

Optionally, the `AWS_ACCOUNT_ID` can be set to restrict output in GitHub actions log.

For the deployment user specified, the permissions set in [aws-role.json5](./resources/aws-role.json5) are required.
