package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"
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
}

type discordPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

// buildEmbed renders recommendations as a Discord webhook embed payload,
// ordered by spread (sell - buy) so the most profitable pairs come first.
func buildEmbed(recs []recommendation, now time.Time) []byte {
	sorted := make([]recommendation, len(recs))
	copy(sorted, recs)
	sort.Slice(sorted, func(i, j int) bool {
		return spreadOf(sorted[i]) > spreadOf(sorted[j])
	})
	if len(sorted) > maxPairs {
		sorted = sorted[:maxPairs]
	}

	embed := discordEmbed{
		Title:       "PoE2 Currency Analysis",
		Description: "All prices in Exalted Orbs",
		Color:       0xf1c40f, // gold, fitting the currency theme
		Timestamp:   now.UTC().Format(time.RFC3339),
		Fields:      make([]discordField, 0, len(sorted)),
	}
	for _, r := range sorted {
		embed.Fields = append(embed.Fields, discordField{
			Name:  displayOf(r),
			Value: "```\n" + tableOf(r) + "```",
		})
	}
	payload, _ := json.Marshal(discordPayload{Embeds: []discordEmbed{embed}})
	return payload
}

// tableOf renders one currency's mini-table as a monospaced block.
func tableOf(r recommendation) string {
	return fmt.Sprintf(
		"%-8s %9.2f\n%-8s %9.2f\n%-8s %9.2f\n%-8s %8.1f%%",
		"Current", r.Current,
		"Buy", r.BuyTarget,
		"Sell", r.SellTarget,
		"Spread", spreadOf(r),
	)
}

func displayOf(r recommendation) string {
	if r.Display != "" {
		return r.Display
	}
	return r.Name
}

func spreadOf(r recommendation) float64 {
	if r.BuyTarget <= 0 {
		return 0
	}
	return (r.SellTarget - r.BuyTarget) / r.BuyTarget * 100
}

// sendDiscord posts a JSON payload to a Discord webhook URL.
func sendDiscord(webhookURL string, payload []byte) error {
	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discord returned status %d", resp.StatusCode)
	}
	return nil
}

// analyzeAndSend runs the analysis once and posts results to Discord.
func analyzeAndSend(db *sql.DB, webhookURL string) {
	recs, err := analyze(db, time.Now())
	if err != nil {
		log.Printf("analysis: %v", err)
		return
	}
	if err := sendDiscord(webhookURL, buildEmbed(recs, time.Now())); err != nil {
		log.Printf("send discord: %v", err)
		return
	}
	log.Printf("sent analysis to discord (%d pairs)", len(recs))
}
