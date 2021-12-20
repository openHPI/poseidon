client {
    enabled = true
    servers = [
        "server domain 1",
        "server domain 2"
    ]
    cni_path = "/usr/lib/cni"
}

plugin "docker" {
    config {
        gc {
            image_delay = "0s"
        }

        # auth {
            # config = "/root/.docker/config.json"
        # }
    }
}
