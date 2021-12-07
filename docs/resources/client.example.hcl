client {
    enabled = true
    servers = [
        "server domain 1",
        "server domain 2"
    ]
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
