package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the configuration for the controller
type Config struct {
	// Kubeconfig path
	KubeConfig string

	// Metrics server configuration
	MetricsAddr string

	// Leader election configuration
	LeaderElection LeaderElectionConfig

	// Controller configuration
	Controller ControllerConfig

	// Logging configuration
	Logging LoggingConfig
}

// LeaderElectionConfig holds leader election settings
type LeaderElectionConfig struct {
	Enabled       bool
	LeaseDuration time.Duration
	RenewDeadline time.Duration
	RetryPeriod   time.Duration
	LockName      string
	LockNamespace string
}

// ControllerConfig holds controller-specific settings
type ControllerConfig struct {
	// Resync period for informers
	ResyncPeriod time.Duration

	// Worker pool sizes
	PodWorkers  int
	PCMCWorkers int

	// Queue settings
	MaxRetries int

	// Reconciliation settings
	ReconciliationTimeout time.Duration
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level      string
	Format     string
	JSONFormat bool
	Debug      bool
}

// NewConfig creates a new configuration with default values
func NewConfig() *Config {
	return &Config{
		KubeConfig:  "",
		MetricsAddr: ":8080",
		LeaderElection: LeaderElectionConfig{
			Enabled:       true,
			LeaseDuration: 15 * time.Second,
			RenewDeadline: 10 * time.Second,
			RetryPeriod:   2 * time.Second,
			LockName:      "podconfigmap-controller-lock",
			LockNamespace: "default",
		},
		Controller: ControllerConfig{
			ResyncPeriod:          10 * time.Minute,
			PodWorkers:            1,
			PCMCWorkers:           1,
			MaxRetries:            5,
			ReconciliationTimeout: 30 * time.Second,
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "text",
			JSONFormat: false,
			Debug:      false,
		},
	}
}

// LoadFromEnvironment loads configuration from environment variables
func (c *Config) LoadFromEnvironment() error {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		c.KubeConfig = kubeconfig
	}

	if metricsAddr := os.Getenv("METRICS_ADDR"); metricsAddr != "" {
		c.MetricsAddr = metricsAddr
	}

	// Leader election settings
	if enabled := os.Getenv("LEADER_ELECTION_ENABLED"); enabled != "" {
		if val, err := strconv.ParseBool(enabled); err != nil {
			return fmt.Errorf("invalid LEADER_ELECTION_ENABLED value: %w", err)
		} else {
			c.LeaderElection.Enabled = val
		}
	}

	if leaseDuration := os.Getenv("LEADER_ELECTION_LEASE_DURATION"); leaseDuration != "" {
		if val, err := time.ParseDuration(leaseDuration); err != nil {
			return fmt.Errorf("invalid LEADER_ELECTION_LEASE_DURATION value: %w", err)
		} else {
			c.LeaderElection.LeaseDuration = val
		}
	}

	if renewDeadline := os.Getenv("LEADER_ELECTION_RENEW_DEADLINE"); renewDeadline != "" {
		if val, err := time.ParseDuration(renewDeadline); err != nil {
			return fmt.Errorf("invalid LEADER_ELECTION_RENEW_DEADLINE value: %w", err)
		} else {
			c.LeaderElection.RenewDeadline = val
		}
	}

	if retryPeriod := os.Getenv("LEADER_ELECTION_RETRY_PERIOD"); retryPeriod != "" {
		if val, err := time.ParseDuration(retryPeriod); err != nil {
			return fmt.Errorf("invalid LEADER_ELECTION_RETRY_PERIOD value: %w", err)
		} else {
			c.LeaderElection.RetryPeriod = val
		}
	}

	if lockName := os.Getenv("LEADER_ELECTION_LOCK_NAME"); lockName != "" {
		c.LeaderElection.LockName = lockName
	}

	if lockNamespace := os.Getenv("LEADER_ELECTION_LOCK_NAMESPACE"); lockNamespace != "" {
		c.LeaderElection.LockNamespace = lockNamespace
	} else if podNamespace := os.Getenv("POD_NAMESPACE"); podNamespace != "" {
		c.LeaderElection.LockNamespace = podNamespace
	}

	// Controller settings
	if resyncPeriod := os.Getenv("CONTROLLER_RESYNC_PERIOD"); resyncPeriod != "" {
		if val, err := time.ParseDuration(resyncPeriod); err != nil {
			return fmt.Errorf("invalid CONTROLLER_RESYNC_PERIOD value: %w", err)
		} else {
			c.Controller.ResyncPeriod = val
		}
	}

	if podWorkers := os.Getenv("CONTROLLER_POD_WORKERS"); podWorkers != "" {
		if val, err := strconv.Atoi(podWorkers); err != nil {
			return fmt.Errorf("invalid CONTROLLER_POD_WORKERS value: %w", err)
		} else {
			c.Controller.PodWorkers = val
		}
	}

	if pcmcWorkers := os.Getenv("CONTROLLER_PCMC_WORKERS"); pcmcWorkers != "" {
		if val, err := strconv.Atoi(pcmcWorkers); err != nil {
			return fmt.Errorf("invalid CONTROLLER_PCMC_WORKERS value: %w", err)
		} else {
			c.Controller.PCMCWorkers = val
		}
	}

	if maxRetries := os.Getenv("CONTROLLER_MAX_RETRIES"); maxRetries != "" {
		if val, err := strconv.Atoi(maxRetries); err != nil {
			return fmt.Errorf("invalid CONTROLLER_MAX_RETRIES value: %w", err)
		} else {
			c.Controller.MaxRetries = val
		}
	}

	if reconciliationTimeout := os.Getenv("CONTROLLER_RECONCILIATION_TIMEOUT"); reconciliationTimeout != "" {
		if val, err := time.ParseDuration(reconciliationTimeout); err != nil {
			return fmt.Errorf("invalid CONTROLLER_RECONCILIATION_TIMEOUT value: %w", err)
		} else {
			c.Controller.ReconciliationTimeout = val
		}
	}

	// Logging settings
	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		c.Logging.Level = logLevel
	}

	if logFormat := os.Getenv("LOG_FORMAT"); logFormat != "" {
		c.Logging.Format = logFormat
	}

	if jsonFormat := os.Getenv("LOG_JSON_FORMAT"); jsonFormat != "" {
		if val, err := strconv.ParseBool(jsonFormat); err != nil {
			return fmt.Errorf("invalid LOG_JSON_FORMAT value: %w", err)
		} else {
			c.Logging.JSONFormat = val
		}
	}

	if debug := os.Getenv("DEBUG"); debug != "" {
		if val, err := strconv.ParseBool(debug); err != nil {
			return fmt.Errorf("invalid DEBUG value: %w", err)
		} else {
			c.Logging.Debug = val
		}
	}

	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.MetricsAddr == "" {
		return fmt.Errorf("metrics address cannot be empty")
	}

	if c.LeaderElection.Enabled {
		if c.LeaderElection.LeaseDuration <= 0 {
			return fmt.Errorf("leader election lease duration must be positive")
		}

		if c.LeaderElection.RenewDeadline <= 0 {
			return fmt.Errorf("leader election renew deadline must be positive")
		}

		if c.LeaderElection.RetryPeriod <= 0 {
			return fmt.Errorf("leader election retry period must be positive")
		}

		if c.LeaderElection.RenewDeadline >= c.LeaderElection.LeaseDuration {
			return fmt.Errorf("leader election renew deadline must be less than lease duration")
		}

		if c.LeaderElection.LockName == "" {
			return fmt.Errorf("leader election lock name cannot be empty")
		}

		if c.LeaderElection.LockNamespace == "" {
			return fmt.Errorf("leader election lock namespace cannot be empty")
		}
	}

	if c.Controller.ResyncPeriod <= 0 {
		return fmt.Errorf("controller resync period must be positive")
	}

	if c.Controller.PodWorkers <= 0 {
		return fmt.Errorf("controller pod workers must be positive")
	}

	if c.Controller.PCMCWorkers <= 0 {
		return fmt.Errorf("controller PCMC workers must be positive")
	}

	if c.Controller.MaxRetries < 0 {
		return fmt.Errorf("controller max retries cannot be negative")
	}

	if c.Controller.ReconciliationTimeout <= 0 {
		return fmt.Errorf("controller reconciliation timeout must be positive")
	}

	if c.Logging.Level == "" {
		return fmt.Errorf("logging level cannot be empty")
	}

	return nil
}

// GetLeaderElectionID returns the leader election ID
func (c *Config) GetLeaderElectionID() string {
	if podName := os.Getenv("POD_NAME"); podName != "" {
		return podName
	}

	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}

	return "unknown"
}

// String returns a string representation of the configuration
func (c *Config) String() string {
	return fmt.Sprintf("Config{KubeConfig: %s, MetricsAddr: %s, LeaderElection: %+v, Controller: %+v, Logging: %+v}",
		c.KubeConfig, c.MetricsAddr, c.LeaderElection, c.Controller, c.Logging)
}
