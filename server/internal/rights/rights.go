// Package rights computes room admission and effective rights without I/O.
package rights

import "cozycast/internal/store"

// Grant is what a temporary access invite gives for one visit.
type Grant struct{ Remote, Image, Upload bool }

type Input struct {
	Room  store.RoomSettings
	User  *store.User
	Perm  store.Permission
	Grant *Grant
	// Given is what an admin gave an anonymous person for as long as they
	// are in the room. Unlike Grant it lets nobody in.
	Given   Grant
	AnonBan *store.AnonBan
	Now     int64
}

type Rights struct {
	Admin   bool `json:"admin"`
	Trusted bool `json:"trusted"`
	Remote  bool `json:"remote"`
	Image   bool `json:"image"`
	Upload  bool `json:"upload"`
}

// Denial explains why someone may not join.
type Denial struct {
	Reason      string // banned | account | verified | invite
	BannedUntil *int64 // for banned: nil = forever
}

// Admit returns nil if the identity may join, or the reason it may not.
func Admit(in Input) *Denial {
	if in.User != nil {
		if in.Perm.Banned && (in.Perm.BannedUntil == nil || *in.Perm.BannedUntil > in.Now) {
			return &Denial{Reason: "banned", BannedUntil: in.Perm.BannedUntil}
		}
	} else if in.AnonBan != nil {
		return &Denial{Reason: "banned", BannedUntil: in.AnonBan.BannedUntil}
	}

	admin := in.User != nil && in.User.Admin
	trusted := admin || in.Perm.Trusted
	invited := in.Perm.Invited || in.Grant != nil
	switch in.Room.Access {
	case "public":
		return nil
	case "account":
		if in.User != nil || invited {
			return nil
		}
	case "verified":
		if admin || (in.User != nil && in.User.Verified) || invited {
			return nil
		}
	case "invite":
		if trusted || invited {
			return nil
		}
	}
	return &Denial{Reason: in.Room.Access}
}

// Compute returns the identity's effective room rights.
func Compute(in Input) Rights {
	var grant Grant
	if in.Grant != nil {
		grant = *in.Grant
	}
	admin := in.User != nil && in.User.Admin
	trusted := admin || in.Perm.Trusted
	return Rights{
		Admin:   admin,
		Trusted: trusted,
		Remote:  trusted || in.Perm.Remote || in.Room.DefaultRemote || grant.Remote || in.Given.Remote,
		Image:   in.User != nil && (trusted || in.Perm.Image || in.Room.DefaultImage || grant.Image),
		Upload:  trusted || in.Perm.Upload || in.Room.DefaultUpload || grant.Upload || in.Given.Upload,
	}
}
