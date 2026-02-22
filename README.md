# Netbula

## Yet another container orchestrator

## Where to start

- Build: `go build -o bin/netbula main.go`

- Run the manager program for the first time:
  `bin/netbula manager \ 
--worker-port <port_number_for_worker_connection> \
--api-port <port_number_for_manager_api>`

- Run the worker program:
  `bin/netbula worker \
--manager <manager_ip_address>:<port_number_for_worker_connection> \
--token <tls_token> \
--name <worker_name>`

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

- `curl -v localhost:<port_number_for_manager_api>/tasks|jq`
