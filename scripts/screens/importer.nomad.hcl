# Fails once and is not tried again, so the screens show a failed allocation.
job "importer" {
  type = "batch"

  group "importer" {
    restart {
      attempts = 0
      mode     = "fail"
    }

    reschedule {
      attempts  = 0
      unlimited = false
    }

    task "importer" {
      driver = "raw_exec"

      config {
        command = "/bin/sh"
        args    = ["-c", "sleep 2; exit 1"]
      }
    }
  }
}
