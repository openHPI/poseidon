# Simple image containing the Nomad binary to deploy Nomad jobs

FROM debian:buster-slim

# Install prerequisites, gettext contains envsubst used in the CI
RUN apt-get update && \
    apt install -y unzip wget gettext && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists

# Download Nomad
RUN wget "https://releases.hashicorp.com/nomad/1.0.4/nomad_1.0.4_linux_amd64.zip" && \
    wget "https://releases.hashicorp.com/nomad/1.0.4/nomad_1.0.4_SHA256SUMS" && \
    grep "nomad_1.0.4_linux_amd64.zip" nomad_1.0.4_SHA256SUMS | sha256sum -c - && \
    unzip nomad_1.0.4_linux_amd64.zip

# Install Nomad
RUN mv nomad /usr/sbin/ && nomad -version

COPY nomad-run-and-wait /usr/sbin/
RUN chmod +x /usr/sbin/nomad-run-and-wait
