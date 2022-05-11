package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/gocolly/colly"
)

var (
	Token string
)

func init() {

	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.Parse()
}

func main() {
	dg, err := discordgo.New("Bot " + Token)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	dg.AddHandler(messageCreate)

	dg.Identify.Intents = discordgo.IntentsGuildMessages

	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	fmt.Println("beanbot online")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	dg.Close()
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	if m.Author.ID == s.State.User.ID {
		return
	}

	isTikTok, URL := containsURL(m.Content)
	if isTikTok {
		err := downloadTikTok(URL, s, m)
		if err != nil {
			fmt.Print(err)
		}
		// remove embed
		_, err = s.Request("PATCH", discordgo.EndpointChannelMessage(m.ChannelID, m.ID), map[string]any{"flags": 4})

		if err != nil {
			fmt.Print(err)
		}

	}
}

func containsURL(message string) (bool, string) {
	expr, err := regexp.Compile(`(https:\/\/)?((www|vm)\.)(tiktok\.com\/)[a-zA-Z0-9@_\.]*\/?(video\/[0-9]{1,20})?`)

	if err != nil {
		fmt.Println(err)
	}
	return expr.MatchString(message), expr.FindString(message)

}

func scrape(tiktok string) (string, string) {
	fullURL := ""
	vidID := ""
	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/101.0.4951.54 Safari/537.36"),
		colly.AllowURLRevisit(),
	)

	c.OnHTML("script[id=SIGI_STATE]", func(e *colly.HTMLElement) {
		var data any
		json.Unmarshal([]byte(e.Text), &data)
		fullURL = data.(map[string]any)["ItemList"].(map[string]any)["video"].(map[string]any)["preloadList"].([]any)[0].(map[string]any)["url"].(string)
		vidID = data.(map[string]any)["ItemList"].(map[string]any)["video"].(map[string]any)["preloadList"].([]any)[0].(map[string]any)["id"].(string)
	})

	c.Visit(tiktok)
	return fullURL, vidID
}

func downloadTikTok(URL string, s *discordgo.Session, m *discordgo.MessageCreate) error {
	fullURL, vidID := scrape(URL)
	vidID = vidID + ".mp4"
	resp, err := http.Get(fullURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(vidID)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	file, err := os.Open(vidID)
	if err != nil {
		return err
	}

	finfo, err2 := (*file).Stat()

	if err2 != nil {
		return err2
	}

	if finfo.Size() >= 10485760 {
		s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
			Title: "video too beeg :( watch here",
			URL:   fullURL,
		})
		return err
	} else {
		sendVid(s, m.ChannelID, file, vidID)
		file.Close()
		err = os.Remove(vidID)

		return err
	}

}

func sendVid(s *discordgo.Session, channelID string, file *os.File, vidID string) {
	s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Files: []*discordgo.File{
			{
				Name:   vidID,
				Reader: file,
			},
		},
	})
}
