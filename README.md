# Netbula

## Yet another container orchestrator

## Where to start

- Build: `go build -o bin/manager ./cmd/manager & go build -o bin/worker ./cmd/worker`

- Run the manager program: `./bin/manager <port_number>`
- Run the worker program: `./bin/worker <manager_ip_address>:<manager_port_number> <tls_token> <worker_name>`
