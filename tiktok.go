package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"

	"github.com/bwmarrin/discordgo"
	"github.com/gocolly/colly"
)

//this looks bad but I think it's correct

type TiktokData struct {
	ItemList struct {
		Video struct {
			PreloadList []struct {
				URL string `json:"url,omitempty"`
				ID  string `json:"id,omitempty"`
			} `json:"preloadList"`
		} `json:"video"`
	} `json:"ItemList"`
}

func containsURL(message string) (bool, string) {
	expr, err := regexp.Compile(`(https:\/\/)?((www|vm)\.)(tiktok\.com\/)[a-zA-Z0-9@_\.\/]*`)

	if err != nil {
		fmt.Println(err)
	}
	return expr.MatchString(message), expr.FindString(message)

}

func scrape(tiktok string) (string, string, error) {
	fullURL := ""
	vidID := ""
	c := colly.NewCollector(
		colly.UserAgent("let-me-in"),
		colly.AllowURLRevisit(),
	)

	c.OnHTML("script[id=SIGI_STATE]", func(e *colly.HTMLElement) {
		data := TiktokData{}
		err := json.Unmarshal([]byte(e.Text), &data)

		if err != nil {
			return
		}

		if len(data.ItemList.Video.PreloadList) != 1 {
			return
		}

		fullURL = data.ItemList.Video.PreloadList[0].URL
		vidID = data.ItemList.Video.PreloadList[0].ID
	})

	err := c.Visit(tiktok)

	if err != nil {
		return fullURL, vidID, err
	}

	if fullURL == "" {
		return fullURL, vidID, errors.New("no URL found")
	}

	return fullURL, vidID, nil
}

func downloadTikTok(URL string, s *discordgo.Session, m *discordgo.MessageCreate) error {
	fullURL, vidID, err := scrape(URL)

	if err != nil {
		return err
	}

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
