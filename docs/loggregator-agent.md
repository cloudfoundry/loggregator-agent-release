### Loggregator Agent
Loggregator Agent is a Cloud Foundry component that forwards logs and metrics
into the Loggregator subsystem by taking traffic from the various emitter
sources (diego, router, etc) and routing that traffic to one or more dopplers.
An instance of Loggregator Agent runs on each VM in an environment and is
therefore co-located on the emitter sources.

Traffic is routed to Dopplers in the same AZ, but it can fall back to any
Doppler if none are available in the current AZ. All Loggregator Agent traffic
is randomly distributed across available Dopplers.

Loggregator Agent only listens to local network interfaces and all logs and
metrics are immediately signed before forwarding to Dopplers. This prevents
man-in-the-middle attacks and ensures data integrity.

#### Compatability

The Loggregator Agent is compatable with Loggregator v96+.

#### Using Loggregator Agent

#### Deploying

##### Standalone

To deploy loggregator agent standalone, add the following job to all instance groups and the variables to the variables section.
If metrics from loggregator agent are desired, deploy prom scraper with the same `metric_scraper_ca`

```yaml
jobs:
- name: loggregator_agent
  release: loggregator-agent
  properties:
    disable_udp: true
    loggregator:
      tls:
        ca_cert: "((loggregator_tls_agent.ca))"
        agent:
          cert: "((loggregator_tls_agent.certificate))"
          key: "((loggregator_tls_agent.private_key))"
    metrics:
      ca_cert: "((loggregator_agent_metrics_tls.ca))"
      cert: "((loggregator_agent_metrics_tls.certificate))"
      key: "((loggregator_agent_metrics_tls.private_key))"
      server_name: loggregator_agent_metrics

variables:
- name: loggregator_tls_agent
  type: certificate
  options:
    ca: /bosh-<ENV_NAME>/cf/loggregator_ca
    common_name: metron
    extended_key_usage:
    - client_auth
    - server_auth

- name: loggregator_agent_metrics_tls
  type: certificate
  options:
    ca: metric_scraper_ca
    common_name: loggregator_agent_metrics
    extended_key_usage:
    - server_auth

- name: metric_scraper_ca
  type: certificate
  options:
    is_ca: true
    common_name: metricScraperCA
```

##### With Forwarder Agent

To deploy loggregator agent with other downstream agents, add the following jobs to all instance groups and the variables to the variables section.
If metrics from loggregator agent are desired, deploy prom scraper with the same `metric_scraper_ca`

```yaml
jobs:
- name: loggregator_agent
  release: loggregator-agent
  properties:
    grpc_port: 3459
    disable_udp: true
    loggregator:
      tls:
        ca_cert: "((loggregator_tls_agent.ca))"
        agent:
          cert: "((loggregator_tls_agent.certificate))"
          key: "((loggregator_tls_agent.private_key))"
    metrics:
      ca_cert: "((loggregator_agent_metrics_tls.ca))"
      cert: "((loggregator_agent_metrics_tls.certificate))"
      key: "((loggregator_agent_metrics_tls.private_key))"
      server_name: loggregator_agent_metrics

- name: forwarder_agent
  include:
    stemcell:
    - os: ubuntu-xenial
  jobs:
  - name: loggr-forwarder-agent
    release: loggregator-agent
    properties:
      tls:
        ca_cert: "((loggregator_tls_agent.ca))"
        cert: "((loggregator_tls_agent.certificate))"
        key: "((loggregator_tls_agent.private_key))"
      metrics:
        ca_cert: "((forwarder_agent_metrics_tls.ca))"
        cert: "((forwarder_agent_metrics_tls.certificate))"
        key: "((forwarder_agent_metrics_tls.private_key))"
        server_name: forwarder_agent_metrics

variables:
- name: loggregator_tls_agent
   type: certificate
  options:
    ca: /bosh-<ENV_NAME>/cf/loggregator_ca
    common_name: metron
    extended_key_usage:
    - client_auth
    - server_auth

- name: loggregator_agent_metrics_tls
  type: certificate
  options:
    ca: metric_scraper_ca
    common_name: loggregator_agent_metrics
    extended_key_usage:
    - server_auth

- name: forwarder_agent_metrics_tls
  type: certificate
  options:
    ca: metric_scraper_ca
    common_name: forwarder_agent_metrics
    extended_key_usage:
    - server_auth

- name: metric_scraper_ca
  type: certificate
  options:
    is_ca: true
    common_name: metricScraperCA
```

##### go-loggregator

There is Go client library: [go-loggregator][go-loggregator]. The client
library has several useful patterns along with examples to interact with a
Loggregator Agent.

[go-loggregator]: https://code.cloudfoundry.org/go-loggregator