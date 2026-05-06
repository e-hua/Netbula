[![codecov](https://codecov.io/gh/e-hua/Netbula/graph/badge.svg?token=CGUPT6W1GA)](https://codecov.io/gh/e-hua/Netbula)

# Netbula

## Yet another container orchestrator

### Architecture

- `docs/adr`: ARDs (Architectural Decision Record), tracking all important architectural decision made when developing Netbula.

## Developer Guide

### Use `Makefile` for local development

- Build: Run command `make` in the root directory of the project

- Check overall test coverage rate of the entire project: `make total`

- Check the coverage report in HTML view: `make report`

- Remove all the binaries and mock types generated: `make clean`

### Use `act` to test GitHub workflow (on Apple Silicon Macs)

#### 1. Install `act` using homebrew

```bash
brew install act
```

#### 2. Run `act` to test workflow locally

```bash
act --container-architecture linux/arm64 --secret-file .secrets
```

## User Guide

### Initialize the manager and worker

- Run the manager program for the first time:
  `bin/netbula manager \ 
--worker-port <port_number_for_worker_connection> \
--api-port <port_number_for_manager_api>`

- Run the worker program:
  `bin/netbula worker \
--manager <manager_ip_address>:<port_number_for_worker_connection> \
--token <tls_token> \
--name <worker_name>`

### How to use the control program to get info from your manager instance

#### Initialize the ctl program:

`bin/netbula control \
--manager-address <manager_ip_address>:<port_number_for_manager_api> \
--token <tls_token>`

#### Start a task

`bin/netbula control run \
--filename <fileName>(Example: demo/startTask.json)`

#### Delete a task

`bin/netbula control stop <taskUuid>(Example: 21b23589-5d2d-4731-b5c9-a97e9832d021)`

#### Get all tasks

`bin/netbula control tasks`

#### Get all nodes

`bin/netbula control nodes`

### Using Systemd to keep the manager / worker alive

#### 1. Setup config file

`sudo vim /etc/systemd/system/netbula.service`

#### 2. Paste the Configuration

```
[Unit]
Description=Netbula Manager(Worker) Service
After=network.target

[Service]
# Should avoid <userName> as <user>
User=<userName>
Group=<userName>

# The directory where your app lives
WorkingDirectory=<Path_to_Directory_holding_project>

# The command you want to run (using absolute paths)
ExecStart=/home/<userName>/Netbula/bin/netbula manager --worker-port 1111 --api-port 2222

# Restart the app automatically if it crashes
Restart=always
RestartSec=5

# Keep track of logs
StandardOutput=append:/home/<userName>/Netbula/manager.log
StandardError=append:/home/<userName>/Netbula/manager.log

[Install]
WantedBy=multi-user.target
```

#### 3. Start the service with Systemd

`sudo systemctl start netbula`

#### 4. Enable it to start on boot

`sudo systemctl enable netbula`

#### 5. Check the status

`sudo systemctl status netbula`

#### 6. Watch the logs

`tail -f /home/cgh/Netbula/manager.log`
