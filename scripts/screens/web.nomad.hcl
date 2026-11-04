job "web" {
  namespace = "prod"

  group "web" {
    count = 2

    task "web" {
      driver = "raw_exec"

      # Works for a moment every few seconds, so the chart of CPU moves.
      config {
        command = "/bin/sh"
        args    = ["-c", "while :; do i=0; while [ $i -lt 100000 ]; do i=$((i+1)); done; sleep 4; done"]
      }

      resources {
        cpu    = 500
        memory = 256
      }
    }
  }
}
