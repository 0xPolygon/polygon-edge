package agent

type Config struct {
	Chain     string
	DataDir   string
	BindAddr  string
	BindPort  int
	Telemetry *Telemetry
}

type Telemetry struct {
	PrometheusPort int
}

func DefaultTelemetry() *Telemetry {
	return &Telemetry{
		PrometheusPort: 8080,
	}
}

func DefaultConfig() *Config {
	return &Config{
		Chain:     "foundation",
		DataDir:   "./test-chain",
		BindAddr:  "0.0.0.0",
		BindPort:  30303,
		Telemetry: DefaultTelemetry(),
	}
}
