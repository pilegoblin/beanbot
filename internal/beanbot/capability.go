package beanbot

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/pilegoblin/beanbot/internal/gemini"
	"google.golang.org/genai"
)

// Guild is the narrow slice of Discord that BeanBot needs beyond the gateway.
// Capabilities depend on this rather than *discordgo.Session so their gates
// and validation can be tested without a network.
type Guild interface {
	MemberPermissions(userID, channelID string) (int64, error)
	CreateEvent(guildID string, params *discordgo.GuildScheduledEventParams) (*discordgo.GuildScheduledEvent, error)
	SendMessage(channelID string, send *discordgo.MessageSend) error
}

// Execution is everything a Capability needs to run: the arguments the model
// chose, where the Trigger came from, and the clock and timezone that relative
// times resolve against.
type Execution struct {
	Args      map[string]any
	GuildID   string
	ChannelID string
	UserID    string
	// Author is what the channel calls the member who sent the Trigger, the
	// same name the Backlog shows. UserID identifies them; this attributes them.
	Author string
	// Mentions are the users @-mentioned in the Trigger, already resolved by
	// Discord. They are the only names besides the Author's that a snowflake may
	// be attached to, because they are the only ones Discord stated outright.
	Mentions []namedUser
	Images   []gemini.Image
	Now      time.Time
	Location *time.Location
	Guild    Guild
}

// Result is what a Capability produces. Summary is fed back to the model so it
// can narrate what happened; Files are posted to the channel alongside the
// final reply.
type Result struct {
	Summary string
	Files   []*discordgo.File
}

// Capability is something BeanBot can do beyond talking. Each one declares its
// own schema, its own Gate, and whether it changes the guild.
type Capability interface {
	// Declaration is the tool schema shown to the model.
	Declaration() *genai.FunctionDeclaration
	// RequiredPermission is the Discord permission the requesting member must
	// hold. Zero means anyone may ask.
	RequiredPermission() int64
	// Mutating reports whether this changes *Guild* state — the server's event
	// list, its channels, anything visible in Discord. At most one Guild-mutating
	// Capability runs per turn. Writing to Memory is a mutation that deliberately
	// does not count: it costs no API call and changes nothing in the Guild.
	Mutating() bool
	Execute(ctx context.Context, inv Execution) (Result, error)
}

// checkGate enforces a Capability's permission requirement against the member
// who sent the Trigger. This lives in Go, never in the Backstory: a system
// instruction is advisory and a language model can be talked out of it.
func checkGate(c Capability, inv Execution) error {
	need := c.RequiredPermission()
	if need == 0 {
		return nil
	}

	have, err := inv.Guild.MemberPermissions(inv.UserID, inv.ChannelID)
	if err != nil {
		return fmt.Errorf("could not read permissions: %w", err)
	}

	// Discord treats Administrator as implying every permission. Otherwise the
	// member must hold every bit the Capability asks for, not merely one.
	if have&discordgo.PermissionAdministrator != 0 || have&need == need {
		return nil
	}

	// A refusal is nearly always a misconfiguration rather than an actual
	// intruder, and the bitfield is the only thing that says which.
	log.Printf("gate refused user %s in channel %s: have %#x, need %#x",
		inv.UserID, inv.ChannelID, have, need)
	return fmt.Errorf("you need the %q permission in this server to do that", permissionName(need))
}

// permissionName names a permission the way Discord's own UI does, so a
// refusal tells someone which switch to flick.
func permissionName(bit int64) string {
	switch bit {
	case discordgo.PermissionManageEvents:
		return "Manage Events"
	case discordgo.PermissionManageServer:
		return "Manage Server"
	case discordgo.PermissionManageMessages:
		return "Manage Messages"
	default:
		return fmt.Sprintf("permission %#x", bit)
	}
}

// argString reads a required string argument from the model's chosen arguments.
func argString(args map[string]any, key string) (string, error) {
	v, ok := args[key].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("missing required argument %q", key)
	}
	return v, nil
}

// optionalString reads an argument that may legitimately be absent.
func optionalString(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

// optionalStrings reads a list argument that may legitimately be absent. The
// model's arguments arrive as decoded JSON, so a list of strings is a []any of
// strings; anything else in it is skipped rather than failing the whole call.
func optionalStrings(args map[string]any, key string) []string {
	items, ok := args[key].([]any)
	if !ok {
		return nil
	}

	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
