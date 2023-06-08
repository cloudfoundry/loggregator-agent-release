package app_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/sirupsen/logrus"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/otel-collector-manager/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/otel-collector-manager/app/appfakes"
)

var _ = Describe("Manager", func() {
	var (
		m      *app.Manager
		c      *appfakes.FakeChangeGetter
		w      *appfakes.FakeConfigWriter
		r      *appfakes.FakeRunner
		a      *appfakes.FakeConfigApplier
		l      *logrus.Logger
		ctx    context.Context
		cancel context.CancelFunc
		stopCh chan struct{}
	)

	BeforeEach(func() {
		c = &appfakes.FakeChangeGetter{}
		c.ChangedReturns(true)
		w = &appfakes.FakeConfigWriter{}
		r = &appfakes.FakeRunner{}
		a = &appfakes.FakeConfigApplier{}
		l = logrus.New()
		l.SetOutput(GinkgoWriter)
		m = app.NewManager(c, 10*time.Millisecond, w, r, a, l)
		ctx, cancel = context.WithCancel(context.Background())
		stopCh = make(chan struct{}, 1)
	})

	AfterEach(func() {
		cancel()
	})

	Describe("Run", func() {
		It("removes an existing pid file", func() {
			go m.Run(ctx, stopCh)
			Eventually(r.RemovePidFileCallCount).Should(Equal(1))
		})
		It("gets the OTel exporter configuration from the server", func() {
			go m.Run(ctx, stopCh)
			Eventually(c.GetCallCount).Should(BeNumerically(">=", 1))
		})

		It("writes the OTel exporter configuration out every time it changes", func() {
			go m.Run(ctx, stopCh)
			Eventually(w.WriteCallCount).Should(BeNumerically(">=", 2))
		})

		Context("when the configuration has not changed after first retrieved", func() {
			BeforeEach(func() {
				c.ChangedStub = func() bool {
					return c.ChangedCallCount() == 1
				}
				r.IsRunningReturns(true)
			})

			It("does not write the OTel exporter configuration out again", func() {
				go m.Run(ctx, stopCh)
				Eventually(w.WriteCallCount).Should(Equal(1))
				Consistently(w.WriteCallCount).Should(Equal(1))
			})

			It("does not re-apply the configuration", func() {
				go m.Run(ctx, stopCh)
				Eventually(a.ApplyCallCount).Should(Equal(1))
				Consistently(a.ApplyCallCount).Should(Equal(1))
			})
		})

		Context("when the collector is not running", func() {
			BeforeEach(func() {
				r.IsRunningReturns(false)
			})
			It("starts the collector", func() {
				go m.Run(ctx, stopCh)
				Eventually(r.StartCallCount).Should(BeNumerically(">=", 1))
				Consistently(a.ApplyCallCount).Should(BeZero())
			})
			Context("and starting the collector errors", func() {
				var o *gbytes.Buffer

				BeforeEach(func() {
					o = gbytes.NewBuffer()
					l.SetOutput(o)
					r.StartReturns(errors.New("some-error"))
				})

				It("logs the error", func() {
					go m.Run(ctx, stopCh)
					Eventually(o).Should(gbytes.Say("Failed to run otel collector.*some-error"))
				})
			})
			Context("and the configuration has not changed", func() {
				BeforeEach(func() {
					c.ChangedStub = func() bool {
						return c.ChangedCallCount() == 1
					}
				})
				It("starts the collector", func() {
					go m.Run(ctx, stopCh)
					Eventually(r.StartCallCount).Should(BeNumerically(">=", 2))
					Consistently(a.ApplyCallCount).Should(BeZero())
				})
			})
		})

		Context("when the collector is already running", func() {
			BeforeEach(func() {
				r.IsRunningReturns(true)
			})
			It("applies the OTel exporter configuration", func() {
				go m.Run(ctx, stopCh)
				Consistently(r.StartCallCount).Should(BeZero())
				Eventually(a.ApplyCallCount).Should(BeNumerically(">=", 1))
			})
			Context("and the context is cancelled", func() {
				It("calls Stop on the runner", func() {
					go m.Run(ctx, stopCh)
					cancel()

					Eventually(r.StopCallCount).Should(BeNumerically(">=", 1))
				})
				It("closes the stop channel to indicate it is done", func() {
					go m.Run(ctx, stopCh)
					cancel()

					Eventually(stopCh).Should(BeClosed())
				})

			})
		})

		Context("when the getter returns an error", func() {
			var o *gbytes.Buffer

			BeforeEach(func() {
				o = gbytes.NewBuffer()
				l.SetOutput(o)
				c.GetReturns(nil, errors.New("some-error"))
			})

			It("logs the error", func() {
				go m.Run(ctx, stopCh)
				Eventually(o).Should(gbytes.Say("Failed to retrieve exporter configuration.*some-error"))
			})

			It("does not write the config", func() {
				go m.Run(ctx, stopCh)
				Consistently(w.WriteCallCount).Should(BeZero())
			})

			It("does not apply the config", func() {
				go m.Run(ctx, stopCh)
				Consistently(a.ApplyCallCount).Should(BeZero())
			})
		})

		Context("when writing the config out errors", func() {
			var o *gbytes.Buffer

			BeforeEach(func() {
				o = gbytes.NewBuffer()
				l.SetOutput(o)
				w.WriteReturns(errors.New("some-error"))
			})

			It("logs the error", func() {
				go m.Run(ctx, stopCh)
				Eventually(o).Should(gbytes.Say("Failed to write otel collector configuration.*some-error"))
			})

			It("does not apply the config", func() {
				go m.Run(ctx, stopCh)
				Consistently(a.ApplyCallCount).Should(BeZero())
			})
		})

		Context("when applying the config errors", func() {
			var o *gbytes.Buffer

			BeforeEach(func() {
				o = gbytes.NewBuffer()
				l.SetOutput(o)
				r.IsRunningReturns(true)
				a.ApplyReturns(errors.New("some-error"))
			})

			It("logs the error", func() {
				go m.Run(ctx, stopCh)
				Eventually(o).Should(gbytes.Say("Failed to apply otel collector configuration.*some-error"))
			})
		})
	})
})
