package moderation

import (
	"log/slog"
	"math"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rekognition/types"
)

// categories is Rekognition's current top-level (L1) moderation taxonomy.
// AWS renamed categories in the v7 model, so a top-level name outside this
// set is logged rather than silently ignored.
var categories = map[string]bool{
	"Explicit": true,
	"Non-Explicit Nudity of Intimate parts and Kissing": true,
	"Swimwear or Underwear":                             true,
	"Violence":                                          true,
	"Visually Disturbing":                               true,
	"Drugs & Tobacco":                                   true,
	"Alcohol":                                           true,
	"Rude Gestures":                                     true,
	"Gambling":                                          true,
	"Hate Symbols":                                      true,
}

// thresholds is the minimum confidence at which a label under each
// top-level category rejects an image. Categories not listed never do.
var thresholds = map[string]float32{
	"Explicit":            80,
	"Violence":            80,
	"Visually Disturbing": 80,
	"Hate Symbols":        80,
}

// minConfidence is the MinConfidence to request: the lowest threshold, so
// every label that could reject is returned, or 0 in calibration mode to
// log everything Rekognition sees.
func minConfidence(calibration bool) float32 {
	if calibration {
		return 0
	}
	lowest := float32(math.MaxFloat32)
	for _, t := range thresholds {
		lowest = min(lowest, t)
	}
	return lowest
}

// decision is the outcome for one image. Label is the triggering label
// when rejected.
type decision struct {
	Rejected bool
	Label    string
}

// decide rejects on the highest-confidence label whose top-level category
// has a threshold it meets.
func decide(labels []types.ModerationLabel) decision {
	parents := make(map[string]string, len(labels))
	for _, l := range labels {
		parents[aws.ToString(l.Name)] = aws.ToString(l.ParentName)
	}

	var d decision
	var best float32
	for _, l := range labels {
		top := topLevel(aws.ToString(l.Name), parents)
		threshold, ok := thresholds[top]
		if !ok {
			if !categories[top] {
				slog.Warn("unknown moderation category", "category", top, "label", aws.ToString(l.Name))
			}
			continue
		}
		if c := aws.ToFloat32(l.Confidence); c >= threshold && c > best {
			d = decision{Rejected: true, Label: aws.ToString(l.Name)}
			best = c
		}
	}
	return d
}

// topLevel follows ParentName up from name. A response lists a detected
// label's ancestors as labels of their own, but an ancestor that isn't
// listed still ends the walk at its name.
func topLevel(name string, parents map[string]string) string {
	for range 3 {
		parent, ok := parents[name]
		if !ok || parent == "" {
			return name
		}
		name = parent
	}
	return name
}
