job "report" {
  type = "batch"

  group "report" {
    task "report" {
      driver = "raw_exec"

      config {
        command = "/bin/sh"
        args    = ["-c", "sleep 1"]
      }
    }
  }
}
