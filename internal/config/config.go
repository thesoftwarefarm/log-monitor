package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Defaults Defaults       `yaml:"defaults"`
	Servers  []ServerConfig `yaml:"servers"`
}

type Defaults struct {
	SSHKey       string        `yaml:"ssh_key"`
	SSHPort      int           `yaml:"ssh_port"`
	TailLines    int           `yaml:"tail_lines"`
	PollInterval time.Duration `yaml:"poll_interval"`
}

type LogFolder struct {
	Name         string   `yaml:"name"`
	Path         string   `yaml:"path"`
	FilePatterns []string `yaml:"file_patterns"`
}

type ServerConfig struct {
	Name         string      `yaml:"name"`
	Host         string      `yaml:"host"`
	Port         int         `yaml:"port"`
	User         string      `yaml:"user"`
	Auth         AuthConfig  `yaml:"auth"`
	LogPath      string      `yaml:"log_path"`
	FilePatterns []string    `yaml:"file_patterns"`
	LogFolders   []LogFolder `yaml:"log_folders"`
	Sudo         bool        `yaml:"sudo"`
}

// EffectiveFolders returns the list of folders to monitor. If LogFolders is
// set it is returned directly; otherwise a single LogFolder is synthesised
// from the legacy LogPath/FilePatterns fields.
func (s *ServerConfig) EffectiveFolders() []LogFolder {
	if len(s.LogFolders) > 0 {
		return s.LogFolders
	}
	return []LogFolder{{
		Name:         "",
		Path:         s.LogPath,
		FilePatterns: s.FilePatterns,
	}}
}

type AuthConfig struct {
	Method  string `yaml:"method"`  // "key", "password", or "agent"
	KeyPath string `yaml:"key_path"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	d := &cfg.Defaults
	if d.SSHPort == 0 {
		d.SSHPort = 22
	}
	if d.TailLines == 0 {
		d.TailLines = 100
	}
	if d.PollInterval == 0 {
		d.PollInterval = 5 * time.Second
	}
	d.SSHKey = expandTilde(d.SSHKey)

	for i := range cfg.Servers {
		s := &cfg.Servers[i]
		if s.Port == 0 {
			s.Port = d.SSHPort
		}
		if s.Auth.Method == "" {
			if d.SSHKey != "" {
				s.Auth.Method = "key"
			} else {
				s.Auth.Method = "agent"
			}
		}
		if s.Auth.Method == "key" && s.Auth.KeyPath == "" {
			s.Auth.KeyPath = d.SSHKey
		}
		s.Auth.KeyPath = expandTilde(s.Auth.KeyPath)
	}
}

func validate(cfg *Config) error {
	if len(cfg.Servers) == 0 {
		return fmt.Errorf("no servers defined")
	}
	for i, s := range cfg.Servers {
		if s.Host == "" {
			return fmt.Errorf("server %d: host is required", i)
		}
		if s.User == "" {
			return fmt.Errorf("server %d (%s): user is required", i, s.Host)
		}
		hasLogPath := s.LogPath != ""
		hasLogFolders := len(s.LogFolders) > 0
		if hasLogPath && hasLogFolders {
			return fmt.Errorf("server %d (%s): cannot set both log_path and log_folders", i, s.Host)
		}
		if !hasLogPath && !hasLogFolders {
			return fmt.Errorf("server %d (%s): log_path or log_folders is required", i, s.Host)
		}
		if hasLogFolders {
			for j, f := range s.LogFolders {
				if f.Name == "" {
					return fmt.Errorf("server %d (%s): log_folders[%d]: name is required", i, s.Host, j)
				}
				if f.Path == "" {
					return fmt.Errorf("server %d (%s): log_folders[%d]: path is required", i, s.Host, j)
				}
			}
		}
		if s.Name == "" {
			cfg.Servers[i].Name = fmt.Sprintf("%s@%s", s.User, s.Host)
		}
		switch s.Auth.Method {
		case "key", "password", "agent":
		default:
			return fmt.Errorf("server %d (%s): unknown auth method %q", i, s.Host, s.Auth.Method)
		}
	}
	return nil
}

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
