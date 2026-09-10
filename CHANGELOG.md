# Changelog

All notable changes to this project will be documented in this file.

## 1.0.1

- The `User-Agent` header now includes the package version, for example `cronitor-go/1.0.1`.
- Added `cronitor.Version`.

## 1.0.0

Initial release.

- Telemetry pings via `Monitor.Ping` with state, message, series, host, metrics and env parameters.
- `Job`, `NewJob` and `RunJob` wrappers that send run/complete/fail pings with a duration metric.
- Monitor API support: `Fetch`/`Data`, `Delete`, `Pause`/`Unpause`, `ListMonitors`, `PutMonitors`, `PutMonitorsYAML` and `AsYAML`.
- YAML configuration helpers: `ReadConfig`, `ValidateConfig`, `ApplyConfig` and `GenerateConfig`.
- Package configuration from `CRONITOR_API_KEY`, `CRONITOR_API_VERSION`, `CRONITOR_ENVIRONMENT`, `CRONITOR_TIMEOUT` and `CRONITOR_CONFIG`.
- Automatic retries with exponential backoff on connection errors and 5xx responses.
