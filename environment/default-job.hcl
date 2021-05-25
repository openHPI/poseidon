// This is the default job configuration that is used when no path to another default configuration is given

job "default-poseidon-job" {
  datacenters = ["dc1"]
  type = "batch"

  group "default-poseidon-group" {
    ephemeral_disk {
      migrate = false
      size    = 10
      sticky  = false
    }
    count = 1
    scaling {
      enabled = true
      max = 300
    }
    spread {
      // see https://www.nomadproject.io/docs/job-specification/spread#even-spread-across-data-center
      // This spreads the load evenly amongst our nodes
      attribute = "${node.unique.name}"
      weight = 100
    }

    task "default-poseidon-task" {
      driver = "docker"
      kill_timeout = "0s"
      kill_signal = "SIGKILL"

      config {
        image = "python:latest"
        command = "sleep"
        args = ["infinity"]
        network_mode = "none"
      }

      logs {
        max_files     = 1
        max_file_size = 1
      }

      resources {
        cpu    = 40
        memory = 40
      }

      restart {
        delay = "0s"
      }
    }
  }
}
