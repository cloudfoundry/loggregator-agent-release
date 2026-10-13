Loggregator Agent Release
===================================================
A collection of composable agents used for interacting with Loggregator and Syslog

If you have any questions, or want to get attention for a PR or issue please reach out on the [#logging-and-metrics channel in the cloudfoundry slack](https://cloudfoundry.slack.com/archives/CUW93AF3M)

## Agents Architecture

![architecture]

### UDP Forwarder
Accepts the Loggregator v1 api over UDP and forwards it to a downstream Loggregator v2 consumer. 
This is available for backwards compatibility between the v1 API and the Forwarder, Syslog, or Loggregator agents.

### Forwarder Agent
A Loggregator v2 API multiplexer. Downstream registration files supply a port and protocol; destinations default to localhost.
`protocol: otelcol` destinations use `otel_collector.host` (default `127.0.0.1`), allowing an OTel Collector on another host.
To configure downstream agents, place an `ingress_port.yml` file in the job's config directory. [example][ingress-port]

### Syslog Agent
An agent that forwards app logs to a syslog drain. Drains are registered by binding User Provided Services to apps. 
Any logs coming from a registered app are forwarded to the configured endpoint. [docs][syslog-agent]

#### Syslog Binding Cache
Syslog Agents can overwhelm CAPI when querying for existing bindings. This component acts a a proxy for the CAPI Binding
query.

### Loggregator Agent
Loggregator Agent is a Cloud Foundry component that forwards logs and metrics
into the Loggregator subsystem. More information can be found in the [docs][loggregator-agent]

### Prom Scraper
A simple agent that reads from any local prometheus-scrapable endpoint and forwards the results to Loggregator.
Configurable via files in colocated bosh jobs as described in the [docs][prom-scraper]
scrapes are avoided in an HA environment.

### OTel Collector
This is the Cloud Foundry distribution of the OTel Collector. It can be found in the
[otel-collector-release](https://github.com/cloudfoundry/otel-collector-release) repo. It accepts metrics in OTLP format
and forwards them to any configured exporters. More information can be found in the [OpenTelemetry
docs](https://opentelemetry.io/docs/collector)

## More Resources and Documentation

### Feedback

We are always looking to gather feedback and input
from Loggregator operators. Get in touch or file an issue if you have feature
suggestions you'd like to see added.


[ingress-port]:        https://github.com/cloudfoundry/loggregator-agent-release/blob/develop/jobs/loggr-syslog-agent/templates/ingress_port.yml.erb

[loggregator-agent]:    docs/loggregator-agent.md
[prom-scraper]:         docs/prom-scraper.md
[architecture]:         docs/agent-architecture.png
[syslog-agent]:         docs/syslog-agent.md
