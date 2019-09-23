#!/bin/bash

set -eo pipefail

function main() {
  job=$1

  if [[ -z ${OVERRIDE_ENDPOINT} ]]; then
    validate
    find_job
    construct_endpoint
    get_metrics --resolve "${server_name}:${port}:127.0.0.1"
  else
    endpoint=${OVERRIDE_ENDPOINT}
    get_metrics -k
  fi
}

function validate {
    if [[ "$job" = "-h" ]] || [[ "$job" = "--help" ]] || [[ -z "$job" ]]; then
      print_usage
      exit 1
    fi
}

function print_usage {
    printf "usage:\n\n./get-metrics.sh <job_name>\nOVERRIDE_ENDPOINT='https://localhost:8888/metrics' ./get-metrics.sh"
}

function find_job {
    if [ ! -d /var/vcap/jobs/$job ] ; then
    printf "job: ${job} not found \n"

    similar_job=$(ls /var/vcap/jobs | grep ${job})
    if [ ! -z similar_job ]; then
      printf "getting metrics for similar job instead: ${similar_job}\n"
    fi

    job=$similar_job
  fi
}

function construct_endpoint {
  scheme=$(cat /var/vcap/jobs/$job/config/prom_scraper_config.yml | grep scheme | awk '{print $2}')
  port=$(cat /var/vcap/jobs/$job/config/prom_scraper_config.yml | grep port | awk '{print $2}')
  server_name=$(cat /var/vcap/jobs/$job/config/prom_scraper_config.yml | grep server_name | awk '{print $2}')

  endpoint="${scheme}://${server_name}:$port/metrics"
}

function get_metrics {
  curl ${endpoint} \
  --cacert /var/vcap/jobs/prom_scraper/config/certs/scrape_ca.crt \
  --cert /var/vcap/jobs/prom_scraper/config/certs/scrape.crt \
  --key /var/vcap/jobs/prom_scraper/config/certs/scrape.key \
  $@
}

main $1