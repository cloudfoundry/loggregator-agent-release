# Prom Scraper

Prom Scraper can be used to scrape Prometheus Exposition style endpoints on localhost and forward those metrics to Loggregator.

## Usage

### Instrumenting Applications
- Follow steps for instrumenting a Prometheus application. 
[Instructions for a GO App][instrumenting-go-app]

### Configuring scraping
Add a config file matching one of the globs in the `config_globs` property in prom scraper.
  - By default, prom scraper looks for a `prom_scraper_config.yml` file in each job's config directory
  
#### File contents
```yaml
port: Required - port on localhost where metrics endpoint is available
source_id: Optional - the source ID to set on scraped metrics (defaults to infra_job_name) 
instance_id: Optional - the instance ID to set on scraped metrics (defaults to "")
scheme: Optional - the scheme to use when scraping the target metrics endpoint. Either "http" or "https" (defaults to "http")
server_name: Required for HTTPS targets. Prom scraper uses this to set the server name for cert verification despite using localhost to resolve the request.
path: Optional - the path to the metrics endpoint (defaults to "/metrics")
headers: Optional - a map of headers to add to the scrape request
labels: Optional - a map of labels that will be added to all metrics
scrape_interval: Optional - how often to scrape the metrics endpoint. Non-positive numbers cause endpoint to not be scraped.

# NOTE: if you would like to override the use of certificates
# ensure that you include a blob that includes your cert and key files
# in the prom scraper job's additional_volumes property so that
# it will not be blocked from viewing these files by bpm.
ca_path: Optional - path to ca to override the default scraping ca.
client_key_path: Optional - path to a client key to provide to override default mutual tls client key
client_cert_path: Optional - path to a client cert to provide to override default mutual tls client cert
```

#### Example `prom_scraper_config.yml.erb`
```yaml
port: 6061
source_id: "my-job-name"
instance_id: <%= spec.id || spec.index.to_s %>
scheme: https
path: /metrics/prometheus
headers:
  "Authorization": "lemons" 
labels:
  bosh_job: my-cool-bosh-job
```

### Output
- Prom Scraper will scrape the endpoint defined in the scrape config file.
- It will add the `source_id` and `instance_id` values as tags to all metrics
- The scraped metrics will be converted to Loggregator metrics and emitted through Loggregator Agent

#### Deploying Prom Scraper

To deploy prom scraper, add the following jobs to all instance groups and the variables to the variables section.
Any other components deployed on the same VM with scrapable endpoints should have tls certs signed by the `metric_scraper_ca`

**Notes**
- Prom scraper must be deployed alongside a [loggregator-agent](loggregator-agent.md)
- Prom scraper is subject to the same limitations as syslog-agent. As such, certain fields may be truncated
  or cause some consumers to drop the message if they are too long. This includes: the sourceid, which must be less then or equal to 48 characters;
  the instance id, which must be shorter to or equal to 125 characters combined with the source type; Org, space, and app names will be truncated at 63 characters each.
  metrics labels should also be shorter then 32 characters. Some consumers can ignore this limit, but it is not compliant to the syslog rfc. These will not affect metrics-agent users
  or loggregator users, only syslog users (notably log-cache).

```yaml
jobs:
- name: prom_scraper
  release: loggregator-agent
  properties:
    scrape:
      tls:
        ca_cert: "((prom_scraper_scrape_tls.ca))"
        cert: "((prom_scraper_scrape_tls.certificate))"
        key: "((prom_scraper_scrape_tls.private_key))"
    metrics:
      ca_cert: "((prom_scraper_metrics_tls.ca))"
      cert: "((prom_scraper_metrics_tls.certificate))"
      key: "((prom_scraper_metrics_tls.private_key))"
      server_name: prom_scraper_metrics

variables:
- name: prom_scraper_scrape_tls
  type: certificate
  options:
    ca: metric_scraper_ca
    common_name: prom_scraper
    extended_key_usage:
    - client_auth

- name: prom_scraper_metrics_tls
  type: certificate
  options:
    ca: metric_scraper_ca
    common_name: prom_scraper_metrics
    extended_key_usage:
    - server_auth

- name: metric_scraper_ca
  type: certificate
  options:
    is_ca: true
    common_name: metricScraperCA
```

[instrumenting-go-app]: https://prometheus.io/docs/guides/go-application/
[loggregator-agent]:    docs/loggregator-agent.md
