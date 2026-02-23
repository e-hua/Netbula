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

## How to use the control program to get info from your manager instance

### Initialize the ctl program:

`bin/netbula control \
--manager-address <manager_ip_address>:<port_number_for_manager_api> \
--token <tls_token>`

### Start a task

`bin/netbula control run \
--filename <fileName>(Example: demo/startTask.json)`

### Delete a task

`bin/netbula control stop <taskUuid>(Example: 21b23589-5d2d-4731-b5c9-a97e9832d021)`

### Get all tasks

`bin/netbula tasks`

### Get all nodes

`bin/netbula nodes`
