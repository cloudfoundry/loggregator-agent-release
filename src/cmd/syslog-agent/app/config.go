package app

import (
	"fmt"
	"strings"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/config"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings"

	"code.cloudfoundry.org/go-envstruct"
)

// GRPC stores the configuration for the router as a server using a PORT
// with mTLS certs and as a client.
type GRPC struct {
	Port         int      `env:"AGENT_PORT,                     report"`
	CAFile       string   `env:"AGENT_CA_FILE_PATH,   required, report"`
	CertFile     string   `env:"AGENT_CERT_FILE_PATH, required, report"`
	KeyFile      string   `env:"AGENT_KEY_FILE_PATH,  required, report"`
	CipherSuites []string `env:"AGENT_CIPHER_SUITES,            report"`
}

type Cache struct {
	URL             string                   `env:"CACHE_URL,                 report"`
	CAFile          string                   `env:"CACHE_CA_FILE_PATH,        report"`
	CertFile        string                   `env:"CACHE_CERT_FILE_PATH,      report"`
	KeyFile         string                   `env:"CACHE_KEY_FILE_PATH,       report"`
	CommonName      string                   `env:"CACHE_COMMON_NAME,         report"`
	PollingInterval time.Duration            `env:"CACHE_POLLING_INTERVAL,    report"`
	Blacklist       bindings.BlacklistRanges `env:"BLACKLISTED_SYSLOG_RANGES, report"`
}

// Config holds the configuration for the syslog agent
type Config struct {
	UseRFC3339           bool          `env:"USE_RFC3339"`
	BindingsPerAppLimit  int           `env:"BINDING_PER_APP_LIMIT,  report"`
	DrainSkipCertVerify  bool          `env:"DRAIN_SKIP_CERT_VERIFY, report"`
	DrainCipherSuites    string        `env:"DRAIN_CIPHER_SUITES,    report"`
	DrainTrustedCAFile   string        `env:"DRAIN_TRUSTED_CA_FILE,  report"`
	DefaultDrainMetadata bool          `env:"DEFAULT_DRAIN_METADATA, report"`
	IdleDrainTimeout     time.Duration `env:"IDLE_DRAIN_TIMEOUT, report"`

	GRPC          GRPC
	Cache         Cache
	MetricsServer config.MetricsServer

	AggregateConnectionRefreshInterval time.Duration `env:"AGGREGATE_CONNECTION_REFRESH_INTERVAL, report"`
	AggregateDrainURLs                 []string      `env:"AGGREGATE_DRAIN_URLS,                  report"`
}

// LoadConfig will load the configuration for the syslog agent from the
// environment. If loading the config fails for any reason this function will
// panic.
func LoadConfig() Config {
	cfg := Config{
		BindingsPerAppLimit: 5,
		IdleDrainTimeout:    10 * time.Minute,

		Cache: Cache{
			PollingInterval: 1 * time.Minute,
		},
		GRPC: GRPC{
			Port: 3458,
		},
		AggregateConnectionRefreshInterval: 1 * time.Minute,
		DefaultDrainMetadata:               true,
	}
	if err := envstruct.Load(&cfg); err != nil {
		panic(fmt.Sprintf("Failed to load config from environment: %s", err))
	}

	envstruct.WriteReport(&cfg) //nolint:errcheck

	return cfg
}

func (c *Config) processCipherSuites() (*[]uint16, error) {
	cipherMap := map[string]uint16{
		"AES128-SHA256":                           0x003c,
		"AES128-GCM-SHA256":                       0x009c,
		"AES256-GCM-SHA384":                       0x009d,
		"ECDHE-ECDSA-RC4-SHA":                     0xc007,
		"ECDHE-ECDSA-AES128-SHA":                  0xc009,
		"ECDHE-ECDSA-AES256-SHA":                  0xc00a,
		"ECDHE-RSA-RC4-SHA":                       0xc011,
		"ECDHE-RSA-DES-CBC3-SHA":                  0xc012,
		"ECDHE-RSA-AES128-SHA":                    0xc013,
		"ECDHE-RSA-AES256-SHA":                    0xc014,
		"ECDHE-ECDSA-AES128-SHA256":               0xc023,
		"ECDHE-RSA-AES128-SHA256":                 0xc027,
		"ECDHE-RSA-AES128-GCM-SHA256":             0xc02f,
		"ECDHE-ECDSA-AES128-GCM-SHA256":           0xc02b,
		"ECDHE-RSA-AES256-GCM-SHA384":             0xc030,
		"ECDHE-ECDSA-AES256-GCM-SHA384":           0xc02c,
		"ECDHE-RSA-CHACHA20-POLY1305":             0xcca8,
		"ECDHE-ECDSA-CHACHA20-POLY1305":           0xcca9,
		"TLS_RSA_WITH_RC4_128_SHA":                0x0005, // RFC formatted values
		"TLS_RSA_WITH_3DES_EDE_CBC_SHA":           0x000a,
		"TLS_RSA_WITH_AES_128_CBC_SHA":            0x002f,
		"TLS_RSA_WITH_AES_256_CBC_SHA":            0x0035,
		"TLS_RSA_WITH_AES_128_CBC_SHA256":         0x003c,
		"TLS_RSA_WITH_AES_128_GCM_SHA256":         0x009c,
		"TLS_RSA_WITH_AES_256_GCM_SHA384":         0x009d,
		"TLS_ECDHE_ECDSA_WITH_RC4_128_SHA":        0xc007,
		"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA":    0xc009,
		"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA":    0xc00a,
		"TLS_ECDHE_RSA_WITH_RC4_128_SHA":          0xc011,
		"TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA":     0xc012,
		"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA":      0xc013,
		"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA":      0xc014,
		"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256": 0xc023,
		"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256":   0xc027,
		"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":   0xc02f,
		"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256": 0xc02b,
		"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":   0xc030,
		"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384": 0xc02c,
		"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305":    0xcca8,
		"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305":  0xcca9,
	}

	// default to external ciphers. empty string should be errored on the bosh
	// templeting level
	if len(strings.TrimSpace(c.DrainCipherSuites)) == 0 {
		return nil, nil
	}

	ciphers := strings.Split(c.DrainCipherSuites, ":")
	return convertCipherStringToInt(ciphers, cipherMap)
}

func convertCipherStringToInt(cipherStrs []string, cipherMap map[string]uint16) (*[]uint16, error) {
	ciphers := []uint16{}
	for _, cipher := range cipherStrs {
		if val, ok := cipherMap[cipher]; ok {
			ciphers = append(ciphers, val)
		} else {
			var supportedCipherSuites = []string{}
			for key := range cipherMap {
				supportedCipherSuites = append(supportedCipherSuites, key)
			}
			errMsg := fmt.Sprintf("Invalid cipher string configuration: %s, please choose from %v", cipher, supportedCipherSuites)
			return nil, fmt.Errorf(errMsg)
		}
	}

	return &ciphers, nil
}
