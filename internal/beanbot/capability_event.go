package beanbot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"google.golang.org/genai"
)

// defaultEventDuration is how long an event runs when nobody says otherwise.
// Discord requires an end time on EXTERNAL events, so one must be invented.
const defaultEventDuration = 2 * time.Hour

// createEvent schedules a Guild Event. Arguments come from a language model
// rather than a validated form, so every field is checked here.
type createEvent struct{}

func (createEvent) RequiredPermission() int64 { return discordgo.PermissionManageEvents }

func (createEvent) Mutating() bool { return true }

func (createEvent) Declaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name: "create_event",
		Description: "Schedule a Discord event in this server. Use when someone asks to " +
			"organise, plan or schedule something at a particular time.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"name": {
					Type:        genai.TypeString,
					Description: "Short title of the event, e.g. \"Smash Night\".",
				},
				"scheduled_start_time": {
					Type: genai.TypeString,
					Description: "Start time as a full RFC3339 timestamp with offset, e.g. " +
						"\"2026-08-07T20:00:00-04:00\". Resolve relative times such as " +
						"\"friday 8pm\" against the current time given in the context. " +
						"Must be in the future.",
				},
				"scheduled_end_time": {
					Type:        genai.TypeString,
					Description: "Optional end time, same RFC3339 format. Defaults to two hours after the start.",
				},
				"location": {
					Type:        genai.TypeString,
					Description: "Where it happens — a place, a link, or e.g. \"online\".",
				},
				"description": {
					Type:        genai.TypeString,
					Description: "Optional longer description of the event.",
				},
			},
			Required: []string{"name", "scheduled_start_time", "location"},
		},
	}
}

func (createEvent) Execute(ctx context.Context, inv Execution) (Result, error) {
	name, err := argString(inv.Args, "name")
	if err != nil {
		return Result{}, err
	}
	location, err := argString(inv.Args, "location")
	if err != nil {
		return Result{}, err
	}

	rawStart, err := argString(inv.Args, "scheduled_start_time")
	if err != nil {
		return Result{}, err
	}
	start, err := parseEventTime(rawStart)
	if err != nil {
		return Result{}, err
	}
	if !start.After(inv.Now) {
		return Result{}, fmt.Errorf("that start time (%s) is in the past — the current time is %s",
			start.In(inv.Location).Format(time.RFC1123), inv.Now.In(inv.Location).Format(time.RFC1123))
	}

	end := start.Add(defaultEventDuration)
	if raw := optionalString(inv.Args, "scheduled_end_time"); raw != "" {
		end, err = parseEventTime(raw)
		if err != nil {
			return Result{}, err
		}
		if !end.After(start) {
			return Result{}, errors.New("the end time must be after the start time")
		}
	}

	// EXTERNAL is the only entity type that doesn't require binding the event
	// to a specific voice or stage channel, but Discord then demands both an
	// end time and a location.
	event, err := inv.Guild.CreateEvent(inv.GuildID, &discordgo.GuildScheduledEventParams{
		Name:               name,
		Description:        optionalString(inv.Args, "description"),
		ScheduledStartTime: &start,
		ScheduledEndTime:   &end,
		EntityType:         discordgo.GuildScheduledEventEntityTypeExternal,
		EntityMetadata:     &discordgo.GuildScheduledEventEntityMetadata{Location: location},
		PrivacyLevel:       discordgo.GuildScheduledEventPrivacyLevelGuildOnly,
	})
	if err != nil {
		return Result{}, fmt.Errorf("Discord refused to create the event: %w", err)
	}

	return Result{Summary: fmt.Sprintf(
		"Created event %q (id %s) starting %s at %s.",
		event.Name, event.ID, start.In(inv.Location).Format(time.RFC1123), location,
	)}, nil
}

// parseEventTime insists on RFC3339 so there is no ambiguity about offset.
// The error names the format because the model reads it and retries.
func parseEventTime(raw string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"%q is not a valid RFC3339 timestamp — use the form 2026-08-07T20:00:00-04:00", raw)
	}
	return t, nil
}
