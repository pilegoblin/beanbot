package beanbot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/pilegoblin/beanbot/internal/gemini"
)

// maxAttachmentBytes caps how much of an attachment BeanBot will download.
// Discord allows large uploads and the bytes go straight into a model request.
const maxAttachmentBytes = 8 << 20 // 8 MiB

// Config is everything BeanBot needs that isn't a secret.
type Config struct {
	// BacklogSize is how many recent channel messages are read as context.
	BacklogSize int
	// Location is the Server Timezone that relative times resolve against.
	Location *time.Location
}

type BeanBot struct {
	session  *discordgo.Session
	prompter *gemini.Prompter
	agent    *agent
	guild    Guild
	config   Config
}

func NewBot(ctx context.Context, prompter *gemini.Prompter, config Config) (*BeanBot, error) {
	key, ok := os.LookupEnv("DISCORD_API_KEY")
	if !ok || key == "" {
		return nil, errors.New("DISCORD_API_KEY is not set")
	}

	dg, err := discordgo.New("Bot " + key)
	if err != nil {
		return nil, err
	}

	guild := &discordGuild{session: dg}
	bb := &BeanBot{
		session:  dg,
		prompter: prompter,
		guild:    guild,
		config:   config,
		agent: newAgent([]Capability{
			createEvent{},
			generateImage{drawer: prompter},
			editImage{drawer: prompter},
		}),
	}

	// MessageContent is privileged and must also be enabled in the Discord
	// developer portal. Without it every message arrives with empty content
	// and the Backlog is worthless.
	//
	// Guilds populates the state cache with guilds, channels and roles. Without
	// it every Gate check falls back to three sequential REST calls to work out
	// what the requesting member is allowed to do.
	dg.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentMessageContent
	dg.AddHandler(bb.onMessage(ctx))

	return bb, nil
}

func (bb *BeanBot) Start() error {
	if err := bb.session.Open(); err != nil {
		return err
	}
	defer func() {
		if err := bb.session.Close(); err != nil {
			log.Println(err)
		}
	}()

	wait := make(chan os.Signal, 1)
	signal.Notify(wait, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	log.Println("Beanbot Active")

	//blocks indefinitely letting the bot run
	<-wait

	return nil
}

// SetStatus sets the bot's status to 'Playing <status>'
func (bb *BeanBot) SetStatus(status string) {
	bb.session.AddHandler(func(s *discordgo.Session, event *discordgo.Ready) {
		if err := s.UpdateGameStatus(0, status); err != nil {
			log.Println(err)
		}
	})
}

func (bb *BeanBot) onMessage(ctx context.Context) func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// State.User is unset until READY; a nil deref here would take down
		// the whole bot rather than one message.
		if s.State == nil || s.State.User == nil {
			return
		}
		if !isTrigger(m.Message, s.State.User.ID) {
			return
		}
		// Each Trigger is independent — there is no shared session to
		// serialise on, so they may run concurrently.
		go bb.respond(ctx, m)
	}
}

func (bb *BeanBot) respond(ctx context.Context, m *discordgo.MessageCreate) {
	stopTyping := showTyping(bb.session, m.ChannelID)
	defer stopTyping()

	reply, files, err := bb.think(ctx, m)
	if err != nil {
		log.Printf("responding in %s: %v", m.ChannelID, err)
		bb.say(m.ChannelID, m.ID, "something went wrong in my head: "+err.Error(), nil)
		return
	}

	bb.say(m.ChannelID, m.ID, reply, files)
}

func (bb *BeanBot) think(ctx context.Context, m *discordgo.MessageCreate) (string, []*discordgo.File, error) {
	recent, err := bb.session.ChannelMessages(m.ChannelID, bb.config.BacklogSize, m.ID, "", "")
	if err != nil {
		// A missing Backlog is a degraded conversation, not a dead one.
		log.Printf("fetching backlog for %s: %v", m.ChannelID, err)
	}

	// The Trigger itself is not in the Backlog, since we asked for messages
	// before it.
	backlog := renderBacklog(append([]*discordgo.Message{m.Message}, recent...),
		bb.session.State.User.ID, bb.config.Location)

	// Editing works on whatever pictures are in view: the ones attached to the
	// Trigger, or the ones on the message it replies to.
	images := bb.downloadImages(m.Attachments)
	if m.ReferencedMessage != nil {
		images = append(images, bb.downloadImages(m.ReferencedMessage.Attachments)...)
	}

	now := time.Now().In(bb.config.Location)
	exec := Execution{
		GuildID:   m.GuildID,
		ChannelID: m.ChannelID,
		UserID:    m.Author.ID,
		Images:    images,
		Now:       now,
		Location:  bb.config.Location,
		Guild:     bb.guild,
	}

	turn := bb.prompter.NewTurn(bb.agent.declarations)
	return bb.agent.run(ctx, turn, situate(backlog, now), images, exec)
}

// situate tells the model where and when it is. It has no clock, so anything
// relative — "friday", "tomorrow" — is a guess without this.
func situate(backlog string, now time.Time) string {
	return fmt.Sprintf(
		"Current time: %s\nServer timezone: %s\n\n"+
			"Recent conversation in this channel, oldest first. The last line is the "+
			"message you are replying to.\n\n%s",
		now.Format(time.RFC3339), now.Location(), backlog)
}

func (bb *BeanBot) say(channelID, replyTo, text string, files []*discordgo.File) {
	chunks := splitMessage(text, discordMessageLimit)
	if len(chunks) == 0 && len(files) == 0 {
		return
	}

	for i, chunk := range chunks {
		send := &discordgo.MessageSend{Content: chunk}
		// Attach files to the last message so they land beside the sign-off.
		if i == len(chunks)-1 {
			send.Files = files
			files = nil
		}
		if err := bb.guild.SendMessage(channelID, send); err != nil {
			log.Printf("sending to %s: %v", channelID, err)
			return
		}
	}

	if len(files) > 0 {
		if err := bb.guild.SendMessage(channelID, &discordgo.MessageSend{Files: files}); err != nil {
			log.Printf("sending files to %s: %v", channelID, err)
		}
	}
}

func (bb *BeanBot) downloadImages(attachments []*discordgo.MessageAttachment) []gemini.Image {
	var images []gemini.Image
	for _, a := range attachments {
		if !strings.HasPrefix(strings.ToLower(a.ContentType), "image") {
			continue
		}

		data, err := download(a.URL)
		if err != nil {
			log.Printf("downloading %s: %v", a.Filename, err)
			continue
		}
		images = append(images, gemini.Image{Data: data, MIME: a.ContentType})
	}
	return images
}

// attachmentClient bounds how long a Trigger can be held open by a slow host.
// Every Trigger runs in its own goroutine, so an untimed fetch parks one
// indefinitely.
var attachmentClient = &http.Client{Timeout: 30 * time.Second}

func download(url string) ([]byte, error) {
	resp, err := attachmentClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	// Read one byte past the cap so an oversized attachment is an error rather
	// than a silently truncated, corrupt image.
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAttachmentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAttachmentBytes {
		return nil, fmt.Errorf("attachment is larger than %d bytes", maxAttachmentBytes)
	}
	return data, nil
}

// showTyping keeps the typing indicator alive until the returned function is
// called. Discord expires it after about ten seconds.
func showTyping(s *discordgo.Session, channelID string) func() {
	send := func() {
		if err := s.ChannelTyping(channelID); err != nil {
			log.Println(err)
		}
	}
	send()

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				send()
			case <-done:
				return
			}
		}
	}()

	return func() { close(done) }
}

// discordGuild is the live implementation of Guild, backed by a real session.
type discordGuild struct{ session *discordgo.Session }

func (d *discordGuild) MemberPermissions(userID, channelID string) (int64, error) {
	return d.session.UserChannelPermissions(userID, channelID)
}

func (d *discordGuild) CreateEvent(guildID string, params *discordgo.GuildScheduledEventParams) (*discordgo.GuildScheduledEvent, error) {
	return d.session.GuildScheduledEventCreate(guildID, params)
}

func (d *discordGuild) SendMessage(channelID string, send *discordgo.MessageSend) error {
	_, err := d.session.ChannelMessageSendComplex(channelID, send)
	return err
}
