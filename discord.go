package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
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

// analyzeAndSend runs the analysis once, generates a PNG, and posts it to
// every configured Discord webhook.
func analyzeAndSend(db *sql.DB, webhookURLs []string) {
	recs, err := analyze(db, time.Now())
	if err != nil {
		log.Printf("analysis: %v", err)
		return
	}
	if err := sendAnalysisDiscord(webhookURLs, recs, time.Now()); err != nil {
		log.Printf("send discord: %v", err)
		return
	}
	log.Printf("sent analysis to discord (%d pairs)", len(recs))
}

// sendAnalysisDiscord renders the table to a PNG once, then POSTs it as a
// multipart attachment to every webhook URL. Each URL is attempted
// independently: a failure on one does not stop the others.
func sendAnalysisDiscord(webhookURLs []string, recs []recommendation, now time.Time) error {
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

	// POST the same PNG to every webhook; collect per-URL errors so a
	// single bad URL is reported without blocking the others.
	var errs []error
	for _, url := range webhookURLs {
		if err := sendMultipart(url, tmp.Name(), recs, now); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", url, err))
		}
	}
	return errors.Join(errs...)
}

// sendMultipart POSTs the PNG file along with a small embed summary.
func sendMultipart(webhookURL, imagePath string, recs []recommendation, now time.Time) error {
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)

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
