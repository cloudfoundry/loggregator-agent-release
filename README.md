Loggregator Agent Release
===================================================
A collection of composable agents used for interacting with Loggregator and Syslog



adsfasdsf

If you have any questions, or want to get attention for a PR or issue please reach out on the [#logging-and-metrics channel in the cloudfoundry slack](https://cloudfoundry.slack.com/archives/CUW93AF3M)

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
