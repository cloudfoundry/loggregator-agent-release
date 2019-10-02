module code.cloudfoundry.org/loggregator-agent

go 1.12

require (
	code.cloudfoundry.org/go-batching v0.0.0-20171020220229-924d2a9b48ac
	code.cloudfoundry.org/go-diodes v0.0.0-20190809170250-f77fb823c7ee
	code.cloudfoundry.org/go-envstruct v1.5.0
	code.cloudfoundry.org/go-loggregator v0.0.0-20190809213911-969cb33bee6a // pinned
	code.cloudfoundry.org/go-metric-registry v0.0.0-20190930200807-cfc6a44b372f
	code.cloudfoundry.org/rfc5424 v0.0.0-20180905210152-236a6d29298a
	code.cloudfoundry.org/tlsconfig v0.0.0-20190710180242-462f72de1106
	github.com/apoydence/eachers v0.0.0-20181020210610-23942921fe77
	github.com/cloudfoundry/dropsonde v1.0.0
	github.com/cloudfoundry/sonde-go v0.0.0-20171206171820-b33733203bb4
	github.com/gogo/protobuf v1.3.0
	github.com/golang/protobuf v1.3.2
	github.com/google/go-cmp v0.3.1 // indirect
	github.com/gorilla/mux v1.7.3
	github.com/klauspost/compress v1.8.5 // indirect
	github.com/onsi/ginkgo v1.10.2
	github.com/onsi/gomega v1.7.0
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4
	github.com/prometheus/common v0.7.0
	github.com/prometheus/procfs v0.0.5 // indirect
	github.com/square/certstrap v1.2.0 // indirect
	github.com/valyala/fasthttp v1.5.0
	golang.org/x/net v0.0.0-20191002035440-2ec189313ef0
	golang.org/x/sys v0.0.0-20191002091554-b397fe3ad8ed // indirect
	golang.org/x/text v0.3.2 // indirect
	google.golang.org/genproto v0.0.0-20190927181202-20e1ac93f88c // indirect
	google.golang.org/grpc v1.24.0
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v2 v2.2.4
)

replace golang.org/x/sys => golang.org/x/sys v0.0.0-20190801041406-cbf593c0f2f3
