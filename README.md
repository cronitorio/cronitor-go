# Cronitor Go Library
![Test](https://github.com/cronitorio/cronitor-go/workflows/Test/badge.svg)

[Cronitor](https://cronitor.io/) provides end-to-end monitoring for background jobs, websites, APIs, and anything else that can send or receive an HTTP request. This library provides convenient access to the Cronitor API from applications written in Go. See our [API docs](https://cronitor.io/docs/api) for detailed references on configuring monitors and sending telemetry pings.

In this guide:

- [Installation](#installation)
- [Monitoring Background Jobs](#monitoring-background-jobs)
- [Sending Telemetry Events](#sending-telemetry-events)
- [Configuring Monitors](#configuring-monitors)
- [Package Configuration & Env Vars](#package-configuration)
- [Roadmap](#roadmap)
- [Contributing](#contributing)

## Installation

```
go get github.com/cronitorio/cronitor-go
```

The package requires Go 1.21 or later and depends only on `gopkg.in/yaml.v3`.

## Monitoring Background Jobs

`cronitor.Job` is a lightweight way to monitor any background task regardless of how it is executed. It sends a telemetry event before calling your function and after it returns. If your function returns an error (or panics) a `fail` event is sent and the error is returned (or the panic re-raised).

```go
package main

import "github.com/cronitorio/cronitor-go"

func main() {
    // your api keys can found here - https://cronitor.io/settings/api
    cronitor.APIKey = "apiKey123"

    // Wrap any function to monitor it.
    // If no monitor matches the provided key, one will be created automatically.
    out, err := cronitor.Job("send-invoices", func() (any, error) {
        return sendInvoices()
    })
    _ = out
    _ = err
}
```

`cronitor.RunJob` is the generic form that preserves the return type of your function, and `cronitor.NewJob` wraps a function once so every invocation is monitored:

```go
count, err := cronitor.RunJob("send-invoices", func() (int, error) {
    return sendInvoices()
})

sendInvoicesJob := cronitor.NewJob("send-invoices", sendInvoices)
sendInvoicesJob() // pings run, then complete or fail
```

Options: `cronitor.WithJobEnv("staging")` sends the pings with a specific environment, and `cronitor.WithLogOutput(false)` stops the function's return value being sent as the message of the `complete` event.

#### You can provide monitor attributes that will be synced when your app starts

To sync attributes, provide an API key with monitor:write privileges. Attributes are queued by `cronitor.WithAttributes` and sent to Cronitor with a single call to `cronitor.SyncMonitors()`, so call it once your jobs are registered (there is no background goroutine). `NewJob` queues attributes when the job is wrapped; `Job` and `RunJob` queue them each time they run. The queue holds one entry per monitor key, so the most recent attributes for a key win.

```go
// Copy your SDK Integration key from https://cronitor.io/app/settings/api
cronitor.APIKey = "apiKey123"

sendInvoicesJob := cronitor.NewJob("send-invoices", sendInvoices,
    cronitor.WithAttributes(map[string]any{
        "schedule": "0 8 * * *",
        "notify":   []string{"devops-alerts"},
    }))

if err := cronitor.SyncMonitors(); err != nil {
    log.Printf("could not sync monitors: %v", err)
}
```

## Sending Telemetry Events

If you want to send heartbeat events, or want finer control over when/how [telemetry events](https://cronitor.io/docs/telemetry-api) are sent for your jobs, you can create a monitor instance and call the `Ping` method.

```go
// your api keys can found here - https://cronitor.io/settings/api
cronitor.APIKey = "apiKey123"

// optionally, set an environment
cronitor.Environment = "staging"

monitor := cronitor.NewMonitor("heartbeat-monitor")
monitor.Ping(cronitor.PingOptions{}) // send a heartbeat event

// optional params can be passed in PingOptions.
// for a complete list see https://cronitor.io/docs/telemetry-api#parameters
monitor.Ping(cronitor.PingOptions{
    State:   cronitor.StateRun, // run|complete|fail used to measure lifecycle of a job, ok used for manual reset only.
    Message: "",                // message that will be displayed in alerts as well as monitor activity panel on your dashboard.
    Metrics: map[string]float64{
        "duration":      100,  // how long the job ran (complete|fail only). cronitor will calculate this when not provided
        "count":         4500, // if your job is processing a number of items you can report a count
        "error_count":   10,   // the number of errors that occurred while this job was running
        "queue_depth":   42,   // custom names are sent as metric=name:value alongside the built-ins
        "quality_score": 0.98,
    },
})
```

`PingOptions.Metrics` accepts any metric name. Built-ins (`duration`, `count`, `error_count`) have special meaning in Cronitor; other keys such as `queue_depth` or `quality_score` are sent as additional `metric=name:value` query parameters.

`Ping` returns an error if the request could not be delivered. If no API key is configured it logs an error and returns `nil` without sending anything, so a missing key never breaks your job.

Per-monitor overrides are available as options: `cronitor.WithAPIKey`, `cronitor.WithAPIVersion` and `cronitor.WithEnv`.

## Configuring Monitors

### YAML Configuration File

You can configure all of your monitors using a single YAML file. This can be version controlled and synced to Cronitor as part of a deployment or build process. For details on all of the attributes that can be set, see the [Monitor API](https://cronitor.io/docs/monitor-api) documentation.

```go
// your api keys can found here - https://cronitor.io/settings/api
cronitor.APIKey = "apiKey123"

config, err := cronitor.ReadConfig("./cronitor.yaml") // parse the yaml file of monitors

err = cronitor.ValidateConfig() // send monitors to Cronitor for configuration validation

err = cronitor.ApplyConfig() // sync the monitors from the config file to Cronitor

err = cronitor.GenerateConfig() // generate a new config file from the Cronitor API
```

The timeout value for `ValidateConfig`, `ApplyConfig` and `GenerateConfig` is 10 seconds by default. The value can be rewritten by setting the environment variable `CRONITOR_TIMEOUT` (in seconds). It can also be rewritten by assigning a value to `cronitor.Timeout`.

```go
cronitor.Timeout = 30 * time.Second
cronitor.ApplyConfig()
```

The `cronitor.yaml` file includes three top level keys `jobs`, `checks`, `heartbeats`. You can configure monitors under each key by defining [monitors](https://cronitor.io/docs/monitor-api#attributes).

```yaml
jobs:
    nightly-database-backup:
        schedule: 0 0 * * *
        notify:
            - devops-alert-pagerduty
        assertions:
            - metric.duration < 5 minutes

    send-welcome-email:
        schedule: every 10 minutes
        assertions:
            - metric.count > 0
            - metric.duration < 30 seconds

checks:
    cronitor-homepage:
        request:
            url: https://cronitor.io
            regions:
                - us-east-1
                - eu-central-1
                - ap-northeast-1
        assertions:
            - response.code = 200
            - response.time < 2s

    cronitor-ping-api:
        request:
            url: https://cronitor.link/ping
        assertions:
            - response.body contains ok
            - response.time < .25s

heartbeats:
    production-deploy:
        notify:
            alerts: ['deploys-slack']
            events: true # send alert when the event occurs

```
#### Async Uploads
If you are working with large YAML files (300+ monitors), you may hit timeouts when trying to sync monitors in a single http request. This workload can be processed asynchronously by adding the key `async: true` to the config file. The request will immediately return a `batch_key`. If a `webhook_url` parameter is included, Cronitor will POST to that URL with the results of the background processing and will include the `batch_key` matching the one returned in the initial response.

### PutMonitors

You can also create and update monitors by calling `cronitor.PutMonitors`. For details on all of the attributes that can be set see the Monitor API [documentation](https://cronitor.io/docs/monitor-api#attributes).

```go
monitors, err := cronitor.PutMonitors([]map[string]any{
    {
        "type":     "job",
        "key":      "send-customer-invoices",
        "schedule": "0 0 * * *",
        "assertions": []string{
            "metric.duration < 5 min",
        },
        "notify": []string{"devops-alerts-slack"},
    },
    {
        "type":     "check",
        "key":      "Cronitor Homepage",
        "schedule": "every 45 seconds",
        "request": map[string]any{
            "url": "https://cronitor.io",
        },
        "assertions": []string{
            "response.code = 200",
            "response.time < 600ms",
        },
    },
}, cronitor.PutOptions{})
```

Pass `cronitor.PutOptions{Rollback: true}` to validate the monitors without saving them. A `400` response is returned as a `*cronitor.APIValidationError`. `cronitor.PutMonitorsYAML` accepts a document in the `cronitor.yaml` shape (`jobs`, `checks`, `heartbeats`) and `cronitor.AsYAML` downloads all of your monitors as YAML.

### Listing and Inspecting Monitors

You can fetch multiple monitors using `cronitor.ListMonitors` with optional filtering and pagination:

```go
// Fetch specific monitors by key
monitors, err := cronitor.ListMonitors(cronitor.ListOptions{Keys: []string{"backup-job", "health-check", "send-invoices"}})

// Fetch all monitors (first page, 100 results by default)
monitors, err = cronitor.ListMonitors(cronitor.ListOptions{})

// Fetch monitors with filters
monitors, err = cronitor.ListMonitors(cronitor.ListOptions{Type: "check", State: "failing"})

// Fetch a specific page
monitors, err = cronitor.ListMonitors(cronitor.ListOptions{Type: "job", Page: 2, PageSize: 50})

// Fetch all pages automatically
monitors, err = cronitor.ListMonitors(cronitor.ListOptions{Type: "job", AutoPaginate: true})

// Search monitors
monitors, err = cronitor.ListMonitors(cronitor.ListOptions{Search: "backup"})
```

After fetching a monitor, access its data with `Data()` (cached after the first call; `Fetch()` always hits the API). The result is a `cronitor.MonitorData`, a plain `map[string]any` with typed accessors for common fields and `Get` for nested values:

```go
// Fetch an existing monitor
monitor := cronitor.NewMonitor("send-invoices")
data, err := monitor.Data()
if err != nil {
    // errors.Is(err, cronitor.ErrMonitorNotFound) when the key is unknown
}

// Access monitor attributes
fmt.Println(data.Name())
fmt.Println(data.Type())
fmt.Println(data.Schedule())

// Access nested data
fmt.Println(data.GetString("attributes", "code"))
if stamp, ok := data.Get("latest_event", "stamp"); ok {
    fmt.Println(stamp, data.GetString("latest_event", "event"))
}

// Pretty print the entire monitor as JSON
fmt.Println(data)
```

### Pausing, Resetting, and Deleting

```go
monitor := cronitor.NewMonitor("heartbeat-monitor")

monitor.Pause(24) // pause alerting for 24 hours
monitor.Unpause() // alias for Pause(0)
monitor.OK()      // manually reset to a passing state alias for monitor.Ping(cronitor.PingOptions{State: cronitor.StateOK})
monitor.Delete()  // destroy the monitor
```

### Errors

Every error returned by the package wraps one of the sentinels `cronitor.ErrMonitorNotFound`, `cronitor.ErrConfigValidation`, `cronitor.ErrAPIValidation`, `cronitor.ErrAuthentication` and `cronitor.ErrAPI`, so they can be checked with `errors.Is`. The concrete types `*cronitor.MonitorNotFoundError`, `*cronitor.ConfigValidationError`, `*cronitor.APIValidationError`, `*cronitor.AuthenticationError` and `*cronitor.APIError` carry details (key, HTTP status code, response body) and can be extracted with `errors.As`.

## Package Configuration

The package needs to be configured with your account's `API key`, which is available on the [account settings](https://cronitor.io/settings) page. You can also optionally specify an `APIVersion` and an `Environment`. If not provided, your account default is used. These can also be supplied using the environment variables `CRONITOR_API_KEY`, `CRONITOR_API_VERSION`, `CRONITOR_ENVIRONMENT`.

```go
// your api keys can found here - https://cronitor.io/settings
cronitor.APIKey = "apiKey123"
cronitor.APIVersion = "2020-10-01"
cronitor.Environment = "cluster_1_prod"
```

| Setting                 | Environment variable   | Default                          |
|-------------------------|------------------------|----------------------------------|
| `cronitor.APIKey`       | `CRONITOR_API_KEY`     |                                  |
| `cronitor.APIVersion`   | `CRONITOR_API_VERSION` |                                  |
| `cronitor.Environment`  | `CRONITOR_ENVIRONMENT` |                                  |
| `cronitor.Timeout`      | `CRONITOR_TIMEOUT`     | `10s` (Monitor API calls)        |
| `cronitor.ConfigPath`   | `CRONITOR_CONFIG`      |                                  |
| `cronitor.PingURL`      |                        | `https://cronitor.link/p`        |
| `cronitor.MonitorAPIURL`|                        | `https://cronitor.io/api/monitors` |
| `cronitor.Logger`       |                        | `log.New(os.Stderr, "cronitor: ", log.LstdFlags)` |

Environment variables are read when the package is initialised; call `cronitor.LoadEnv()` to re-read them. Requests that fail with a connection error or a `5xx` response are retried up to 3 times with exponential backoff.

## Roadmap

- Integrations with popular cron libraries such as [robfig/cron](https://github.com/robfig/cron), so scheduled functions can be monitored automatically.

## Contributing

Pull requests and features are happily considered! By participating in this project you agree to abide by the [Code of Conduct](http://contributor-covenant.org/version/2/0).

### To contribute

Fork, then clone the repo:

    git clone git@github.com:your-username/cronitor-go.git

Make sure the tests pass:

    go test ./...

**Optional:** Run integration tests against the real Cronitor API:

    export CRONITOR_TEST_API_KEY=your_api_key_here
    go test -run Integration -v ./...

Make your change. Add tests for your change. Make the tests pass:

    gofmt -l .
    go vet ./...
    go test ./...

Push to your fork and [submit a pull request](https://github.com/cronitorio/cronitor-go/compare/)
