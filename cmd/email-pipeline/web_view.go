package main

import (
	"time"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
)

type webPreview struct {
	Format string
	Text   string
	HTML   string
}

func makeWebPreview(rawFormat string) webPreview {
	format, err := campaign.ParseFormat(rawFormat)
	if err != nil {
		format = campaign.TextFormat
	}
	recipient := campaign.Recipient{Name: "Customer 000001"}
	text, _ := campaign.RenderMessage(recipient, campaign.TextFormat)
	html, _ := campaign.RenderMessage(recipient, campaign.HTMLFormat)
	return webPreview{Format: format.String(), Text: string(text.Bytes()), HTML: string(html.Bytes())}
}

func prepareWebPage(page webPage) webPage {
	page.Preview = makeWebPreview(page.Form.Format)
	return page
}

func requestDuration(started, ready time.Time) time.Duration {
	return ready.Sub(started)
}
