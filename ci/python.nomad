// ToDo: Delete when creating job on test startup #26.
job "python" {
  datacenters = ["dc1"]
  type = "batch"

  group "python" {
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

    task "python" {
      driver = "docker"
      kill_timeout = "0s"
      kill_signal = "SIGKILL"

      config {
        image = "openhpi/co_execenv_python:3.8"
        command = "sleep"
        args = ["infinity"]
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
