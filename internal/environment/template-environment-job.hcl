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
    spread {
      // see https://www.nomadproject.io/docs/job-specification/spread#even-spread-across-data-center
      // This spreads the load evenly amongst our nodes
      attribute = "${node.unique.name}"
      weight = 100
    }
    restart {
      attempts = 3
      delay = "15s"
      interval = "1h"
      mode = "fail"
    }
    reschedule {
      unlimited = false
      attempts = 3
      interval = "6h"
      delay = "1m"
      max_delay = "4m"
      delay_function = "exponential"
    }

    task "default-task" {
      driver = "docker"
      kill_timeout = "0s"
      kill_signal = "SIGKILL"

      config {
        image = "openhpi/docker_exec_phusion"
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
        memory = 50
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
