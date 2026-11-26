package hub

import (
	"net"

	"cozycast/internal/neko"
)

// BuildRoomConfig is shared by configured and registered rooms. Container
// control is attached separately, only for locally configured rooms. dial
// (nil = the network) reaches neko and its helpers, e.g. through a tunnel.
func BuildRoomConfig(name, nekoURL, token, defaultScreen string, dial neko.DialFunc) (RoomConfig, error) {
	nc, err := neko.NewClient(nekoURL, token, dial)
	if err != nil {
		return RoomConfig{}, err
	}
	host := nc.BaseURL().Hostname()
	return RoomConfig{Name: name, Neko: nc, DefaultScreen: defaultScreen,
		TitleURL:  "http://" + net.JoinHostPort(host, "8081") + "/title",
		MemoryURL: "http://" + net.JoinHostPort(host, "8081") + "/memory",
		PlayURL:   "http://" + net.JoinHostPort(host, "8082") + "/play", PlayToken: token,
		StatusURL:    "http://" + net.JoinHostPort(host, "8082") + "/status",
		OpenURL:      "http://" + net.JoinHostPort(host, "8082") + "/open",
		HibernateURL: "http://" + net.JoinHostPort(host, "8083")}, nil
}
