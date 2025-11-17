## Voice Processor Load Test

The `cmd/loadtest` program simulates a stream of `GameSessionCreated` events without calling any external services. It exercises the session snapshot parsing logic along with participant creation/notification to help validate concurrency behaviour and investigate CPU/memory usage.

### Running the load test

```bash
# Build/run directly
go run ./cmd/loadtest \
  -requests 20000 \
  -workers 16 \
  -participants 12 \
  -cpuprofile cpu.out \
  -memprofile mem.out

# Or throttle the test to ~1 vCPU / 512MiB using Docker or systemd-run
scripts/run-loadtest.sh \
  -requests 20000 \
  -workers 16 \
  -participants 12
```

Flags:

- `-requests` total number of simulated game sessions (default: 1000).
- `-workers` size of the worker pool driving concurrent events (default: NumCPU).
- `-participants` number of active session members per snapshot (default: 8).
- `-cpuprofile` optional path that triggers CPU profiling for the duration of the test.
- `-memprofile` optional path that writes a heap profile after the run completes.

### Analysing profiles

Profiles can be inspected with the standard Go tooling:

```bash
go tool pprof cpu.out
go tool pprof mem.out
```

From inside `pprof`, use commands such as `top`, `list <symbol>`, or `web` to explore hotspots. This makes it easy to reason about how the snapshot decoder and participant fan-out behave under sustained load before shipping changes into the gRPC handler stack.
