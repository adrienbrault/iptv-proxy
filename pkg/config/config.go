package config

import (
	"net/url"

	"github.com/jamesnetherton/m3u"
)

// HostConfiguration containt host infos
type HostConfiguration struct {
	Hostname string
	Port     int64
}

// ProxyConfig Contain original m3u playlist and HostConfiguration
type ProxyConfig struct {
	Playlist   *m3u.Playlist
	HostConfig *HostConfiguration
	RemoteURL  *url.URL
	//XXX Very unsafe
	User, Password string
}
