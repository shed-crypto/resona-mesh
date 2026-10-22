# ResonaMesh Multi-Node Docker Testbed

This directory contains the Docker Compose environment for running a multi-node P2P mesh network locally:
- **Node A (`resonamesh_node_a`)**: Initial publisher / seeder node (HTTP: `http://localhost:8081`, P2P: 4001).
- **Node B (`resonamesh_node_b`)**: Consumer / listener node (HTTP: `http://localhost:8082`, P2P: 4002).
- **Node C (`resonamesh_node_c`)**: Autonomous keeper / secondary replica node (HTTP: `http://localhost:8083`, P2P: 4003).
- **Node D (`resonamesh_node_d`)**: Network governance / moderator node (HTTP: `http://localhost:8084`, P2P: 4004).

## Running the Cluster

```bash
# Build and start all 4 nodes in background
docker compose -f deploy/compose/docker-compose.yml up --build -d

# Check cluster logs
docker compose -f deploy/compose/docker-compose.yml logs -f

# Stop and wipe cluster state
docker compose -f deploy/compose/docker-compose.yml down -v
```
