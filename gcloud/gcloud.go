package gcloud

import (
	"context"
	"fmt"
	"strings"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/models"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
)

func GenerateBriefing(conf *config.Config, penalties *models.Penalties) (string, error) {
	briefingFile, err := copyFile(conf.BriefingTemplateDocID, conf.BriefingFolderID, fmt.Sprintf("Drivers Briefing Round %d at %s", conf.NextRound.Number, conf.NextRound.Track))
	if err != nil {
		return "", fmt.Errorf("failed to copy Briefing Template to Briefing folder: %s", err)
	}

	ctx := context.Background()
	service, err := docs.NewService(ctx)
	if err != nil {
		return "", fmt.Errorf("failed connecting to Google Docs: %s", err)
	}

	docRequest := service.Documents.Get(briefingFile.Id)
	briefingDoc, err := docRequest.Do()
	if err != nil {
		return "", fmt.Errorf("failed getting Briefing Doc: %s", err)
	}
	// json, err := briefingDoc.MarshalJSON()
	// if err != nil {
	// 	return "", fmt.Errorf("failed marshaling briefing doc to json: %s", err)
	// }
	// os.WriteFile("doc.json", json, 0600)

	updates, err := generateUpdates(conf, penalties, briefingDoc)
	if err != nil {
		return "", fmt.Errorf("failed processing Briefing Template: %s", err)
	}
	docUpdateRequest := service.Documents.BatchUpdate(briefingFile.Id, updates)

	_, err = docUpdateRequest.Do()
	if err != nil {
		return "", fmt.Errorf("could not update the Briefing Doc: %s", err)
	}

	return fmt.Sprintf("https://docs.google.com/document/d/%s", briefingFile.Id), nil
}

func GeneratePenaltyTracker(conf *config.Config) (string, error) {
	file, err := copyFile(conf.TrackerTemplateDocID, conf.TrackerFolderID, fmt.Sprintf("%s Rookies Round %d - %s", conf.Season, conf.NextRound.Number, conf.NextRound.Track))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s", file.Id), nil
}

func copyFile(template, folder, title string) (*drive.File, error) {
	driveCtx := context.Background()
	driveService, err := drive.NewService(driveCtx)
	if err != nil {
		return nil, err
	}
	copyReq := driveService.Files.Copy(template, &drive.File{
		Name:    title,
		Parents: []string{folder},
	})
	return copyReq.Do()
}

func generateUpdates(conf *config.Config, penalties *models.Penalties, doc *docs.Document) (*docs.BatchUpdateDocumentRequest, error) {
	requests := []*docs.Request{}

	// Grab index of "Stream" heading, and work backwards when building new text
	var penaltyStartIndex int64
	for _, elem := range doc.Body.Content {
		if elem.Paragraph != nil && elem.Paragraph.ParagraphStyle != nil && len(elem.Paragraph.Elements) > 0 &&
			elem.Paragraph.ParagraphStyle.NamedStyleType == "HEADING_3" &&
			strings.HasPrefix(elem.Paragraph.Elements[0].TextRun.Content, "Stream") {
			penaltyStartIndex = elem.StartIndex
		}
	}

	if penaltyStartIndex < 0 {
		return nil, fmt.Errorf("could not find H3 'Stream' to start inserting penalty data ahead of")
	}

	// Pit Starts R2
	if len(penalties.PitStartsR2CarriedOver)+len(penalties.PitStartsR2) == 0 {
		requests = append(requests, generatePenaltyEntry(penaltyStartIndex, "None!\n")...)
	} else {
		for _, driver := range penalties.PitStartsR2CarriedOver {
			requests = append(requests, generatePenaltyEntry(penaltyStartIndex, fmt.Sprintf("#%03d - %s %s (carried over)\n", driver.CarNumber, driver.FirstName, driver.LastName))...)
		}
		for _, driver := range penalties.PitStartsR2 {
			requests = append(requests, generatePenaltyEntry(penaltyStartIndex, fmt.Sprintf("#%03d - %s %s\n", driver.CarNumber, driver.FirstName, driver.LastName))...)
		}
	}
	requests = append(requests, generateHeading(penaltyStartIndex, "HEADING_4", "Race 2 Pit Starts\n")...)

	// Pit Starts R1
	if len(penalties.PitStartsR1CarriedOver)+len(penalties.PitStartsR1) == 0 {
		requests = append(requests, generatePenaltyEntry(penaltyStartIndex, "None!\n")...)
	} else {
		for _, driver := range penalties.PitStartsR1CarriedOver {
			requests = append(requests, generatePenaltyEntry(penaltyStartIndex, fmt.Sprintf("#%03d - %s %s (carried over)\n", driver.CarNumber, driver.FirstName, driver.LastName))...)
		}
		for _, driver := range penalties.PitStartsR1 {
			requests = append(requests, generatePenaltyEntry(penaltyStartIndex, fmt.Sprintf("#%03d - %s %s\n", driver.CarNumber, driver.FirstName, driver.LastName))...)
		}
	}
	requests = append(requests, generateHeading(penaltyStartIndex, "HEADING_4", "Race 1 Pit Starts\n")...)

	// Quali Bans
	if len(penalties.QualiBansCarriedOver)+len(penalties.QualiBans) == 0 {
		requests = append(requests, generatePenaltyEntry(penaltyStartIndex, "None!\n")...)
	} else {
		for _, driver := range penalties.QualiBansCarriedOver {
			requests = append(requests, generatePenaltyEntry(penaltyStartIndex, fmt.Sprintf("#%03d - %s %s (carried over)\n", driver.CarNumber, driver.FirstName, driver.LastName))...)
		}
		for _, driver := range penalties.QualiBans {
			requests = append(requests, generatePenaltyEntry(penaltyStartIndex, fmt.Sprintf("#%03d - %s %s\n", driver.CarNumber, driver.FirstName, driver.LastName))...)
		}
	}
	requests = append(requests, generateHeading(penaltyStartIndex, "HEADING_4", "Quali Bans\n")...)

	// Penalties Heading
	requests = append(requests, generateHeading(penaltyStartIndex, "HEADING_3", "Drivers Serving Penalties Tonight\n")...)

	// Now replace all templated text
	requests = append(requests, replaceText("[num]", fmt.Sprintf("%d", conf.NextRound.Number)))
	requests = append(requests, replaceText("[Track Name]", conf.NextRound.Track))

	group1 := "ODD"
	group2 := "EVEN"
	if conf.NextRound.Number%2 == 0 {
		group1 = "EVEN"
		group2 = "ODD"
	}
	requests = append(requests, replaceText("[group1]", group1))
	requests = append(requests, replaceText("[group2]", group2))
	requests = append(requests, replaceText("[briefing time]", "7:30PM Eastern/4:30PM Pacific"))
	requests = append(requests, replaceText("[SEASON]", conf.Season))

	return &docs.BatchUpdateDocumentRequest{
		Requests: requests,
	}, nil
}

func replaceText(find, replace string) *docs.Request {
	return &docs.Request{
		ReplaceAllText: &docs.ReplaceAllTextRequest{
			ContainsText: &docs.SubstringMatchCriteria{
				MatchCase: true,
				Text:      find,
			},
			ReplaceText: replace,
		},
	}
}

func generateHeading(startIndex int64, style, text string) []*docs.Request {
	var requests []*docs.Request
	requests = append(requests, &docs.Request{
		InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{
				Index: startIndex,
			},
			Text: text,
		},
	})
	requests = append(requests, &docs.Request{
		DeleteParagraphBullets: &docs.DeleteParagraphBulletsRequest{
			Range: &docs.Range{
				StartIndex: startIndex,
				EndIndex:   startIndex + int64(len(text)),
			},
		},
	})
	requests = append(requests, &docs.Request{
		UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
			Range: &docs.Range{
				StartIndex: startIndex,
				EndIndex:   startIndex + int64(len(text)),
			},
			Fields: "*",
			ParagraphStyle: &docs.ParagraphStyle{
				NamedStyleType: style,
			},
		},
	})
	return requests
}

func generatePenaltyEntry(startIndex int64, text string) []*docs.Request {
	var requests []*docs.Request
	requests = append(requests, &docs.Request{
		InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{
				Index: startIndex,
			},
			Text: text,
		},
	})
	requests = append(requests, &docs.Request{
		CreateParagraphBullets: &docs.CreateParagraphBulletsRequest{
			Range: &docs.Range{
				StartIndex: startIndex,
				EndIndex:   startIndex + int64(len(text)),
			},
			BulletPreset: "BULLET_DISC_CIRCLE_SQUARE",
		},
	})
	requests = append(requests, &docs.Request{
		UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
			Range: &docs.Range{
				StartIndex: startIndex,
				EndIndex:   startIndex + int64(len(text)),
			},
			Fields: "*",
			ParagraphStyle: &docs.ParagraphStyle{
				NamedStyleType: "NORMAL_TEXT",
				IndentFirstLine: &docs.Dimension{
					Magnitude: float64(18),
					Unit:      "PT",
				},
				IndentStart: &docs.Dimension{
					Magnitude: float64(36),
					Unit:      "PT",
				},
				SpacingMode: "COLLAPSE_LISTS",
				Direction:   "LEFT_TO_RIGHT",
			},
		},
	})
	return requests
}
