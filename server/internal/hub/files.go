package hub

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cozycast/internal/neko"
)

// Files of the room desktop's Downloads folder. Browsers list, upload and
// download them through neko's file transfer; deleting and playing are
// asked of the server, which does them for a tab that may.

// fileAction deletes or plays a file of the Downloads folder for a tab and
// tells the tab how that went.
func (r *Room) fileAction(ctx context.Context, c *Client, action, name string) {
	err := r.doFileAction(ctx, c, action, name)
	res := fileResultMsg{Type: "file_result", Action: action, Name: name}
	var userErr *userError
	switch {
	case err == nil:
		r.log.Info(action + " file")
	case errors.As(err, &userErr):
		res.Error = userErr.msg
	case errors.Is(err, ErrNotAllowed):
		res.Error = "You are not allowed to do that."
	case errors.Is(err, neko.ErrNotFound):
		res.Error = "That file is not in Downloads any more."
	default:
		r.log.Error(action+" file", "err", err)
		res.Error = "Something went wrong."
	}
	c.send(res)
}

func (r *Room) doFileAction(ctx context.Context, c *Client, action, name string) error {
	r.mu.Lock()
	rt := c.m.rights
	// Playing changes what everyone watches, like the remote does: it needs
	// that right too, and respects who owns the remote.
	owner := ""
	if action == "play" {
		owner = r.remoteOwnerLocked(c)
	}
	r.mu.Unlock()
	if !rt.Upload || (action == "play" && !rt.Remote) {
		return ErrNotAllowed
	}
	if owner != "" {
		return &userError{owner + " owns the remote."}
	}
	if name == "" || len(name) > 255 || strings.ContainsAny(name, "/\x00") {
		return &userError{"That file is not in Downloads."}
	}
	if action == "delete" {
		// neko's file transfer deletes for its admin, which is the server.
		return r.neko.DeleteFile(ctx, name)
	}
	return r.playFile(ctx, name)
}

// remoteOwnerLocked is the nickname of who owns the remote, where the room
// respects that and it is not c's person; "" if nobody stands in c's way.
func (r *Room) remoteOwnerLocked(c *Client) string {
	if host := r.clients[r.hostID]; r.settings.RemoteOwnership && host != nil && host.m != c.m {
		return r.userLocked(host.m).Nickname
	}
	return ""
}

// playFile asks the helper in the room's container (worker/play.py) to play
// the file on the desktop.
func (r *Room) playFile(ctx context.Context, name string) error {
	if r.playURL == "" {
		return &userError{"This room's desktop cannot play files."}
	}
	status, err := r.postHelper(ctx, r.playURL+"?name="+url.QueryEscape(name))
	if err != nil {
		// Restarting, or a room image from before the helper.
		r.log.Warn("play file", "err", err)
		return &userError{"The room's desktop did not answer. It may need a newer room image."}
	}
	switch status {
	case http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return neko.ErrNotFound
	default:
		return fmt.Errorf("play helper: %d", status)
	}
}

// postHelper posts to the play helper at address, with its token, and
// answers the status of its answer.
func (r *Room) postHelper(ctx context.Context, address string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+r.playToken)
	res, err := (&http.Client{Timeout: 15 * time.Second, Transport: r.helperTransport(false)}).Do(req)
	if err != nil {
		return 0, err
	}
	res.Body.Close()
	return res.StatusCode, nil
}
