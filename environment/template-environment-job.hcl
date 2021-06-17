// This is the default job configuration that is used when no path to another default configuration is given

job "template-0" {
  datacenters = ["dc1"]
  type = "batch"

  group "default-group" {
    ephemeral_disk {
      migrate = false
      size    = 10
      sticky  = false
    }
    count = 1
    scaling {
      enabled = true
      min = 0
      max = 300
    }
    spread {
      // see https://www.nomadproject.io/docs/job-specification/spread#even-spread-across-data-center
      // This spreads the load evenly amongst our nodes
      attribute = "${node.unique.name}"
      weight = 100
    }

    task "default-task" {
      driver = "docker"
      kill_timeout = "0s"
      kill_signal = "SIGKILL"

      config {
        image = "drp.codemoon.xopic.de/openhpi/co_execenv_python:3.8"
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

  group "config" {
    // We want to store whether a task is in use in order to recover from a downtime.
    // Without a separate config task, marking a task as used would result in a restart of that task,
    // as the meta information is passed to the container as environment variables.
    count = 0
    task "config" {
      driver = "exec"
      config {
        command = "whoami"
      }
      logs {
        max_files     = 1
        max_file_size = 1
      }
      resources {
        // minimum values
        cpu    = 1
        memory = 10
      }
    }
    meta {
      environment = "0"
      used = "false"
      prewarmingPoolSize = "1"
    }
  }
}