# Netbula

## Yet another container orchestrator

## Where to start

- Build: `go build -o bin/manager ./cmd/manager & go build -o bin/worker ./cmd/worker`

- Run the manager program: `./bin/manager <port_number_for_worker_connection> <port_number_for_manager_api>`
- Run the worker program: `./bin/worker <manager_ip_address>:<port_number_for_worker_connection> <tls_token> <worker_name>`

## How to test manually

### Start a task

- `curl -v --request POST \
header 'Content-Type: application/json' \
--data @demo/startTask.json \
localhost:<port_number_for_manager_api>/tasks`

### Delete a task

- `curl -v --request DELETE \
'localhost:<port_number_for_manager_api>/tasks/21b23589-5d2d-4731-b5c9-a97e9832d021'`

### Get all tasks

- `curl -v localhost:9999/tasks|jq`
