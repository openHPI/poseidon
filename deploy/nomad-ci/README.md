# Nomad-in-Docker Image

The [`Dockerfile`](Dockerfile) in this folder creates a Docker image that contains Docker and Nomad.

Running the image requires the following Docker options:

- Allow Nomad to use mount: `--cap-add=SYS_ADMIN`
- Allow Nomad to use bind mounts: `--security-opt apparmor=unconfined`
- Add access to Docker daemon: `-v /var/run/docker.sock:/var/run/docker.sock`
- Map port to host: `-p 4646:4646`

A complete command to run the container is as follows.

```shell
docker run --rm --name nomad \
  --cap-add=SYS_ADMIN \
  --security-opt apparmor=unconfined \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -p 4646:4646 \
  nomad-ci
```
