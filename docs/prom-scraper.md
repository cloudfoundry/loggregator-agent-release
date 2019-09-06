# Prom Scraper

Prom Scraper can be used to scrape Prometheus Exposition style endpoints on localhost and forward those metrics to Loggregator.

## Usage

### Instrumenting Applications
- Follow steps for instrumenting a Prometheus application. 
[Instructions for a GO App][instrumenting-go-app]

### Configuring scraping
Add a config file matching one of the globs in the `config_globs` property in prom scraper.
  - By default, prom scraper looks for a `prom-scraper-config.yml` file in each job's config directory
  
#### File contents
```yaml
port: Required - port on localhost where metrics endpoint is available
source_id: Optional - the source ID to set on scraped metrics (defaults to infra_job_name) 
instance_id: Optional - the instance ID to set on scraped metrics (defaults to "")
scheme: Optional - the scheme to use when scraping the target metrics endpoint. Either "http" or "https" (defaults to "http")
path: Optional - the path to the metrics endpoint (defaults to "/metrics")
headers: Optional - a map of headers to add to the scrape request
labels: Optional - a map of labels that will be added to all metrics
```

#### Example `prom-scraper-config.yml.erb`
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

[instrumenting-go-app]: https://prometheus.io/docs/guides/go-application/
