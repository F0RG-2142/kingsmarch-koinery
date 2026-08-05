package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"kingsmarch-koinery/render"
)

const (
	maxPairs        = 10
	analyzeInterval = 4 * time.Hour
)

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type discordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Color       int            `json:"color"`
	Timestamp   string         `json:"timestamp"`
	Fields      []discordField `json:"fields"`
	Image       *discordImage  `json:"image,omitempty"`
}

type discordImage struct {
	URL string `json:"url"`
}

type discordPayload struct {
	Content   string         `json:"content,omitempty"`
	Embeds    []discordEmbed `json:"embeds"`
	Username  string         `json:"username,omitempty"`
	AvatarURL string         `json:"avatar_url,omitempty"`
}

// analyzeAndSend runs the analysis once, generates a PNG, and posts it to Discord.
func analyzeAndSend(db *sql.DB, webhookURL string) {
	recs, err := analyze(db, time.Now())
	if err != nil {
		log.Printf("analysis: %v", err)
		return
	}
	if err := sendAnalysisDiscord(webhookURL, recs, time.Now()); err != nil {
		log.Printf("send discord: %v", err)
		return
	}
	log.Printf("sent analysis to discord (%d pairs)", len(recs))
}

// sendAnalysisDiscord renders the table to a PNG, POSTs it as a multipart
// attachment to the webhook, then cleans up the temp file.
func sendAnalysisDiscord(webhookURL string, recs []recommendation, now time.Time) error {
	// Convert to render.Recommendation.
	rRecs := make([]render.Recommendation, len(recs))
	for i, r := range recs {
		rRecs[i] = render.Recommendation{
			Name:       r.Name,
			Display:    r.Display,
			Current:    r.Current,
			BuyTarget:  r.BuyTarget,
			SellTarget: r.SellTarget,
			BulkSell:   r.BulkSell,
			HalfLife:   r.HalfLife,
		}
	}

	league := "runes"
	img := render.Table(rRecs, league, "PoE2 Currency Analysis")

	// Save to a temp file.
	tmp, err := os.CreateTemp("", "poe2-analysis-*.png")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := png.Encode(tmp, img); err != nil {
		return err
	}
	tmp.Close()

	return sendMultipart(webhookURL, tmp.Name(), recs, now)
}

// sendMultipart POSTs the PNG file along with a small embed summary.
func sendMultipart(webhookURL, imagePath string, recs []recommendation, now time.Time) error {
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)

	// File part.
	f, err := os.Open(imagePath)
	if err != nil {
		return err
	}
	defer f.Close()
	filePart, err := mw.CreateFormFile("files[0]", filepath.Base(imagePath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(filePart, f); err != nil {
		return err
	}

	// Embed summary referencing the attachment by filename.
	payload := discordPayload{
		Username: "PoE2 Scout",
		Embeds: []discordEmbed{{
			Title:       "PoE2 Currency Analysis",
			Description: "All prices in Exalted Orbs. Bulk = order \u2265 500",
			Color:       0xf1c40f,
			Timestamp:   now.UTC().Format(time.RFC3339),
			Image:       &discordImage{URL: "attachment://" + filepath.Base(imagePath)},
		}},
	}
	pj, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := mw.WriteField("payload_json", string(pj)); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, webhookURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discord returned status %d", resp.StatusCode)
	}
	return nil
}
