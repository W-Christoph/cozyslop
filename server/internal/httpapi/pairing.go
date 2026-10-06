package httpapi

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"cozycast/internal/config"
	"cozycast/internal/pairing"
	"cozycast/internal/store"
	"cozycast/internal/tunnel"
)

// Pairing: a computer at someone's home asks to run a room, an admin who
// sees the same code accepts it (docs/home-hosting.md, "Pairing").

// pairWait is how long a computer's poll for the answer is held open.
var pairWait = 25 * time.Second

func (s *Server) pairingOff(w http.ResponseWriter) bool {
	if s.pairing == nil {
		writeError(w, http.StatusServiceUnavailable, "This server does not take rooms from other computers (COZYCAST_TUNNEL_PORT is not set).")
		return true
	}
	return false
}

func (s *Server) createPairing(w http.ResponseWriter, r *http.Request) {
	if s.pairingOff(w) {
		return
	}
	ip := s.auth.ClientIP(r)
	if !s.pairLimit.Allow(ip) {
		writeError(w, http.StatusTooManyRequests, "Too many pairing requests. Try again later.")
		return
	}
	var req struct {
		Name    string `json:"name"`
		NodeKey string `json:"nodeKey"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !config.ValidRoomName(req.Name) {
		writeError(w, http.StatusBadRequest, "Room names must contain only letters, digits, underscores or hyphens.")
		return
	}
	key, err := tunnel.ParseKey(req.NodeKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid key.")
		return
	}
	if room, ok := s.pairedRoomByKey(w, r, key); !ok {
		return
	} else if room != "" {
		writeError(w, http.StatusConflict, "This computer is already paired, as room "+room+".")
		return
	}
	pr, secret, err := s.pairing.Create(req.Name, key, ip)
	if errors.Is(err, pairing.ErrTooMany) {
		writeError(w, http.StatusTooManyRequests, "Too many computers are waiting to be accepted. Try again later.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         pr.ID,
		"secret":     secret,
		"hubKey":     s.tunnel.PublicKey().String(),
		"nonce":      base64.StdEncoding.EncodeToString(pr.Nonce),
		"tunnelPort": s.tunnel.Port(),
		"expiresAt":  pr.Expires.UnixMilli(),
	})
}

// pairedRoomByKey is the room a node key is paired as, "" if none.
func (s *Server) pairedRoomByKey(w http.ResponseWriter, r *http.Request, key tunnel.Key) (string, bool) {
	rooms, err := s.store.RegisteredRooms(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return "", false
	}
	for _, room := range rooms {
		if room.NodeKey == key.String() {
			return room.Name, true
		}
	}
	return "", true
}

func (s *Server) pairingStatus(w http.ResponseWriter, r *http.Request) {
	if s.pairingOff(w) {
		return
	}
	secret, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	ctx, cancel := context.WithTimeout(r.Context(), pairWait)
	defer cancel()
	pr, err := s.pairing.Wait(ctx, r.PathValue("id"), secret)
	if errors.Is(err, pairing.ErrUnknown) {
		writeError(w, http.StatusNotFound, "Unknown pairing request.")
		return
	}
	res := map[string]any{"status": pr.State}
	if pr.State == pairing.Accepted {
		room, err := s.store.RegisteredRoom(r.Context(), pr.Room)
		if err != nil || room.NodeKey != pr.NodeKey.String() {
			// Removed (or given to another computer) since.
			writeJSON(w, http.StatusOK, map[string]any{"status": pairing.Rejected})
			return
		}
		res["room"] = room.Name
		res["address"] = room.TunnelAddress
		res["hubAddress"] = s.tunnel.Address().String()
		res["network"] = s.tunnel.Network().String()
	}
	writeJSON(w, http.StatusOK, res)
}

type pairingView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	IP        string `json:"ip"`
	CreatedAt int64  `json:"createdAt"` // unix ms
	ExpiresAt int64  `json:"expiresAt"`
}

func (s *Server) adminListPairing(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	list := []pairingView{}
	if s.pairing != nil {
		for _, pr := range s.pairing.Pending() {
			list = append(list, pairingView{pr.ID, pr.Name, pr.Code, pr.IP, pr.Created.UnixMilli(), pr.Expires.UnixMilli()})
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) adminRejectPairing(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil || s.pairingOff(w) {
		return
	}
	if err := s.pairing.Reject(r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, "This request is no longer waiting.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// adminAcceptPairing makes the computer a room: a new one ({"name"}), or the
// new computer of a paired room ({"replace"}), which keeps its address,
// token, chat, settings and permissions.
func (s *Server) adminAcceptPairing(w http.ResponseWriter, r *http.Request) {
	actor := s.requireAdmin(w, r)
	if actor == nil || s.pairingOff(w) {
		return
	}
	var req struct {
		Name    string `json:"name"`
		Replace string `json:"replace"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if (req.Name == "") == (req.Replace == "") {
		writeError(w, http.StatusBadRequest, "Give either a new room name or the room to replace.")
		return
	}
	pr, err := s.pairing.Claim(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "This request is no longer waiting.")
		return
	}
	s.roomMu.Lock()
	defer s.roomMu.Unlock()
	// Complete DB/runtime changes even if the admin disconnects midway.
	ctx := context.WithoutCancel(r.Context())
	var room string
	if req.Replace != "" {
		room, err = s.replaceNode(ctx, w, req.Replace, pr.NodeKey)
	} else {
		room, err = s.pairNewRoom(ctx, w, req.Name, pr.NodeKey, actor.ID)
	}
	if err != nil {
		s.pairing.Release(pr.ID)
		if !errors.Is(err, errAnswered) {
			s.internalError(w, r, err)
		}
		return
	}
	s.pairing.Finish(pr.ID, room)
	s.log.Info("computer paired", "room", room, "by", actor.Username, "replaced", req.Replace != "")
	writeJSON(w, http.StatusOK, toAdminRoom(s.hub.Room(room)))
}

// errAnswered: the handler already wrote the response.
var errAnswered = errors.New("answered")

// pairNewRoom creates a paired room. Caller holds roomMu.
func (s *Server) pairNewRoom(ctx context.Context, w http.ResponseWriter, name string, key tunnel.Key, by int64) (string, error) {
	if !config.ValidRoomName(name) {
		writeError(w, http.StatusBadRequest, "Room names must contain only letters, digits, underscores or hyphens.")
		return "", errAnswered
	}
	if s.hub.Room(name) != nil {
		writeError(w, http.StatusConflict, "That room already exists.")
		return "", errAnswered
	}
	rooms, err := s.store.RegisteredRooms(ctx)
	if err != nil {
		return "", err
	}
	var used []netip.Addr
	for _, room := range rooms {
		if a, err := netip.ParseAddr(room.TunnelAddress); err == nil {
			used = append(used, a)
		}
	}
	addr, err := tunnel.NextAddress(s.tunnel.Network(), s.tunnel.Address(), used)
	if err != nil {
		writeError(w, http.StatusConflict, "No tunnel address is free; see COZYCAST_TUNNEL_NET.")
		return "", errAnswered
	}
	token, err := newRoomToken()
	if err != nil {
		return "", err
	}
	nekoURL := "http://" + net.JoinHostPort(addr.String(), "8080")
	rc, err := s.buildRoom(name, nekoURL, token)
	if err != nil {
		return "", err
	}
	rc.Source = "paired"
	room := store.RegisteredRoom{Name: name, NekoURL: nekoURL, NekoToken: token, CreatedBy: &by,
		NodeKey: key.String(), TunnelAddress: addr.String()}
	if err := s.store.CreateRegisteredRoom(ctx, &room); err != nil {
		if errors.Is(err, store.ErrRoomExists) {
			writeError(w, http.StatusConflict, "That room already exists.")
			return "", errAnswered
		}
		return "", err
	}
	if err := s.tunnel.SetPeer(tunnel.Peer{Key: key, Address: addr}); err != nil {
		_ = s.store.DeleteRegisteredRoom(ctx, name)
		return "", err
	}
	if err := s.hub.Add(ctx, rc); err != nil {
		_ = s.tunnel.RemovePeer(key)
		_ = s.store.DeleteRegisteredRoom(ctx, name)
		return "", err
	}
	return name, nil
}

// replaceNode gives a paired room a new computer. Caller holds roomMu.
func (s *Server) replaceNode(ctx context.Context, w http.ResponseWriter, name string, key tunnel.Key) (string, error) {
	room, err := s.store.RegisteredRoom(ctx, name)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !room.Paired()) {
		writeError(w, http.StatusConflict, "Only a paired room can get a new computer.")
		return "", errAnswered
	}
	if err != nil {
		return "", err
	}
	old, oldErr := tunnel.ParseKey(room.NodeKey)
	addr, err := netip.ParseAddr(room.TunnelAddress)
	if err != nil {
		return "", err
	}
	if err := s.store.ReplaceNode(ctx, name, key.String()); err != nil {
		return "", err
	}
	if oldErr == nil {
		_ = s.tunnel.RemovePeer(old)
	}
	if err := s.tunnel.SetPeer(tunnel.Peer{Key: key, Address: addr}); err != nil {
		return "", err
	}
	// The room keeps running: the server reconnects to the new computer's
	// neko at the same address, and tabs reconnect as after a restart.
	return name, nil
}

// nodeCheckedIn notices a computer whose agent started anew (its boot ID
// changed): connections to its room through the old tunnel are dead, so
// the server reconnects now instead of at its next ping, up to 30 s later.
func (s *Server) nodeCheckedIn(room, boot string) {
	s.nodeMu.Lock()
	prev, seen := s.nodeBoots[room]
	s.nodeBoots[room] = boot
	s.nodeMu.Unlock()
	if seen && prev != boot {
		if rm := s.hub.Room(room); rm != nil {
			s.log.Info("paired computer restarted; reconnecting", "room", room)
			rm.Neko().Reconnect()
		}
	}
}

// NodeHandler serves the paired computers, inside the tunnel only: each
// gets its own room's settings, recognized by its tunnel address (which
// WireGuard ties to its key). The neko token never leaves the tunnel.
func (s *Server) NodeHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /node/config", func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		rooms, err := s.store.RegisteredRooms(r.Context())
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		for _, room := range rooms {
			if room.Paired() && room.TunnelAddress == host {
				s.nodeCheckedIn(room.Name, r.URL.Query().Get("boot"))
				writeJSON(w, http.StatusOK, map[string]string{"room": room.Name, "nekoToken": room.NekoToken})
				return
			}
		}
		writeError(w, http.StatusNotFound, "This computer is not paired.")
	})
	return mux
}
