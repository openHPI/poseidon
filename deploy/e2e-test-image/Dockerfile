# Minimal working Docker image used in our e2e tests
FROM python:latest

RUN useradd --home-dir /workspace --no-create-home --user-group user && \
    mkdir /workspace && chown user:user /workspace

WORKDIR /workspace

USER user
