package bindings_test

import (
	"errors"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Drain Param Config", func() {
	It("sets OmitMetadata to false if the drain doesn't contain 'disable-metadata=true'", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain"}},
		}
		f := newStubFetcher(bs, nil)
		dp := bindings.NewDrainParamParser(f, true)

		configedBindings, _ := dp.FetchBindings()
		Expect(configedBindings[0].OmitMetadata).To(BeFalse())
	})

	It("sets OmitMetadata to true if the drain contains 'disable-metadata=true'", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain?disable-metadata=true"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?omit-metadata=true"}},
		}
		f := newStubFetcher(bs, nil)
		dp := bindings.NewDrainParamParser(f, true)

		configedBindings, _ := dp.FetchBindings()
		Expect(configedBindings[0].OmitMetadata).To(BeTrue())
		Expect(configedBindings[1].OmitMetadata).To(BeTrue())
	})

	It("sets OmitMetadata to true if global flag is off", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain"}},
		}
		f := newStubFetcher(bs, nil)
		dp := bindings.NewDrainParamParser(f, false)

		configedBindings, _ := dp.FetchBindings()
		Expect(configedBindings[0].OmitMetadata).To(BeTrue())
	})

	It("sets OmitMetadata to false if global flag is off, but drain enables it", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain?disable-metadata=false"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?omit-metadata=false"}},
		}
		f := newStubFetcher(bs, nil)
		dp := bindings.NewDrainParamParser(f, false)

		configedBindings, _ := dp.FetchBindings()
		Expect(configedBindings[0].OmitMetadata).To(BeFalse())
		Expect(configedBindings[1].OmitMetadata).To(BeFalse())
	})

	It("sets internal tls to true if the drain contains 'ssl-strict-internal=true'", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain?ssl-strict-internal=true"}},
		}
		f := newStubFetcher(bs, nil)
		dp := bindings.NewDrainParamParser(f, true)

		configedBindings, _ := dp.FetchBindings()
		Expect(configedBindings[0].InternalTls).To(BeTrue())
	})

	It("sets drain data appropriately'", func() {
		testCases := []struct {
			name     string
			url      string
			expected syslog.DrainData
		}{
			{
				name:     "no drain-data parameter defaults to logs",
				url:      "https://test.org/drain",
				expected: syslog.LOGS,
			},
			{
				name:     "drain-data=logs",
				url:      "https://test.org/drain?drain-data=logs",
				expected: syslog.LOGS,
			},
			{
				name:     "drain-data=metrics",
				url:      "https://test.org/drain?drain-data=metrics",
				expected: syslog.METRICS,
			},
			{
				name:     "drain-data=traces",
				url:      "https://test.org/drain?drain-data=traces",
				expected: syslog.TRACES,
			},
			{
				name:     "drain-data=all",
				url:      "https://test.org/drain?drain-data=all",
				expected: syslog.ALL,
			},
		}

		for _, tc := range testCases {
			By(tc.name)
			bs := []syslog.Binding{
				{Drain: syslog.Drain{Url: tc.url}},
			}
			f := newStubFetcher(bs, nil)
			dp := bindings.NewDrainParamParser(f, true)

			configedBindings, _ := dp.FetchBindings()
			Expect(configedBindings[0].DrainData).To(Equal(tc.expected))
		}
	})

	It("sets drain filter appropriately", func() {
		testCases := []struct {
			name     string
			url      string
			expected *syslog.LogTypeSet
		}{
			{
				name:     "empty drain URL defaults to all types",
				url:      "https://test.org/drain",
				expected: NewLogTypeSet(),
			},
			{
				name:     "include-log-types=app",
				url:      "https://test.org/drain?include-log-types=app",
				expected: NewLogTypeSet(syslog.LOG_APP),
			},
			{
				name:     "include-log-types=app,stg,cell",
				url:      "https://test.org/drain?include-log-types=app,stg,cell",
				expected: NewLogTypeSet(syslog.LOG_APP, syslog.LOG_STG, syslog.LOG_CELL),
			},
			{
				name:     "exclude-log-types=rtr,cell,stg",
				url:      "https://test.org/drain?exclude-log-types=rtr,cell,stg",
				expected: NewLogTypeSet(syslog.LOG_API, syslog.LOG_LGR, syslog.LOG_APP, syslog.LOG_SSH),
			},
			{
				name:     "exclude-log-types=rtr",
				url:      "https://test.org/drain?exclude-log-types=rtr",
				expected: NewLogTypeSet(syslog.LOG_API, syslog.LOG_STG, syslog.LOG_LGR, syslog.LOG_APP, syslog.LOG_SSH, syslog.LOG_CELL),
			},
		}

		for _, tc := range testCases {
			By(tc.name)
			bs := []syslog.Binding{
				{Drain: syslog.Drain{Url: tc.url}},
			}
			f := newStubFetcher(bs, nil)
			dp := bindings.NewDrainParamParser(f, true)

			configedBindings, _ := dp.FetchBindings()
			Expect(configedBindings[0].LogFilter).To(Equal(tc.expected), "failed for case: %s", tc.name)
		}
	})

	It("sets drain data for old parameter appropriately'", func() {
		testCases := []struct {
			name     string
			url      string
			expected syslog.DrainData
		}{
			{
				name:     "drain-type=metrics",
				url:      "https://test.org/drain?drain-type=metrics",
				expected: syslog.METRICS,
			},
			{
				name:     "drain-type=logs",
				url:      "https://test.org/drain?drain-type=logs",
				expected: syslog.LOGS_NO_EVENTS,
			},
			{
				name:     "no drain-type parameter",
				url:      "https://test.org/drain",
				expected: syslog.LOGS,
			},
			{
				name:     "drain-type=all",
				url:      "https://test.org/drain?drain-type=all",
				expected: syslog.LOGS_AND_METRICS,
			},
			{
				name:     "include-metrics-deprecated=true",
				url:      "https://test.org/drain?include-metrics-deprecated=true",
				expected: syslog.ALL,
			},
		}

		for _, tc := range testCases {
			By(tc.name)
			bs := []syslog.Binding{
				{Drain: syslog.Drain{Url: tc.url}},
			}
			f := newStubFetcher(bs, nil)
			dp := bindings.NewDrainParamParser(f, true)

			configedBindings, _ := dp.FetchBindings()
			Expect(configedBindings[0].DrainData).To(Equal(tc.expected))
		}
	})

	It("omits bindings with bad Drain URLs", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "   https://leading-spaces-are-invalid"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?disable-metadata=true"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?omit-metadata=true"}},
		}
		f := newStubFetcher(bs, nil)
		dp := bindings.NewDrainParamParser(f, true)

		configedBindings, err := dp.FetchBindings()
		Expect(err).ToNot(HaveOccurred())
		Expect(configedBindings).To(HaveLen(2))
		Expect(configedBindings[0].Drain).To(Equal(syslog.Drain{Url: "https://test.org/drain?disable-metadata=true"}))
		Expect(configedBindings[1].Drain).To(Equal(syslog.Drain{Url: "https://test.org/drain?omit-metadata=true"}))
	})

	It("Returns a error when fetching fails", func() {
		f := newStubFetcher(nil, errors.New("Ahhh an error"))
		dp := bindings.NewDrainParamParser(f, true)

		_, err := dp.FetchBindings()
		Expect(err).To(MatchError("Ahhh an error"))
	})
})

type stubFetcher struct {
	bindings []syslog.Binding
	err      error
}

func newStubFetcher(bs []syslog.Binding, err error) *stubFetcher {
	return &stubFetcher{
		bindings: bs,
		err:      err,
	}
}

func (f *stubFetcher) FetchBindings() ([]syslog.Binding, error) {
	return f.bindings, f.err
}

func (f *stubFetcher) DrainLimit() int {
	return -1
}

func NewLogTypeSet(logTypes ...syslog.LogType) *syslog.LogTypeSet {
	if len(logTypes) == 0 {
		return nil
	}
	set := make(syslog.LogTypeSet, len(logTypes))
	for _, t := range logTypes {
		set[t] = struct{}{}
	}
	return &set
}
