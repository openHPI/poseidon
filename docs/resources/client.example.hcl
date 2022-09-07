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
        allow_runtimes = ["runsc"]
        gc {
            image_delay = "0s"
        }
    }
}
