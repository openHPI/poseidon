FROM openhpi/co_execenv_java:17
MAINTAINER openHPI Team <openhpi-info@hpi.de>
LABEL description='This image provides an AWS Lambda environment with Java 17.0 and JUnit 5.'
LABEL run_command='make run'
LABEL test_command='make test CLASS_NAME="%{class_name}" FILENAME="%{filename}"'

COPY target/dependency/* /function/
COPY target/java17Exec-1.0.jar /function

CMD [ "/usr/bin/java", "--add-opens", "java.base/java.util=ALL-UNNAMED", "-cp", "/function/*", "com.amazonaws.services.lambda.runtime.api.client.AWSLambda", "poseidon.App::handleRequest" ]
