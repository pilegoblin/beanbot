package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"

	"github.com/bwmarrin/discordgo"
	"github.com/gocolly/colly"
)

func containsURL(message string) (bool, string) {
	expr, err := regexp.Compile(`(https:\/\/)?((www|vm)\.)(tiktok\.com\/)[a-zA-Z0-9@_\.\/]*`)

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
		expr, err := regexp.Compile(`(https:\/\/)[a-zA-Z0-9@_\.\-\/]*`)

		if err != nil {
			fmt.Println(err)
		}

		s.ChannelMessageSend(m.ChannelID, expr.FindString(fullURL))

	} else {
		sendVid(s, m.ChannelID, file, vidID)
		file.Close()

	}
	err = os.Remove(vidID)

	return err

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
