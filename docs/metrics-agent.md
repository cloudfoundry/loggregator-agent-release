# Metrics Agent

Metrics Agent converts Loggregator metrics from the Forwarder Agent into Prometheus Exposition style metrics and hosts
them on a Prometheus-scrapable endpoint

### Conversion
| Loggregator envelope type                                   | Prometheus type                                                                                                                                                                                   |
|-------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Counter <br> - Integer                                      | Counter <br> - Float                                                                                                                                                                              |
| Gauge <br> - Potentially many distinct metrics per envelope | Gauge(s) <br> - Potentially many per Loggregator envelope                                                                                                                                         |
| Timers (http metrics only)                                  | Histogram <br> - Tags are collapsed into a label set based on the `metrics.whitelisted_timer_tags` property <br> - Values recorded are the difference between the start and stop times in seconds |