# frozen_string_literal: true

require 'bosh/template/renderer'
require 'yaml'

# TemplateHelpers module functions
module TemplateHelpers
  def bpm_process(bpm_yml, index)
    bpm_yml['processes'][index]
  end
end
