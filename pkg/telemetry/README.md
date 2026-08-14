# Cluster Toolkit Telemetry

The `pkg/telemetry` package provides a robust telemetry system for the Google Cloud Cluster Toolkit CLI. It is responsible for collecting CLI usage metrics, formatting them into Concord-compatible events, and uploading them to Google's Clearcut infrastructure.

This helps the product development team assess module adoption, prioritize features, and identify recurring deployment issues based on real usage insights to make customer experience better.

> [!WARNING]
> Users are requested to NOT modify any part of this package to avoid tampering with telemetry data collection. In the case of wanting to disable telemetry, run this command: `./gcluster telemetry off`.

## Architecture & Flow

The execution flow of the telemetry lifecycle follows these steps:

1. **Initialization**: A `Collector` is initialized at the start of a CLI command using `NewCollector(cmd, args)`. This records the start time to calculate latency and other state variables.
2. **Metrics Collection**: Upon command completion, `CollectMetrics(errorCode)` is invoked to capture the exit code and event metadata.
3. **Event Construction**: The toolkit state is formatted into a `ConcordEvent` which includes the toolkit version, command name, latency, and metadata.
4. **Payload Building**: The `ConcordEvent` is marshaled into a `SourceExtensionJson` string and wrapped in a `LogRequest` payload.
5. **Uploading (Flush)**: The `LogRequest` is sent via an HTTP POST request to the Clearcut production endpoint (`https://play.googleapis.com/log`).

## Key Components

* `types.go`: Defines the core data structures (`Collector`, `LogRequest`, `ConcordEvent`) and constants (URLs, Client Types).
* `collector.go`: Contains the logic for initializing the `Collector` state, parsing Cobra commands, and calculating execution latency.
* `telemetry.go`: Exposes the `Execute()` entry point which runs the end-to-end collection and upload process.
* `uploader.go`: Handles the HTTP network logic to send the JSON payload to Clearcut securely without disrupting the user experience.

## Usage

Telemetry collection is globally integrated into the CLI via the root command in `cmd/root.go`.

Because it is initialized at the root level, all subcommands automatically inherit telemetry collection. Individual commands do not need to manually instantiate or flush the telemetry collector.
