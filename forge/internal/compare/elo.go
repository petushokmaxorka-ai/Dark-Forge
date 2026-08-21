// Package compare — Elo rating math for blind model comparisons.
package compare

import (
	"crypto/rand"
	"fmt"
	"math"
	"time"
)

var randReader = rand.Reader

// timeNow is overridable for deterministic tests.
var timeNow = func() time.Time { return time.Now() }

const defaultElo = 1500.0

// expectedScore returns the predicted outcome for ratingA against ratingB.
func expectedScore(ratingA, ratingB float64) float64 {
	return 1.0 / (1.0 + math.Pow(10.0, (ratingB-ratingA)/400.0))
}

// kFactor returns the K-factor based on number of games played.
func kFactor(games int) float64 {
	if games < 30 {
		return 32.0
	}
	return 16.0
}

// updatePair updates ratings for a single winner vs loser pair.
func updatePair(ratings map[string]*Rating, winnerID, loserID string) {
	w := rating(ratings, winnerID)
	l := rating(ratings, loserID)

	expW := expectedScore(w.Rating, l.Rating)
	expL := expectedScore(l.Rating, w.Rating)

	w.Rating += kFactor(w.Games) * (1.0 - expW)
	l.Rating += kFactor(l.Games) * (0.0 - expL)

	w.Games++
	l.Games++
	w.Wins++
	l.Losses++
}

// GenerateID creates a time-sortable, collision-resistant identifier.
func GenerateID() string {
	b := make([]byte, 8)
	_, _ = randReader.Read(b)
	return fmt.Sprintf("%s-%x", timeNow().UTC().Format("20060102-150405"), b)
}
