package syslog

import (
	"fmt"
	"strconv"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
)

const RFC5424TimeOffsetNum = "2006-01-02T15:04:05.999999-07:00"

func ToRFC5424(env *loggregator_v2.Envelope, hostname, appID string) ([][]byte, error) {
	if len(hostname) > 255 {
		return nil, invalidValue("Hostname", hostname)
	}

	if len(appID) > 48 {
		return nil, invalidValue("AppName", appID)
	}

	if len(env.InstanceId) > 128 {
		return nil, invalidValue("AppName", appID)
	}

	switch env.GetMessage().(type) {
	case *loggregator_v2.Envelope_Log:
		return [][]byte{
			toRFC5424LogMessage(env, hostname, appID),
		}, nil
	case *loggregator_v2.Envelope_Gauge:
		return toRFC5424GaugeMessage(env, hostname, appID), nil
	case *loggregator_v2.Envelope_Counter:
		return [][]byte{
			toRFC5424CounterMessage(env, hostname, appID),
		}, nil
	default:
		return nil, nil
	}
}

func invalidValue(property, value string) error {
	return fmt.Errorf("Invalid value \"%s\" for property %s \n", value, property)
}

func toRFC5424CounterMessage(env *loggregator_v2.Envelope, hostname, appID string) []byte {
	priority := "14"
	ts := time.Unix(0, env.GetTimestamp()).UTC().Format(RFC5424TimeOffsetNum)
	hostname = nilify(hostname)
	appID = nilify(appID)
	pid := "[" + env.InstanceId + "]"
	sd := `[counter@47450 name="` + env.GetCounter().GetName() + `" total="` + strconv.FormatUint(env.GetCounter().GetTotal(), 10) + `" delta="` + strconv.FormatUint(env.GetCounter().GetDelta(), 10) + `"]`

	counter := make([]byte, 0, 20+len(priority)+len(ts)+len(hostname)+len(appID)+len(pid)+len(sd))
	counter = append(counter, []byte("<"+priority+">1 ")...)
	counter = append(counter, []byte(ts+" ")...)
	counter = append(counter, []byte(hostname+" ")...)
	counter = append(counter, []byte(appID+" ")...)
	counter = append(counter, []byte(pid+" - ")...)
	counter = append(counter, []byte(sd+" \n")...)

	return counter
}

func toRFC5424GaugeMessage(env *loggregator_v2.Envelope, hostname, appID string) [][]byte {
	gauges := make([][]byte, 0, 5)
	ts := time.Unix(0, env.GetTimestamp()).UTC().Format(RFC5424TimeOffsetNum)
	pid := "[" + env.InstanceId + "]"
	priority := "14"
	hostname = nilify(hostname)
	appID = nilify(appID)

	for name, g := range env.GetGauge().GetMetrics() {
		sd := `[gauge@47450 name="` + name + `" value="` + strconv.FormatFloat(g.GetValue(), 'g', -1, 64) + `" unit="` + g.GetUnit() + `"]`

		gauge := make([]byte, 0, 20+len(priority)+len(ts)+len(hostname)+len(appID)+len(pid)+len(sd))
		gauge = append(gauge, []byte("<"+priority+">1 ")...)
		gauge = append(gauge, []byte(ts+" ")...)
		gauge = append(gauge, []byte(hostname+" ")...)
		gauge = append(gauge, []byte(appID+" ")...)
		gauge = append(gauge, []byte(pid+" - ")...)
		gauge = append(gauge, []byte(sd+" \n")...)
		gauges = append(gauges, gauge)
	}

	return gauges
}

func toRFC5424LogMessage(env *loggregator_v2.Envelope, hostname, appID string) []byte {
	priority := genPriority(env.GetLog().Type)
	ts := time.Unix(0, env.GetTimestamp()).UTC().Format(RFC5424TimeOffsetNum)
	hostname = nilify(hostname)
	appID = nilify(appID)
	pid := nilify(generateProcessID(
		env.Tags["source_type"],
		env.InstanceId,
	))
	msg := appendNewline(removeNulls(env.GetLog().Payload))

	tmp := make([]byte, 0, 20+len(priority)+len(ts)+len(hostname)+len(appID)+len(pid)+len(msg))
	tmp = append(tmp, []byte("<"+priority+">1 ")...)
	tmp = append(tmp, []byte(ts+" ")...)
	tmp = append(tmp, []byte(hostname+" ")...)
	tmp = append(tmp, []byte(appID+" ")...)
	tmp = append(tmp, []byte(pid+" ")...)
	tmp = append(tmp, []byte("- - ")...)
	tmp = append(tmp, msg...)

	return tmp
}

func genPriority(logType loggregator_v2.Log_Type) string {
	switch logType {
	case loggregator_v2.Log_OUT:
		return "14"
	case loggregator_v2.Log_ERR:
		return "11"
	default:
		return "-1"
	}
}

func nilify(x string) string {
	if x == "" {
		return "-"
	}
	return x
}
