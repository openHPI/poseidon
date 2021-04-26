FROM alpine:latest

RUN adduser --disabled-password api
USER api
COPY poseidon /home/api/

EXPOSE 3000
CMD ["/home/api/poseidon"]
