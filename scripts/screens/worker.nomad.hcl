job "worker" {
  namespace = "staging"

  group "worker" {
    task "worker" {
      driver = "raw_exec"

      config {
        command = "/bin/sh"
        args    = ["-c", "while :; do sleep 1; done"]
      }

      resources {
        cpu    = 200
        memory = 128
      }
    }
  }
}
