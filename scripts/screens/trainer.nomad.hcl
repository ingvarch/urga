# Asks for a client the cluster does not have, so it waits to be placed.
job "trainer" {
  namespace = "staging"

  constraint {
    attribute = "${attr.kernel.name}"
    value     = "plan9"
  }

  group "trainer" {
    task "trainer" {
      driver = "raw_exec"

      config {
        command = "/bin/sh"
        args    = ["-c", "sleep 3600"]
      }
    }
  }
}
