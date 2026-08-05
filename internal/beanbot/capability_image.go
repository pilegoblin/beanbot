package beanbot

import (
	"bytes"
	"context"
	"errors"

	"github.com/bwmarrin/discordgo"
	"github.com/pilegoblin/beanbot/internal/gemini"
	"google.golang.org/genai"
)

// imageDrawer is the part of the model client the image Capabilities need.
type imageDrawer interface {
	GenerateImage(ctx context.Context, prompt string, inputs []gemini.Image) (gemini.Image, error)
}

// generateImage draws a new picture with Nano Banana. Not guild-mutating: it
// posts a file but changes nothing about the server. It is the most expensive
// thing BeanBot does per call, and it is ungated, so the image Medium is what
// stops one Trigger — or a planted Memory — from drawing all afternoon.
type generateImage struct{ drawer imageDrawer }

func (generateImage) RequiredPermission() int64 { return 0 }

func (generateImage) Mutating() bool { return false }

func (generateImage) Medium() Medium { return MediumImage }

// Deliberately uncued, for now. Drawing has the same exposure speech had —
// declared on every Trigger, ungated, and several times the price of a Clip —
// but nobody has reported it drawing unasked. When somebody does, it is a word
// list rather than a change of shape.
func (generateImage) Cues() []string { return nil }

func (generateImage) Declaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name: "generate_image",
		Description: "Draw a brand new image and post it to the channel. Use when someone " +
			"asks for a picture, drawing, meme or artwork of something.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"prompt": {
					Type: genai.TypeString,
					Description: "A vivid, self-contained description of the image to draw. " +
						"Include subject, style and mood; the drawing model cannot see the conversation.",
				},
			},
			Required: []string{"prompt"},
		},
	}
}

func (g generateImage) Execute(ctx context.Context, inv Execution) (Result, error) {
	return draw(ctx, g.drawer, inv, nil)
}

// editImage redraws the pictures in view — those attached to the Trigger, or
// to the message it replies to. Same model call as generation, with the
// source images attached.
type editImage struct{ drawer imageDrawer }

func (editImage) RequiredPermission() int64 { return 0 }

func (editImage) Mutating() bool { return false }

// The same Medium as drawing, because it is the same call to the same model at
// the same price. Redrawing repeatedly is the cheapest way to spend a lot.
func (editImage) Medium() Medium { return MediumImage }

func (editImage) Cues() []string { return nil }

func (editImage) Declaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name: "edit_image",
		Description: "Alter the image or images in view — attached either to the message you " +
			"were sent or to the one it replies to — and post the result.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"prompt": {
					Type:        genai.TypeString,
					Description: "What to change about the attached image, e.g. \"give him a wizard hat\".",
				},
			},
			Required: []string{"prompt"},
		},
	}
}

func (e editImage) Execute(ctx context.Context, inv Execution) (Result, error) {
	if len(inv.Images) == 0 {
		return Result{}, errors.New("there are no images attached to edit")
	}
	return draw(ctx, e.drawer, inv, inv.Images)
}

func draw(ctx context.Context, drawer imageDrawer, inv Execution, inputs []gemini.Image) (Result, error) {
	prompt, err := argString(inv.Args, "prompt")
	if err != nil {
		return Result{}, err
	}

	img, err := drawer.GenerateImage(ctx, prompt, inputs)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Summary: "The image was drawn and is being posted to the channel now.",
		Files: []*discordgo.File{{
			Name:        "beanbot" + extensionFor(img.MIME),
			ContentType: img.MIME,
			Reader:      bytes.NewReader(img.Data),
		}},
	}, nil
}

func extensionFor(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}
