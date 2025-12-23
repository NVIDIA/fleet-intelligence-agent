# Usage

## Basic Commands

### Quick Health Scan

```bash
sudo gpuhealth scan
```

Performs a quick health scan of GPUs and system components. Returns immediately with a summary of any detected issues.

**Aliases:** `sudo gpuhealth check`, `sudo gpuhealth s`

### Start Monitoring Server

```bash
sudo gpuhealth run
```

Starts the HTTP API server on port 15133. The server provides REST endpoints and Prometheus metrics.

**Options:**
- `--log-level`: Set logging level (debug, info, warn, error)
- `--listen-address`: Change listen address (default: `0.0.0.0:15133`)
- `--pprof`: Enable pprof profiling endpoint (note: pprof routes are not yet exposed)
- `--components`: Enable/disable specific components (default: all except docker, kubelet, tailscale, containerd, fuse, nfs)

### Check Status

```bash
sudo gpuhealth status
```

Displays the current status of the gpuhealth service and monitored components.

**Alias:** `sudo gpuhealth st`

### Machine Information

```bash
sudo gpuhealth machine-info
```

Shows detailed information about the machine:
- Hardware specifications (CPU, memory, disk)
- GPU configuration and driver version
- CUDA runtime version
- Operating system and kernel version
- Network interfaces
- System UUID and machine ID

### View/Update Metadata

```bash
# View current metadata
sudo gpuhealth metadata

# Set metadata key-value pair
sudo gpuhealth metadata --set-key="key" --set-value="value"
```

Used to view or update the agent's metadata store, including remote export configuration.

### Enroll Agent

```bash
sudo gpuhealth enroll --endpoint=https://api.example.com --token=<your-sak-token>
```

Enrolls the agent with the GPU Health backend by exchanging a Service Account Key (SAK) token for a JWT token. The JWT token and backend endpoints are stored locally for subsequent data exports.

**Required Options:**
- `--endpoint`: Base endpoint URL for the GPU Health backend (must use HTTPS)
- `--token`: Service Account Key (SAK) token for authentication

**What it does:**
1. Validates the endpoint URL (must be HTTPS)
2. Makes an enrollment request to exchange the SAK token for a JWT token
3. Stores the JWT token and backend endpoints (metrics, logs, nonce) in the local metadata database
4. The stored credentials are used automatically by the agent for data export

**Example output:**
```
Enrollment succeeded
```

**Error handling:**
- 400: Token format is incorrect
- 401: Token is invalid
- 403: Token is expired or revoked
- 404: Endpoint not found
- 429: Server is rate limiting (retry later)
- 502/503/504: Temporary server issues (retry)

### Unenroll Agent

```bash
sudo gpuhealth unenroll
```

Removes all enrollment credentials and backend endpoints from the agent. After unenrolling, the agent will no longer export data to the backend until re-enrolled.

**What it does:**
1. Clears the JWT token from local storage
2. Clears the SAK token from local storage
3. Removes all stored backend endpoints (enroll, metrics, logs, nonce)

Use this command when:
- Decommissioning a machine
- Switching to a different backend
- Troubleshooting authentication issues

## Offline Data Collection

For environments without network access or when you need to collect data to files:

```bash
sudo gpuhealth run --offline-mode --path=/tmp/gpu-health --duration=00:05:00 --format=csv
```

**Options:**
- `--offline-mode`: Disable HTTP API server and export to files
- `--path`: Directory to write data files
- `--duration`: How long to collect data (format: HH:MM:SS)
- `--format`: Output format (`csv` or `json`)

## Running as a Service

After package installation, the agent runs as a systemd service:

```bash
# Check service status
sudo systemctl status gpuhealthd

# Start/stop/restart service
sudo systemctl start gpuhealthd
sudo systemctl stop gpuhealthd
sudo systemctl restart gpuhealthd

# View logs
sudo journalctl -u gpuhealthd -f
```

## HTTP API

The gpuhealth HTTP API server runs on port 15133 by default and provides REST endpoints for monitoring data.

### Health Check

```bash
curl http://localhost:15133/healthz
```

Returns the health status of the API server

**Response:**
```json
{
  "status": "ok",
  "version": "v1"
}
```

### Machine Information

```bash
curl http://localhost:15133/machine-info
```

Returns basic machine info

Note: Detailed hardware and GPU information is available via the `gpuhealth machine-info` CLI command.

### Current Health States

```bash
curl http://localhost:15133/v1/states
```

Returns the current health states of all monitored components

### Component Metrics

```bash
curl http://localhost:15133/v1/metrics
```

Returns metrics data in JSON format from all monitored components

**Query Parameters:**
- `startTime`: Unix timestamp to retrieve metrics since a specific time
- `components`: Filter metrics by component name

**Example:**
```bash
# Get metrics from the last hour
curl "http://localhost:15133/v1/metrics?startTime=$(date -d '1 hour ago' +%s)"

# Get metrics for specific component
curl "http://localhost:15133/v1/metrics?components=accelerator-nvidia-temperature"
```

### Component Events

```bash
curl http://localhost:15133/v1/events
```

Returns event data from all monitored components (errors, warnings, state changes)

**Query Parameters:**
- `since`: Unix timestamp to retrieve events since a specific time (default: last hour)
- `components`: Filter events by component name

**Example:**
```bash
# Get events from the last hour
curl "http://localhost:15133/v1/events?since=$(date -d '1 hour ago' +%s)"

# Get events for specific component
curl "http://localhost:15133/v1/events?components=accelerator-nvidia-xid"
```

### Prometheus Metrics

```bash
curl http://localhost:15133/metrics
```

Returns metrics in Prometheus exposition format for integration with monitoring systems

## Troubleshooting

### Service won't start

1. Check service status:
   ```bash
   sudo systemctl status gpuhealthd
   ```

2. View recent logs:
   ```bash
   sudo journalctl -u gpuhealthd -n 50
   ```

3. Verify NVIDIA drivers are installed (if using GPUs):
   ```bash
   nvidia-smi
   ```

4. Check port availability:
   ```bash
   sudo netstat -tlnp | grep 15133
   ```

### Export issues

1. Check the logs:
   ```bash
   sudo journalctl -u gpuhealthd -f
   ```

2. Verify your configuration:
   ```bash
   sudo gpuhealth metadata
   ```

3. Test connectivity to the export endpoint manually with `curl`

4. Check proxy settings in `/etc/default/gpuhealth` if behind a firewall

### High resource usage

The agent should use <100MB RAM and <1% CPU. Higher usage might indicate:

- Very frequent collection intervals (check `GPUHEALTH_COLLECT_INTERVAL`)
- Large lookback windows (check `GPUHEALTH_METRICS_LOOKBACK` and `GPUHEALTH_EVENTS_LOOKBACK`)
- Many GPUs in the system (resource usage scales with GPU count)
- Debug logging enabled (use `--log-level=warn` or `error`)

