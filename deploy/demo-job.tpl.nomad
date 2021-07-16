// This job is used by the e2e tests as a demo job.

job "template-0" {
  datacenters = ["dc1"]
  type = "batch"
  namespace = "${NOMAD_NAMESPACE}"

  group "default-group" {
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

    task "default-task" {
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
  group "config" {
      // We want to store whether a task is in use in order to recover from a downtime.
      // Without a separate config task, marking a task as used would result in a restart of that task,
      // as the meta information is passed to the container as environment variables.
      count = 0
      task "config" {
        driver = "exec"
        config {
          command = "true"
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
        used = "false"
        prewarmingPoolSize = "0"
      }
    }
}
