package filtering

// Source describes a configured blocklist source.
type Source struct {
	ID       string
	Location string
	Enabled  bool
	Auth     AuthConfig
}

// AuthConfig defines optional authentication for a source.
type AuthConfig struct {
	Username string
	Password string
	Token    string
	Header   string
	Scheme   string
}

// ListConfig defines a blocklist configuration entry.
type ListConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	URL      string `mapstructure:"url"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Token    string `mapstructure:"token"`
	Header   string `mapstructure:"header"`
	Scheme   string `mapstructure:"scheme"`
}

// ParseStats summarises list parsing results.
type ParseStats struct {
	TotalLines int
	Domains    int
	Invalid    int
}
