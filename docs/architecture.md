# Netbula Architecture

---

## 1. System Overview

**Architecture diagram** of Netbula.

```mermaid
flowchart LR
    Operator(["fa:fa-user Operator"])

    subgraph Netbula["Netbula System"]
        Control["netbula control\n(operator's machine)"]
        Manager["netbula manager\n(public IP)"]
        Worker["netbula worker\n 1..N"]
    end

    Docker["Docker Daemon\n(per worker host)"]

    Operator -->|CLI| Control
    Control -->|HTTPS REST endpoints| Manager
    Worker -.->|uses TCP dial out to| Manager
    Manager -.->|sends HTTPS requests to| Worker
    Worker -->|Docker SDK| Docker
```

Workers **dial out** to the manager using TCP, traversing NAT without port forwarding. The manager multiplexes HTTP requests back through these same connections using _yamux_. See [ADR 0001](adr/0001-reverse-tunneling-for-nat-traversal.md).

---

## 2. Manager Component

**Class diagram** of the manager component

```mermaid
classDiagram
%% External Entities
class Control["netbula control"] {
    <<system>>
}
class Worker["netbula worker"] {
    <<system>>
}
class Db["BoltDb"] {
    <<Database>>
}

namespace netbula_manager {
    class API

    class ManagerService <<interface>>
    class Manager

    class State <<interface>>
    class MappingStorage

    class Scheduler <<interface>>
    class EPVM_Scheduler["epvm"]

    class Cluster <<interface>>
    class HttpClientCluster

    class Queue~TaskEvent~
    note for Queue "A queue of pending taskEvents
    (state transitions for tasks)"
}

%% Flow and Interaction Labels
Control --|> API : HTTPS Request

API --> ManagerService : invokes
ManagerService <|.. Manager

Manager --> Scheduler: use
Manager --> Queue : enqueue/dequeue
Manager --> State : manages
Manager --> Cluster : manages

State <|.. MappingStorage
MappingStorage ..> Db : persistent storage

Cluster <|.. HttpClientCluster
HttpClientCluster ..> Worker : HTTPS over Yamux

Scheduler <|.. EPVM_Scheduler
```

---

## 3. Task Lifecycle

The **state machine diagram** modeling the state transition behavior of tasks.

```mermaid
stateDiagram-v2
    [*] --> Pending : operator submits task
    Pending --> Scheduled : scheduler assigns to worker
    Scheduled --> Running : worker starts container
    Scheduled --> Failed : container start error
    Running --> Completed : exit code 0 / stopped by operator
    Running --> Failed : non-zero exit code
    Completed --> [*]
    Failed --> [*]
```

Completed and Failed are terminal states. Tasks are kept in the database and never deleted.

---

## 4. Worker Registration

```mermaid
sequenceDiagram
    participant W as Worker
    participant M as Manager

    W->>W: Load manager address and cert-fingerprint from config

    critical TLS Connection
        W->>M: TCP dial-out to manager:workerPort
        M-->>W: TCP accept
        M->>W: TLS handshake + present self-signed certificate
        opt SHA-256(cert) != stored cert-fingerprint
            Note over W,M: Connection aborted
        end
    end

    par
        W->>W: Wraps connection as yamux Server
    and
        M->>M: Wraps connection as yamux Client
        M->>M: Store client
    end

    critical Registration
        M->>W: GET /info
        W-->>M: 200 OK + worker name and UUID
        M->>M: Register worker and fetch node hardware stats
    end
```

---

## 5. Task Submission

```mermaid
sequenceDiagram
    actor O as Operator
    participant C as Control
    participant M as Manager
    participant W as Worker

    critical submit task
        O->>C: netbula control run task.json
        C->>M: POST /tasks, Netbula-Token: auth-token

        alt Valid token
            M-->>M: Create task t
            M-->>C: 201 Created
        else else
            M-->>C: 401 Unauthorized
        end
    end

    par
        loop
            alt manager has taskEvent left in queue to send
                M->>M: Dequeue event & assign + send it to a worker
                M->>W: POST /tasks
                W-->>M: 201 Created
                W->>W: AddTask(task)
            else else
                M-->>M: sleep for 15 seconds
            end
        end
    and
        loop
            alt worker has task left in queue to run
                W->>W: Dequeue task t
                W->>W: transitionTask(t)
            else else
                W-->>W: sleep for 10 seconds
            end
        end
    end
```
