package ssh

import (
	"os"
	"strconv"

	"github.com/kevinburke/ssh_config"
)

// mergeSSHConfig fills jump host settings from ~/.ssh/config when not set in the URI.
func mergeSSHConfig(cfg *jumpConfig) {
	if cfg.host == "" {
		return
	}
	alias := cfg.host
	if hostname := ssh_config.Get(alias, "HostName"); hostname != "" {
		cfg.host = hostname
	}
	if cfg.username == "" {
		cfg.username = ssh_config.Get(alias, "User")
	}
	if cfg.password == "" {
		cfg.password = ssh_config.Get(alias, "Password")
	}
	if p := ssh_config.Get(alias, "Port"); p != "" && cfg.port == 22 {
		if n, err := strconv.Atoi(p); err == nil {
			cfg.port = n
		}
	}
	if cfg.username == "" {
		cfg.username = os.Getenv("USER")
	}
}
