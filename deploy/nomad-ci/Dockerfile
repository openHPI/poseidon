# Simple image containing the Nomad binary to deploy Nomad jobs

FROM golang:latest

# Install prerequisites, gettext contains envsubst used in the CI
RUN apt-get update && \
    apt install -y \
        unzip \
        wget \
        gettext \
        apt-transport-https \
        ca-certificates \
        curl \
        gnupg \
        lsb-release && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists

RUN curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
RUN echo \
      "deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/debian \
      $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
RUN apt-get update && \
    apt-get install -y docker-ce docker-ce-cli containerd.io && \
    rm -rf /var/lib/apt/lists

# Download Nomad
RUN wget "https://releases.hashicorp.com/nomad/1.1.1/nomad_1.1.1_linux_amd64.zip" && \
    wget "https://releases.hashicorp.com/nomad/1.1.1/nomad_1.1.1_SHA256SUMS" && \
    grep "nomad_1.1.1_linux_amd64.zip" nomad_1.1.1_SHA256SUMS | sha256sum -c - && \
    unzip nomad_1.1.1_linux_amd64.zip

# Install Nomad
RUN mv nomad /usr/sbin/ && nomad -version

COPY nomad-run-and-wait /usr/sbin/
RUN chmod +x /usr/sbin/nomad-run-and-wait
