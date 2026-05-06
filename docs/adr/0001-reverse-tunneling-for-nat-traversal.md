# ADR 0001: Use reverse tunneling for NAT traversal

## Status

Accepted

## Context

For typical container orchestrators(e.g., Kubernetes), distributed worker machines are generally required to be able to reach each other directly.

This means users have to either ensure every host has a public IP address, or be forced to use VPNs like Tailscale to organize machines into the same subnet, which introduces external dependencies.

Many "homelab" environments are behind NAT(Network Address Translation) provided by ISPs. This makes it difficult for them to communicate with external compute power, such as Virtual Private Servers(VPS) or cloud hosts.

## Decision

We're implementing a **Reverse-Tunneling scheme**, that requires only a single "manager" server to have a public IP address.

- Workers-Initiated Connection: On startup, the worker application dials out to the public IP address of the manager using TCP.

- Persistent Control Channel: Once the TCP connection is established, the manager wraps the raw TCP stream with a Yamux client and stores the session in memory. This allows the manager to send multiple HTTP requests (multiplex) to and receive responses from the connected worker at any time.

## Consequences

- Pros:
  - Easier deployments: Users only need to run the Netbula binary, no need for VPNs or public IPs.
  - NAT traversal: Enables integration of home-based hardware with cloud-based infrastructures.

- Cons:
  - Connection overheads: The manager must now keep track of all long-lived TCP connections from workers.
  - Manager as a SPOF: Since there's only one manager and the manager is the central coordinator for all commands, its downtime brings down the control plane of the entire distributed system.
  - Harder inter-service communication integration: To implement inter-service communication in the future, we might need to relay all traffics through the manager, which is a serious bottleneck from the networking side
    - Solution: Use WireGuard-based p2p networking, only use the manager as a TURN server when other p2p methods are not working.
