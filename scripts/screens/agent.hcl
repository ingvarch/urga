# A cluster of one agent in dev mode, on ports of its own so that it runs
# next to another local cluster.
name = "client-01"

ports {
  http = 24646
  rpc  = 24647
  serf = 24648
}
