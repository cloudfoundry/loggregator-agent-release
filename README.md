Loggregator Agent Release
[![slack.cloudfoundry.org][slack-badge]][loggregator-slack]
[![CI Badge][ci-badge]][ci-pipeline]
===================================================
A collection of composable agents used for interacting with Loggregator and Syslog

## Agents Architecture

![architecture]

### UDP Forwarder
Accepts the Loggregator v1 api over UDP and forwards it to a downstream Loggregator v2 consumer. 
This is available for backwards compatibility between the v1 API and the Forwarder, Syslog, or Loggregator agents.

### Forwarder Agent
A Loggregator v2 api multiplexer. This Agent Accepts Loggregator v2 and forwards to other agents colocated on the same VM
that accept Loggregator v2. To configure downstream agents, simply place a file named `ingress_port.yml` in the job's config
directory. [example][ingress-port]

### Syslog Agent
An agent that forwards app logs to a syslog drain. Drains are registered by binding User Provided Services to apps. 
Any logs coming from a registered app are forwarded to the configured endpoint. 

#### Syslog Binding Cache
Syslog Agents can overwhelm CAPI when querying for existing bindings. This component acts a a proxy for the CAPI Binding
query.

### Loggregator Agent
Loggregator Agent is a Cloud Foundry component that forwards logs and metrics
into the Loggregator subsystem. More information can be found in the [docs][loggregator-agent]

### System Metrics Agent
A standalone agent to provide VM system metrics via a prometheus-scrapable endpoint. A list of metrics
is available in the [docs][system-metrics-agent]

#### Metric Scraper
A central component for scraping `system-metrics-agents` and forwarding the metrics to the firehose. Metric Scraper
attempts to scrape the configured port across all vms deployed to the director. If present, this job can be configured to
communicate with the [Leadership Election Release][leadership-election] so duplicate
scrapes are avoided in an HA environment.

### Prom Scraper
A simple agent that reads from any local prometheus-scrapable endpoint and forwards the results to Loggregator.
Configurable via files in colocated bosh jobs as described in the [docs][prom-scraper]
scrapes are avoided in an HA environment.

### Metrics agent
A simple agent that receives metrics from the Forwarder Agent and exposes them on a prometheus-scrapable endpoint.
More information can be found in the [docs][metrics-agent]

## More Resources and Documentation

### Feedabck

We are always looking to gather feedback and input
from Loggregator operators. Get in touch or file an issue if you have feature
suggestions you'd like to see added.

### Pivotal Tracker

Items marked as "In Flight" on the Roadmap are tracked as new Features in
[Pivotal Tracker][loggregator-tracker].


[slack-badge]:         https://slack.cloudfoundry.org/badge.svg
[loggregator-slack]:   https://cloudfoundry.slack.com/archives/loggregator
[ci-badge]:            https://loggregator.ci.cf-app.com/api/v1/pipelines/products/jobs/loggregator-tests-with-bumped-modules/badge
[ci-pipeline]:         https://loggregator.ci.cf-app.com/teams/main/pipelines/products?group=loggregator-agent
[loggregator-tracker]: https://www.pivotaltracker.com/n/projects/993188
[leadersip-election]:  https://github.com/cloudfoundry/leadership-election-release
[ingress-port]:        https://github.com/cloudfoundry/loggregator-agent-release/blob/develop/jobs/loggr-syslog-agent/templates/ingress_port.yml.erb

[loggregator-agent]:    docs/loggregator-agent.md
[prom-scraper]:         docs/prom-scraper.md
[system-metrics-agent]: docs/system-metrics-agent.md
[metrics-agent]:        docs/metrics-agent.md
[architecture]:         docs/agent-architecture.png
