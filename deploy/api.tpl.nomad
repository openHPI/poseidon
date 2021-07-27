job "${NOMAD_SLUG}" {
  datacenters = ["dc1"]
  namespace = "${NOMAD_NAMESPACE}"

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

    // Don't allow rescheduling to fail deployment and pipeline if task fails
    reschedule {
      attempts = 0
      unlimited = false
    }

    // No restarts to immediately fail the deployment and pipeline on first task fail
    restart {
      attempts = 0
    }

    network {
      mode = "bridge"

      port "http" {
        to     = 7200
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

      template {
          source        = "${NOMAD_CACERT}"
          destination   = "/home/api/nomad-ca.crt"
          change_mode   = "noop"
      }

      env {
        POSEIDON_SERVER_ADDRESS = "${POSEIDON_LISTEN_ADDRESS}"
        POSEIDON_NOMAD_ADDRESS =  "${NOMAD_SERVER_HOST}"
        POSEIDON_NOMAD_NAMESPACE = "${NOMAD_NAMESPACE}"
        POSEIDON_NOMAD_TLS_ACTIVE = "${NOMAD_TLS_ACTIVE}"
        POSEIDON_NOMAD_TLS_CAFILE = "nomad-ca.crt"
      }

      resources {
        memory = "100" // 100 MB RAM
        cpu    = "100" // 100 MHz
      }
    }
  }
}
