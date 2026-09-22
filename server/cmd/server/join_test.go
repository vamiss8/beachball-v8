package main

import (
	"net/http"
	"strings"
	"testing"

	"beachball-v8/server/internal/game"
	"beachball-v8/server/internal/room"
)

// these go through the same path as a browser following an invite link: a
// real listener, a real handshake and the welcome the room sends back. the
// room package tests the rules on their own; this is what proves the http
// layer actually hands a connection to the room those rules pick.

func TestInviteLinkSeatsTheFriendOnTheOtherSide(t *testing.T) {
	base := testServer(t, room.DefaultMaxRooms)

	host := join(t, base, "")
	if host.RoomID == "" {
		t.Fatal("the first player was not told which room they are in")
	}
	if host.Spectator || host.Side != game.SideLeft {
		t.Fatalf("host: side %q spectator %v, want the left side as a player", host.Side, host.Spectator)
	}

	// chat apps are free to lower-case a link on the way, and the friend
	// must still land in the same match
	guest := join(t, base, strings.ToLower(host.RoomID))
	if guest.RoomID != host.RoomID {
		t.Fatalf("guest landed in %q, want the host's room %q", guest.RoomID, host.RoomID)
	}
	if guest.Spectator || guest.Side != game.SideRight {
		t.Fatalf("guest: side %q spectator %v, want the right side as a player", guest.Side, guest.Spectator)
	}

	// both sides are taken, so a third person watches instead of playing
	if third := join(t, base, host.RoomID); !third.Spectator {
		t.Fatalf("third player got side %q, want to be seated as a spectator", third.Side)
	}
}

func TestFullServerStillLetsFriendsIn(t *testing.T) {
	base := testServer(t, 1)
	host := join(t, base, "")

	// the cap is on simulations, not on people: nobody new may open a room,
	// but the one that is running still takes the friend with the link
	_, resp, err := dial(t, base, "", "")
	if err == nil {
		t.Fatal("a second room opened past a cap of one")
	}
	if resp == nil {
		t.Fatalf("second room: no http response to read the refusal from: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("second room: status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}

	if guest := join(t, base, host.RoomID); guest.RoomID != host.RoomID {
		t.Fatalf("guest landed in %q, want %q", guest.RoomID, host.RoomID)
	}
}
