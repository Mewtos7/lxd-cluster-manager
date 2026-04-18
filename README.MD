# LX Container Weaver

Smart cluster orchestration for LXD environments with automatic scaling, consolidation, and workload management to keep infrastructure efficient and cost-aware.

[![License](https://img.shields.io/github/license/Mewtos7/lx-container-weaver?style=for-the-badge)](LICENSE)
[![Contributions Welcome](https://img.shields.io/badge/contributions-welcome-brightgreen?style=for-the-badge)](CONTRIBUTING.md)
[![Last Commit](https://img.shields.io/github/last-commit/Mewtos7/lx-container-weaver?style=for-the-badge)](https://github.com/Mewtos7/lx-container-weaver/commits/main)
[![Open Issues](https://img.shields.io/github/issues/Mewtos7/lx-container-weaver?style=for-the-badge)](https://github.com/Mewtos7/lx-container-weaver/issues)
[![Repo Size](https://img.shields.io/github/repo-size/Mewtos7/lx-container-weaver?style=for-the-badge)](https://github.com/Mewtos7/lx-container-weaver)
[![Stars](https://img.shields.io/github/stars/Mewtos7/lx-container-weaver?style=for-the-badge)](https://github.com/Mewtos7/lx-container-weaver/stargazers)
[![Forks](https://img.shields.io/github/forks/Mewtos7/lx-container-weaver?style=for-the-badge)](https://github.com/Mewtos7/lx-container-weaver/network/members)
[![Commit Activity](https://img.shields.io/github/commit-activity/m/Mewtos7/lx-container-weaver?style=for-the-badge)](https://github.com/Mewtos7/lx-container-weaver/commits/main)

## Overview

LX Container Weaver helps operate LXD clusters on cloud infrastructure by making scaling and placement decisions based on current usage and workload demand.

The goal is simple:

- Keep enough capacity for container and VM scheduling.
- Avoid overprovisioning by consolidating workloads when possible.
- Reduce cloud spend by removing underused nodes and servers.

## Core Capabilities

- Cluster bootstrap with an initial two-node LXD setup.
- Resource-aware monitoring across nodes in a cluster.
- Horizontal scale-out when new workload placement requires additional capacity.
- Workload consolidation with automatic node/server scale-in when usage drops.
- API-driven management for core entities and operations.
- Multi-cluster management support.
- Live migration support for containers and VMs within the same cluster.
- CRUD-based configuration for nodes and containers through the manager API.

## Why Use It

- Better infrastructure utilization across LXD nodes.
- Lower operational overhead through automation.
- Cost-efficient cluster behavior with continuous right-sizing.
- Flexible management model for VM and container workloads.

## Project Links

- Contributor guide: [CONTRIBUTING.md](CONTRIBUTING.md)
- License (Apache 2.0): [LICENSE](LICENSE)
- Repository: [Mewtos7/lx-container-weaver](https://github.com/Mewtos7/lx-container-weaver)

## Local Development

Full setup instructions — prerequisites, quick-start, environment variables, database migrations, reset workflow, and troubleshooting — are in [docs/local-development.md](docs/local-development.md).

**Quick start (after cloning):**

```sh
cp .env.example .env          # copy config template
make gen-api-key              # generate an API key; paste the bcrypt hash into .env
make dev-up                   # start PostgreSQL and apply migrations
							 # then add the printed database connection value into .env
source .env && go run ./cmd/manager
```

Run `make help` to list all available targets.

## AI Assistance

This project uses AI tooling to assist with writing content. All contributions are reviewed and approved by a human maintainer before they are merged into the repository.

## Status

This project is under active development and open to improvements, fixes, and feature contributions.

If you want to contribute, start with [CONTRIBUTING.md](CONTRIBUTING.md).