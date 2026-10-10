package hub

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Hibernation: once nobody has been in a room for Hub.HibernateAfter, the
// programs on its desktop are paused (worker/hibernate.py), so that a video
// left playing there does not keep the machine busy. neko itself goes on;
// the first person to join has the programs continued.

// What the server knows of the programs on the desktop. Unknown after a
// start of the server, which may have paused them in its last run: they are
// continued for whoever joins, which does nothing to programs that run.
const (
	sleepUnknown = iota
	sleepAwake
	sleepFrozen
)

// Continuing is tried a few times: a desktop that stays paused under the
// people in the room is worse than a room that failed to hibernate.
// Variables for tests.
var (
	wakeTries = 3
	wakeRetry = time.Second
)

// Hibernating reports whether the server paused the desktop's programs.
func (r *Room) Hibernating() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sleep == sleepFrozen
}

// idleLocked starts the wait for hibernation if the room is empty. Every
// time it empties the wait starts over.
func (r *Room) idleLocked() {
	after := r.hub.HibernateAfter
	if after <= 0 || r.hibernateURL == "" || len(r.members) > 0 {
		return
	}
	r.idleGen++
	gen := r.idleGen
	time.AfterFunc(after, func() { r.hibernate(gen) })
}

func (r *Room) wakeNeededLocked() bool {
	return r.hibernateURL != "" && r.sleep != sleepAwake && len(r.members) > 0
}

// settleSleep is for a desktop that has just connected: its programs are
// continued for the people in the room, or the wait for hibernation starts.
func (r *Room) settleSleep() {
	r.mu.Lock()
	wake := r.wakeNeededLocked()
	r.idleLocked()
	r.mu.Unlock()
	if wake {
		go r.wake()
	}
}

func (r *Room) hibernate(gen int) {
	r.sleepMu.Lock()
	defer r.sleepMu.Unlock()
	r.mu.Lock()
	stale := gen != r.idleGen || len(r.members) > 0
	frozen := r.sleep == sleepFrozen
	if !stale && !frozen {
		// Whoever joins from here on has the programs continued, also
		// while they are still being paused.
		r.sleep = sleepUnknown
	}
	r.mu.Unlock()
	if stale {
		return
	}
	if err := r.desktopPrograms("freeze"); err != nil {
		// Unreachable, or a room image from before the helper.
		r.log.Warn("hibernate", "err", err)
		return
	}
	r.mu.Lock()
	r.sleep = sleepFrozen
	r.mu.Unlock()
	if !frozen {
		r.log.Info("hibernating")
	}
}

// wake continues the desktop's programs while someone is in the room.
func (r *Room) wake() {
	r.sleepMu.Lock()
	defer r.sleepMu.Unlock()
	for try := 1; ; try++ {
		r.mu.Lock()
		needed, frozen := r.wakeNeededLocked(), r.sleep == sleepFrozen
		r.mu.Unlock()
		if !needed {
			return
		}
		err := r.desktopPrograms("thaw")
		if err == nil {
			r.mu.Lock()
			r.sleep = sleepAwake
			r.mu.Unlock()
			if frozen {
				r.log.Info("woken")
			}
			return
		}
		if try >= wakeTries {
			// When the desktop connects again, or someone joins, it is
			// tried again. A desktop this server never paused most likely
			// has no helper (an older room image).
			if frozen {
				r.log.Warn("wake", "err", err)
			} else {
				r.log.Debug("wake", "err", err)
			}
			return
		}
		select {
		case <-r.removed.Done():
			return
		case <-time.After(wakeRetry):
		}
	}
}

// desktopPrograms asks the helper in the room's container
// (worker/hibernate.py) to "freeze" or "thaw" the desktop's programs.
func (r *Room) desktopPrograms(action string) error {
	ctx, cancel := context.WithTimeout(r.removed, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.hibernateURL+"/"+action, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.playToken)
	res, err := (&http.Client{Transport: r.neko.Transport()}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("hibernate helper: %s", res.Status)
	}
	return nil
}
