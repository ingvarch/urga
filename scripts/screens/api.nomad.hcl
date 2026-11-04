job "api" {
  namespace = "prod"

  group "api" {
    task "api" {
      driver = "raw_exec"

      config {
        command = "/bin/sh"
        args    = ["-c", "while :; do sleep 1; done"]
      }

      resources {
        cpu    = 300
        memory = 128
      }
    }
  }
}
