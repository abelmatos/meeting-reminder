package main

import (
	_ "embed"
	"html/template"
	"net/url"
	"strings"
	"time"
)

const snoozeDuration = 5 * time.Minute

//go:embed card.html
var cardSource string

var cardTemplate = template.Must(template.New("card").Parse(cardSource))

type cardData struct {
	Summary       string
	TimeRange     string
	Provider      string
	HasLink       bool
	SnoozeMinutes int
	StartMs       int64
	LeadMs        int64
}

// cardHTML renders the reminder card for e. The page counts down to the
// start by itself and reports the chosen action through
// window.chrome.webview.postMessage.
func cardHTML(e Event) (string, error) {
	var b strings.Builder
	err := cardTemplate.Execute(&b, cardData{
		Summary:       e.Summary,
		TimeRange:     e.Start.Local().Format("15:04") + " – " + e.End.Local().Format("15:04"),
		Provider:      meetingProvider(e.Link),
		HasLink:       joinURL(e.Link) != "",
		SnoozeMinutes: int(snoozeDuration.Minutes()),
		StartMs:       e.Start.UnixMilli(),
		LeadMs:        leadTime.Milliseconds(),
	})
	return b.String(), err
}

// joinURL returns link if it is safe to hand to the shell, otherwise "".
func joinURL(link string) string {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return u.String()
}

func meetingProvider(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "meet.google.com":
		return "Google Meet"
	case host == "zoom.us" || strings.HasSuffix(host, ".zoom.us"):
		return "Zoom"
	case strings.HasPrefix(host, "teams.") && strings.Contains(host, "microsoft"):
		return "Microsoft Teams"
	case host != "":
		return "Video call"
	}
	return ""
}
