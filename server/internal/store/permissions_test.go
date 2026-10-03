package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestPermissions(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	if got, err := s.ListPermissions(ctx, ""); err != nil || len(got) != 0 {
		t.Fatalf("empty list: %v %+v", err, got)
	}
	alice := &User{Username: "Alice"}
	bob := &User{Username: "Bob"}
	for _, u := range []*User{alice, bob} {
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	zero := Permission{Room: "main", UserID: alice.ID}
	if got, err := s.Permission(ctx, zero.Room, zero.UserID); err != nil || !reflect.DeepEqual(got, zero) {
		t.Fatalf("defaults: %v %+v", err, got)
	}
	until := int64(2000)
	full := Permission{Room: "main", UserID: alice.ID, Remote: true, Image: true, Upload: true,
		Trusted: true, Invited: true, InviteName: "friends", Banned: true, BannedUntil: &until}
	for _, want := range []Permission{full, zero, full} {
		if err := s.SavePermission(ctx, want); err != nil {
			t.Fatal(err)
		}
		if got, err := s.Permission(ctx, want.Room, want.UserID); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("saved: %v %+v, want %+v", err, got, want)
		}
	}
	for _, p := range []Permission{{Room: "main", UserID: bob.ID}, {Room: "aaa", UserID: bob.ID}} {
		if err := s.SavePermission(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.ListPermissions(ctx, "")
	if err != nil || len(all) != 3 {
		t.Fatalf("list all: %v %+v", err, all)
	}
	if all[0].Room != "aaa" || all[0].Username != "bob" || all[1].Username != "alice" || all[2].Username != "bob" {
		t.Fatalf("list order: %+v", all)
	}
	full.Username = "alice"
	if !reflect.DeepEqual(all[1], full) {
		t.Fatalf("joined permission: %+v, want %+v", all[1], full)
	}
	if got, err := s.ListPermissions(ctx, "main"); err != nil || len(got) != 2 {
		t.Fatalf("room list: %v %+v", err, got)
	}
	if err := s.DeletePermission(ctx, "main", alice.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePermission(ctx, "main", alice.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
	if got, err := s.Permission(ctx, "main", alice.ID); err != nil || !reflect.DeepEqual(got, zero) {
		t.Fatalf("deleted defaults: %v %+v", err, got)
	}
}

func TestBanUser(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	u := &User{Username: "alice"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	p := Permission{Room: "main", UserID: u.ID, Remote: true, Image: true, Upload: true,
		Trusted: true, Invited: true, InviteName: "friends"}
	if err := s.SavePermission(ctx, p); err != nil {
		t.Fatal(err)
	}
	until := int64(2000)
	for _, expiry := range []*int64{&until, nil} {
		if err := s.BanUser(ctx, p.Room, p.UserID, expiry); err != nil {
			t.Fatal(err)
		}
		p.Banned, p.BannedUntil = true, expiry
		if got, err := s.Permission(ctx, p.Room, p.UserID); err != nil || !reflect.DeepEqual(got, p) {
			t.Fatalf("ban preserved columns: %v %+v, want %+v", err, got, p)
		}
	}
	if err := s.BanUser(ctx, "new", u.ID, nil); err != nil {
		t.Fatal(err)
	}
	want := Permission{Room: "new", UserID: u.ID, Banned: true}
	if got, err := s.Permission(ctx, "new", u.ID); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("new ban: %v %+v", err, got)
	}
}
