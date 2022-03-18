server {
    enabled = true
    bootstrap_expect = 2
    server_join {
        retry_join = ["<<other servers domain>>"]
        retry_max = 3
        retry_interval = "15s"
    }

    # https://www.nomadproject.io/docs/configuration/server
    default_scheduler_config {
        scheduler_algorithm = "spread"
        memory_oversubscription_enabled = true
    }
}
