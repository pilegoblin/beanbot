package beanbot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"google.golang.org/genai"
)

// fakeGuild stands in for Discord so gates, validation and argument mapping
// can be exercised without a network.
type fakeGuild struct {
	perms   int64
	created *discordgo.GuildScheduledEventParams
	err     error
}

func (f *fakeGuild) MemberPermissions(userID, channelID string) (int64, error) {
	return f.perms, nil
}

func (f *fakeGuild) CreateEvent(guildID string, p *discordgo.GuildScheduledEventParams) (*discordgo.GuildScheduledEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.created = p
	return &discordgo.GuildScheduledEvent{ID: "evt1", Name: p.Name}, nil
}

func (f *fakeGuild) SendMessage(channelID string, send *discordgo.MessageSend) error { return nil }

var eventCap = createEvent{}

var friday = time.Date(2026, 8, 7, 20, 0, 0, 0, time.UTC)

func execution(g Guild, args map[string]any) Execution {
	return Execution{
		Args:      args,
		GuildID:   "g1",
		ChannelID: "c1",
		UserID:    "u1",
		Now:       time.Date(2026, 8, 4, 14, 0, 0, 0, time.UTC),
		Location:  time.UTC,
		Guild:     g,
	}
}

func eventArgs(start string) map[string]any {
	return map[string]any{
		"name":                 "Smash Night",
		"scheduled_start_time": start,
		"location":             "Drew's place",
	}
}

func TestMemberWithoutManageEventsIsRefused(t *testing.T) {
	// The Gate lives in Go precisely so it cannot be talked around in chat.
	g := &fakeGuild{perms: 0}

	err := checkGate(eventCap, execution(g, nil))

	if err == nil {
		t.Fatal("expected a member without MANAGE_EVENTS to be refused")
	}
	if g.created != nil {
		t.Error("no event should have been created")
	}
}

func TestMemberWithManageEventsPassesTheGate(t *testing.T) {
	g := &fakeGuild{perms: discordgo.PermissionManageEvents}

	if err := checkGate(eventCap, execution(g, nil)); err != nil {
		t.Errorf("expected the gate to pass, got %v", err)
	}
}

func TestAdministratorPassesEveryGate(t *testing.T) {
	// Discord treats Administrator as implying all permissions; a bot that
	// ignored that would refuse the server owner.
	g := &fakeGuild{perms: discordgo.PermissionAdministrator}

	if err := checkGate(eventCap, execution(g, nil)); err != nil {
		t.Errorf("expected an administrator to pass, got %v", err)
	}
}

func TestAGateRequiringTwoPermissionsNeedsBoth(t *testing.T) {
	// Holding one of the required bits is not holding the permission. Testing
	// have&need != 0 would wrongly admit a member with only half of it.
	twoBits := twoBitCap{need: discordgo.PermissionManageEvents | discordgo.PermissionManageServer}
	g := &fakeGuild{perms: discordgo.PermissionManageEvents}

	if err := checkGate(twoBits, execution(g, nil)); err == nil {
		t.Error("expected a member holding only one of two required permissions to be refused")
	}
}

type twoBitCap struct{ need int64 }

func (c twoBitCap) RequiredPermission() int64 { return c.need }
func (twoBitCap) Mutating() bool              { return true }
func (twoBitCap) Medium() Medium              { return NoMedium }
func (twoBitCap) Cues() []string              { return nil }
func (twoBitCap) Declaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{Name: "two_bit"}
}
func (twoBitCap) Execute(context.Context, Execution) (Result, error) { return Result{}, nil }

func TestEventInThePastIsRejected(t *testing.T) {
	// The model has no clock, so a confabulated date is a real possibility.
	g := &fakeGuild{perms: discordgo.PermissionManageEvents}

	_, err := eventCap.Execute(context.Background(),
		execution(g, eventArgs("2020-01-01T20:00:00Z")))

	if err == nil {
		t.Fatal("expected an event in the past to be rejected")
	}
	if g.created != nil {
		t.Error("no event should have been created")
	}
}

func TestUnparseableTimestampIsRejected(t *testing.T) {
	g := &fakeGuild{perms: discordgo.PermissionManageEvents}

	_, err := eventCap.Execute(context.Background(),
		execution(g, eventArgs("friday at 8pm")))

	if err == nil {
		t.Fatal("expected a non-RFC3339 timestamp to be rejected")
	}
	if !strings.Contains(err.Error(), "RFC3339") {
		t.Errorf("error should tell the model what format to use, got: %v", err)
	}
}

func TestEventIsCreatedWithTheRequestedStartTime(t *testing.T) {
	g := &fakeGuild{perms: discordgo.PermissionManageEvents}

	_, err := eventCap.Execute(context.Background(),
		execution(g, eventArgs("2026-08-07T20:00:00Z")))
	if err != nil {
		t.Fatalf("expected the event to be created, got %v", err)
	}

	if g.created == nil {
		t.Fatal("no event was created")
	}
	if !g.created.ScheduledStartTime.Equal(friday) {
		t.Errorf("start time = %v, want %v", g.created.ScheduledStartTime, friday)
	}
	if g.created.Name != "Smash Night" {
		t.Errorf("name = %q", g.created.Name)
	}
}

func TestExternalEventGetsTheEndTimeAndLocationDiscordDemands(t *testing.T) {
	// Discord rejects an EXTERNAL event that lacks either field, so beanbot
	// must supply an end time even when nobody asked for one.
	g := &fakeGuild{perms: discordgo.PermissionManageEvents}

	if _, err := eventCap.Execute(context.Background(),
		execution(g, eventArgs("2026-08-07T20:00:00Z"))); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if g.created.EntityType != discordgo.GuildScheduledEventEntityTypeExternal {
		t.Errorf("entity type = %v, want EXTERNAL", g.created.EntityType)
	}
	if g.created.ScheduledEndTime == nil {
		t.Fatal("EXTERNAL events require an end time; Discord will reject this")
	}
	if !g.created.ScheduledEndTime.After(friday) {
		t.Error("end time must be after the start time")
	}
	if g.created.EntityMetadata == nil || g.created.EntityMetadata.Location == "" {
		t.Error("EXTERNAL events require a location")
	}
}

func TestMissingEventNameIsRejected(t *testing.T) {
	g := &fakeGuild{perms: discordgo.PermissionManageEvents}

	_, err := eventCap.Execute(context.Background(),
		execution(g, map[string]any{"scheduled_start_time": "2026-08-07T20:00:00Z"}))

	if err == nil {
		t.Error("expected a nameless event to be rejected")
	}
}
