# Full configuration options can be found at https://www.nomadproject.io/docs/configuration

data_dir = "/opt/nomad/data"
bind_addr = "0.0.0.0"

limits {
    http_max_conns_per_client = 0
}

# Require TLS
tls {
  http = true
  rpc  = true

  ca_file   = "/home/ubuntu/nomad-ca.pem"
  cert_file = "/home/ubuntu/cert.pem"
  key_file  = "/home/ubuntu/cert-key.pem"

  verify_server_hostname = true
  verify_https_client    = true
}

# telemetry {
#     collection_interval = "10s"
#     prometheus_metrics = true
#     publish_allocation_metrics = true
#     publish_node_metrics = true
# }
