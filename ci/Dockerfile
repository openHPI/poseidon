# Simple image containing the Nomad binary to deploy Nomad jobs

FROM golang:latest

# Install prerequisites, gettext contains envsubst used in the CI
RUN apt-get update && \
    apt install -y unzip wget gettext && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists && \
    apt-get install -y \
        apt-transport-https \
        ca-certificates \
        curl \
        gnupg

RUN apt-get update && apt-get install -y lsb-release && apt-get clean all
RUN curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
RUN echo \
      "deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/debian \
      $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
RUN apt-get update && \
    apt-get install -y docker-ce docker-ce-cli containerd.io

# Download Nomad
RUN wget "https://releases.hashicorp.com/nomad/1.0.4/nomad_1.0.4_linux_amd64.zip" && \
    wget "https://releases.hashicorp.com/nomad/1.0.4/nomad_1.0.4_SHA256SUMS" && \
    grep "nomad_1.0.4_linux_amd64.zip" nomad_1.0.4_SHA256SUMS | sha256sum -c - && \
    unzip nomad_1.0.4_linux_amd64.zip

# Install Nomad
RUN mv nomad /usr/sbin/ && nomad -version

COPY nomad-run-and-wait /usr/sbin/
RUN chmod +x /usr/sbin/nomad-run-and-wait

RUN wget "https://raw.githubusercontent.com/docker-library/docker/f6a0c427f0354dcf5870c430c72c8f1d6b4e6d5e/20.10/docker-entrypoint.sh"
RUN mv ./docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["sh"]
