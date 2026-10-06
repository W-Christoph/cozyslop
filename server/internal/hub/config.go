package hub

import (
	"net"

	"cozycast/internal/neko"
)

// BuildRoomConfig is shared by configured and registered rooms. Container
// control is attached separately, only for locally configured rooms.
func BuildRoomConfig(name, nekoURL, token, defaultScreen string) (RoomConfig, error) {
	nc, err := neko.NewClient(nekoURL, token)
	if err != nil {
		return RoomConfig{}, err
	}
	host := nc.BaseURL().Hostname()
	return RoomConfig{Name: name, Neko: nc, DefaultScreen: defaultScreen,
		TitleURL: "http://" + net.JoinHostPort(host, "8081") + "/title",
		PlayURL:  "http://" + net.JoinHostPort(host, "8082") + "/play", PlayToken: token}, nil
}
