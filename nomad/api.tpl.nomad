job "${NOMAD_SLUG}" {
  datacenters = ["dc1"]

  group "api" {
    count = 1

    update {
      // https://learn.hashicorp.com/tutorials/nomad/job-rolling-update
      max_parallel  = 1
      min_healthy_time  = "30s"
      healthy_deadline  = "5m"
      progress_deadline = "10m"
      auto_revert   = true
    }
    restart {
      attempts = 3
      delay    = "15s"
      interval = "30m"
      mode     = "fail"
    }

    network {
      mode = "bridge"

      port "http" {
        to     = 3000
      }
    }

    service {
      # urlprefix- tag allows Fabio to discover this service and proxy traffic correctly
      tags = ["urlprefix-${HOSTNAME}:80/"]
      name = "${NOMAD_SLUG}"
      port = "http"

      // Health check to let Consul know we are alive
      check {
        name = "health-check"
        type = "http"
        port = "http"
        path = "/api/v1/health"
        interval = "10s"
        timeout  = "2s"
        check_restart {
          limit = 3  // auto-restart task when health check fails 3x in a row
        }

      }
    }

    task "api" {
      driver = "docker"

      config {
        image = "${IMAGE_NAME_ENV}"
      }

      env {
        POSEIDON_SERVER_ADDRESS = "${POSEIDON_LISTEN_ADDRESS}"
      }

      resources {
        memory = "100" // 100 MB RAM
        cpu    = "100" // 100 MHz
      }
    }
  }
}
