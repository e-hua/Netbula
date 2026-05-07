# ADR 0003: E-PVM algorithm for task scheduling

## Status

Accepted

## Context

Netbula needs to decide which worker node to assign a task to when multiple workers are available.

The simplest approach, Round Robin, distributes tasks evenly across workers regardless of their current load. This works for homogeneous workloads but produces poor results in practice:

- A worker that is already CPU-saturated may receive another task simply because it is "next in rotation".
- Workers with different hardware specs (e.g., a 2-core homelab machine vs. an 8-core VPS) are treated as equivalent.
- There is no awareness of whether a worker has enough disk space to run a given container image.

We needed a load-aware scheduler that selects the node where the marginal cost of running the new task is lowest.

## Decision

We implement the **Enhanced Parallel Virtual Machine (E-PVM)** algorithm, described in the [MOSIX paper](https://mosix.cs.huji.ac.il/pub/ocja.pdf), as the default production scheduler.

Scheduling is split into three phases, defined by the `Scheduler` interface:

**1. `SelectCandidateNodes`: hard filtering**

Nodes that cannot physically run the task are eliminated first. Currently the only hard constraint is disk: a node is excluded if its available disk space `(1 - diskUsedPercent) * totalDisk` is less than the task's `Disk` requirement.

**2. `Score`: marginal cost calculation**

Each candidate node is assigned a score representing how much more expensive it becomes if the task is placed there. The score uses the **Lieb square ice constant** (`LIEB ≈ 1.5396`) as the base of an exponential cost function:

```
cpuLoad             = node.CpuAverageLoad / node.Cores
predictedCpuLoad    = cpuLoad + task.Cpu / node.Cores

memUsage            = node.MemoryAllocatedPercent / 100
predictedMemUsage   = memUsage + task.Memory / node.Memory

cpuCost  = LIEB ^ predictedCpuLoad  - LIEB ^ cpuLoad
memCost  = LIEB ^ predictedMemUsage - LIEB ^ memUsage

score = cpuCost + memCost
```

The exponential base means the cost of adding load grows non-linearly — placing a task on an already-busy node is penalised much more heavily than placing it on an idle one.

**3. `Pick`: select lowest-cost candidate**

The candidate node with the lowest total score is selected for the task.

Round Robin remains available as an alternative implementation of the same `Scheduler` interface, used in tests and as a fallback for simple deployments.

## Consequences

- Pros:
  - Load-aware: tasks are naturally spread to underutilised nodes, avoiding saturation of any single worker.
  - Hardware-aware: nodes with more cores or memory receive proportionally higher scores for the same task, reflecting their greater capacity.
  - The exponential cost curve acts as a soft ceiling — nodes approaching full utilisation become increasingly unlikely to be selected without hard-rejecting them.
  - The three-phase `Scheduler` interface (`SelectCandidateNodes` / `Score` / `Pick`) makes it straightforward to swap in alternative algorithms.

- Cons:
  - Candidate filtering only enforces disk constraints. Tasks with CPU or memory requirements that exceed a node's available capacity are not hard-rejected and may still be scheduled there if they score lowest.
  - Node stats are collected by periodic polling (every 15 seconds), so scheduling decisions are based on slightly stale resource data rather than real-time utilisation.
  - The algorithm has no awareness of task runtime or completion time — a long-running task and a short one are treated identically at scheduling time.
