# frozen_string_literal: true

require 'rspec'
require 'bosh/template/test'
require_relative 'spec_helper'

describe 'loggr-syslog-binding-cache job' do
  let(:release_dir) { File.join(File.dirname(__FILE__), '..') }
  let(:release) { Bosh::Template::Test::ReleaseDir.new(release_dir) }
  let(:job) { release.job('loggr-syslog-binding-cache') }

  describe 'bpm.yml' do

    links = [
      Bosh::Template::Test::Link.new(
        name: 'cloud_controller',
        instances: [
          Bosh::Template::Test::LinkInstance.new(id: 'a-b-c-d')
        ]
      )
    ]


    let(:template) { job.template('config/bpm.yml') }

    it 'contains the tls config for aggregate drains' do
      properties = {
        'api' => {
          'tls' => {
            'cn' => 'CN'
          }
        },
        'tls' => {
          'cn' => 'CN'
        },
        'aggregate_drain_certificates' => "aggregate_drain_certificates",
        'external_port' => 8888
      }
      bpm_yml = YAML.safe_load(template.render(properties, consumes: links))
      env = bpm_process(bpm_yml, 0)['env']
      expect(env).to include('AGGREGATE_DRAIN_CERTIFICATES')
      expect(env.fetch("AGGREGATE_DRAIN_CERTIFICATES")).to eq("aggregate_drain_certificates")
    end
  end
end
