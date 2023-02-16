### Syslog Agent

An agent that forwards app logs to a syslog drain. Drains are registered by binding User Provided Services to apps. 
Any logs coming from a registered app are forwarded to the configured endpoint. 

#### Deploying Syslog Agent

To deploy syslog agent, add the following jobs to all instance groups and the variables to the variables section.
If metrics from syslog agent are desired, deploy prom scraper with the same `metric_scraper_ca`

**Notes**
- aggregate_drains forward all metrics and all app logs to the drains.

```yaml
jobs:
- name: loggr-syslog-agent
  release: loggregator-agent
  properties:
    drain_ca_cert: "((log_cache_syslog_tls.ca))"
    port: 3460
    tls:
      ca_cert: "((loggregator_tls_agent.ca))"
      cert: "((loggregator_tls_agent.certificate))"
      key: "((loggregator_tls_agent.private_key))"
    cache:
      tls:
        ca_cert: "((syslog_agent_api_tls.ca))"
        cert: "((syslog_agent_api_tls.certificate))"
        key: "((syslog_agent_api_tls.private_key))"
        cn: "binding-cache"
    metrics:
      ca_cert: "((syslog_agent_metrics_tls.ca))"
      cert: "((syslog_agent_metrics_tls.certificate))"
      key: "((syslog_agent_metrics_tls.private_key))"
      server_name: syslog_agent_metrics

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
- name: syslog_agent_api_tls
  type: certificate
  options:
    ca: /bosh-<ENV_NAME>/cf/loggregator_ca
    common_name: syslog-agent
    extended_key_usage:
    - client_auth

- name: loggregator_tls_agent
   type: certificate
  options:
    ca: /bosh-<ENV_NAME>/cf/loggregator_ca
    common_name: metron
    extended_key_usage:
    - client_auth
    - server_auth
    
- name: log_cache_syslog_tls
  type: certificate
  options:
    ca: /bosh-<ENV_NAME>/cf/loggregator_ca
    common_name: log-cache.service.cf.internal
    alternative_names:
      - "log-cache.service.cf.internal"
    extended_key_usage:
      - server_auth

- name: syslog_agent_metrics_tls
  type: certificate
  options:
    ca: metric_scraper_ca
    common_name: syslog_agent_metrics
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

#### Logs and metrics

Syslog emits metrics relating to per-app egress, total egress, and total dropped
messages. Metrics and logs for app drains will list an anonymized URL of the
syslog drain, where the user, password, and queries are wiped out for security
reasons.

##### go-loggregator

There is Go client library: [go-loggregator][go-loggregator]. The client
library has several useful patterns along with examples to interact with a
Loggregator Agent.

[go-loggregator]: https://code.cloudfoundry.org/go-loggregator
