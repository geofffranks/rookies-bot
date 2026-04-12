package gcloud_test

import (
	"errors"
	"fmt"
	"strings"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/gcloud"
	"github.com/geofffranks/rookies-bot/gcloud/fakes"
	"github.com/geofffranks/rookies-bot/models"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
)

// makeDoc builds a minimal docs.Document with a HEADING_3 "Stream" element
// at the given start index for testing generateUpdates.
func makeDoc(streamIndex int64) *docs.Document {
	return &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					StartIndex: streamIndex,
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{
							NamedStyleType: "HEADING_3",
						},
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Stream\n"}},
						},
					},
				},
			},
		},
	}
}

// insertedTexts extracts all InsertText content values from a BatchUpdateDocumentRequest.
func insertedTexts(req *docs.BatchUpdateDocumentRequest) []string {
	var texts []string
	for _, r := range req.Requests {
		if r.InsertText != nil {
			texts = append(texts, r.InsertText.Text)
		}
	}
	return texts
}

var _ = Describe("Client", func() {
	var (
		fakeDocsService  *fakes.FakeDocsServicer
		fakeDriveService *fakes.FakeDriveServicer
		client           *gcloud.Client
		conf             *config.Config
		penalties        *models.Penalties
	)

	BeforeEach(func() {
		fakeDocsService = new(fakes.FakeDocsServicer)
		fakeDriveService = new(fakes.FakeDriveServicer)
		client = &gcloud.Client{
			Docs:  fakeDocsService,
			Drive: fakeDriveService,
		}
		conf = &config.Config{
			BotConfig: config.BotConfig{
				Season:                "2026",
				BriefingTemplateDocID: "tmpl-doc-id",
				BriefingFolderID:      "briefing-folder-id",
				TrackerTemplateDocID:  "tmpl-tracker-id",
				TrackerFolderID:       "tracker-folder-id",
			},
			RoundConfig: config.RoundConfig{
				NextRound:     config.Round{Number: 5, Track: "Monza"},
				PreviousRound: config.Round{Number: 4, Track: "Spa"},
			},
		}
		penalties = &models.Penalties{}
	})

	Describe("GeneratePenaltyTracker", func() {
		It("copies the tracker template and returns a spreadsheet URL", func() {
			fakeDriveService.CopyFileReturns(&drive.File{Id: "new-tracker-id"}, nil)

			url, err := client.GeneratePenaltyTracker(conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("https://docs.google.com/spreadsheets/d/new-tracker-id"))

			Expect(fakeDriveService.CopyFileCallCount()).To(Equal(1))
			_, templateID, folderID, title := fakeDriveService.CopyFileArgsForCall(0)
			Expect(templateID).To(Equal("tmpl-tracker-id"))
			Expect(folderID).To(Equal("tracker-folder-id"))
			Expect(title).To(Equal("2026 Rookies Round 5 - Monza"))
		})

		It("returns an error when Drive fails", func() {
			fakeDriveService.CopyFileReturns(nil, errors.New("drive unavailable"))
			_, err := client.GeneratePenaltyTracker(conf)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("drive unavailable"))
		})
	})

	Describe("GenerateBriefing", func() {
		BeforeEach(func() {
			fakeDriveService.CopyFileReturns(&drive.File{Id: "new-briefing-id"}, nil)
			fakeDocsService.GetDocumentReturns(makeDoc(10), nil)
			fakeDocsService.BatchUpdateDocumentReturns(&docs.BatchUpdateDocumentResponse{}, nil)
		})

		It("copies the template, fetches the doc, sends updates, returns URL", func() {
			url, err := client.GenerateBriefing(conf, penalties)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("https://docs.google.com/document/d/new-briefing-id"))

			Expect(fakeDriveService.CopyFileCallCount()).To(Equal(1))
			_, templateID, folderID, title := fakeDriveService.CopyFileArgsForCall(0)
			Expect(templateID).To(Equal("tmpl-doc-id"))
			Expect(folderID).To(Equal("briefing-folder-id"))
			Expect(title).To(Equal("Drivers Briefing Round 5 at Monza"))

			Expect(fakeDocsService.GetDocumentCallCount()).To(Equal(1))
			_, docID := fakeDocsService.GetDocumentArgsForCall(0)
			Expect(docID).To(Equal("new-briefing-id"))

			Expect(fakeDocsService.BatchUpdateDocumentCallCount()).To(Equal(1))
		})

		It("returns an error when Drive copy fails", func() {
			fakeDriveService.CopyFileReturns(nil, errors.New("copy failed"))
			_, err := client.GenerateBriefing(conf, penalties)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("copy failed"))
		})

		It("returns an error when GetDocument fails", func() {
			fakeDocsService.GetDocumentReturns(nil, errors.New("docs api down"))
			_, err := client.GenerateBriefing(conf, penalties)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("docs api down"))
		})

		It("returns an error when BatchUpdate fails", func() {
			fakeDocsService.BatchUpdateDocumentReturns(nil, errors.New("batch failed"))
			_, err := client.GenerateBriefing(conf, penalties)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("batch failed"))
		})
	})
})

var _ = Describe("generateUpdates (via GenerateBriefing)", func() {
	var (
		fakeDocsService  *fakes.FakeDocsServicer
		fakeDriveService *fakes.FakeDriveServicer
		client           *gcloud.Client
		conf             *config.Config
	)

	BeforeEach(func() {
		fakeDocsService = new(fakes.FakeDocsServicer)
		fakeDriveService = new(fakes.FakeDriveServicer)
		client = &gcloud.Client{Docs: fakeDocsService, Drive: fakeDriveService}
		fakeDriveService.CopyFileReturns(&drive.File{Id: "doc-id"}, nil)
		fakeDocsService.GetDocumentReturns(makeDoc(5), nil)
		fakeDocsService.BatchUpdateDocumentReturns(&docs.BatchUpdateDocumentResponse{}, nil)
		conf = &config.Config{
			BotConfig: config.BotConfig{Season: "2026"},
			RoundConfig: config.RoundConfig{
				NextRound: config.Round{Number: 4, Track: "Silverstone"},
			},
		}
	})

	It("includes a replaceText request for [num] with the round number", func() {
		_, err := client.GenerateBriefing(conf, &models.Penalties{})
		Expect(err).NotTo(HaveOccurred())

		_, _, req := fakeDocsService.BatchUpdateDocumentArgsForCall(0)
		texts := make([]string, 0)
		for _, r := range req.Requests {
			if r.ReplaceAllText != nil {
				texts = append(texts, fmt.Sprintf("%s->%s", r.ReplaceAllText.ContainsText.Text, r.ReplaceAllText.ReplaceText))
			}
		}
		Expect(texts).To(ContainElement("[num]->4"))
		Expect(texts).To(ContainElement("[Track Name]->Silverstone"))
		Expect(texts).To(ContainElement("[SEASON]->2026"))
	})

	It("sets group1=ODD and group2=EVEN for odd round numbers", func() {
		conf.NextRound.Number = 3
		_, err := client.GenerateBriefing(conf, &models.Penalties{})
		Expect(err).NotTo(HaveOccurred())

		_, _, req := fakeDocsService.BatchUpdateDocumentArgsForCall(0)
		texts := make([]string, 0)
		for _, r := range req.Requests {
			if r.ReplaceAllText != nil {
				texts = append(texts, fmt.Sprintf("%s->%s", r.ReplaceAllText.ContainsText.Text, r.ReplaceAllText.ReplaceText))
			}
		}
		Expect(texts).To(ContainElement("[group1]->ODD"))
		Expect(texts).To(ContainElement("[group2]->EVEN"))
	})

	It("sets group1=EVEN and group2=ODD for even round numbers", func() {
		conf.NextRound.Number = 4
		_, err := client.GenerateBriefing(conf, &models.Penalties{})
		Expect(err).NotTo(HaveOccurred())

		_, _, req := fakeDocsService.BatchUpdateDocumentArgsForCall(0)
		texts := make([]string, 0)
		for _, r := range req.Requests {
			if r.ReplaceAllText != nil {
				texts = append(texts, fmt.Sprintf("%s->%s", r.ReplaceAllText.ContainsText.Text, r.ReplaceAllText.ReplaceText))
			}
		}
		Expect(texts).To(ContainElement("[group1]->EVEN"))
		Expect(texts).To(ContainElement("[group2]->ODD"))
	})
})

var _ = Describe("generateUpdates carried-over penalties", func() {
	var (
		fakeDocsService  *fakes.FakeDocsServicer
		fakeDriveService *fakes.FakeDriveServicer
		client           *gcloud.Client
		conf             *config.Config
	)

	BeforeEach(func() {
		fakeDocsService = new(fakes.FakeDocsServicer)
		fakeDriveService = new(fakes.FakeDriveServicer)
		client = &gcloud.Client{Docs: fakeDocsService, Drive: fakeDriveService}
		fakeDriveService.CopyFileReturns(&drive.File{Id: "doc-id"}, nil)
		fakeDocsService.GetDocumentReturns(makeDoc(5), nil)
		fakeDocsService.BatchUpdateDocumentReturns(&docs.BatchUpdateDocumentResponse{}, nil)
		conf = &config.Config{
			BotConfig: config.BotConfig{Season: "2026"},
			RoundConfig: config.RoundConfig{
				NextRound: config.Round{Number: 4, Track: "Silverstone"},
			},
		}
	})

	getCapturedTexts := func() []string {
		Expect(fakeDocsService.BatchUpdateDocumentCallCount()).To(BeNumerically(">", 0))
		_, _, req := fakeDocsService.BatchUpdateDocumentArgsForCall(0)
		return insertedTexts(req)
	}

	It("includes '(carried over)' for QualiBansR1CarriedOver driver", func() {
		_, err := client.GenerateBriefing(conf, &models.Penalties{
			QualiBansR1CarriedOver: []models.Driver{{FirstName: "Alice", LastName: "Anderson", CarNumber: 1}},
		})
		Expect(err).NotTo(HaveOccurred())
		texts := getCapturedTexts()
		Expect(strings.Join(texts, " ")).To(ContainSubstring("Alice"))
		Expect(strings.Join(texts, " ")).To(ContainSubstring("carried over"))
	})

	It("includes '(carried over)' for QualiBansR2CarriedOver driver", func() {
		_, err := client.GenerateBriefing(conf, &models.Penalties{
			QualiBansR2CarriedOver: []models.Driver{{FirstName: "Bob", LastName: "Brown", CarNumber: 2}},
		})
		Expect(err).NotTo(HaveOccurred())
		texts := getCapturedTexts()
		Expect(strings.Join(texts, " ")).To(ContainSubstring("Bob"))
		Expect(strings.Join(texts, " ")).To(ContainSubstring("carried over"))
	})

	It("includes '(carried over)' for PitStartsR1CarriedOver driver", func() {
		_, err := client.GenerateBriefing(conf, &models.Penalties{
			PitStartsR1CarriedOver: []models.Driver{{FirstName: "Carol", LastName: "Chen", CarNumber: 3}},
		})
		Expect(err).NotTo(HaveOccurred())
		texts := getCapturedTexts()
		Expect(strings.Join(texts, " ")).To(ContainSubstring("Carol"))
		Expect(strings.Join(texts, " ")).To(ContainSubstring("carried over"))
	})

	It("includes '(carried over)' for PitStartsR2CarriedOver driver", func() {
		_, err := client.GenerateBriefing(conf, &models.Penalties{
			PitStartsR2CarriedOver: []models.Driver{{FirstName: "Dave", LastName: "Davis", CarNumber: 4}},
		})
		Expect(err).NotTo(HaveOccurred())
		texts := getCapturedTexts()
		Expect(strings.Join(texts, " ")).To(ContainSubstring("Dave"))
		Expect(strings.Join(texts, " ")).To(ContainSubstring("carried over"))
	})

	It("includes driver without '(carried over)' for QualiBansR1", func() {
		_, err := client.GenerateBriefing(conf, &models.Penalties{
			QualiBansR1: []models.Driver{{FirstName: "Eve", LastName: "Edwards", CarNumber: 5}},
		})
		Expect(err).NotTo(HaveOccurred())
		texts := getCapturedTexts()
		joined := strings.Join(texts, " ")
		Expect(joined).To(ContainSubstring("Eve"))
		Expect(joined).NotTo(ContainSubstring("carried over"))
	})

	It("includes driver without '(carried over)' for QualiBansR2", func() {
		_, err := client.GenerateBriefing(conf, &models.Penalties{
			QualiBansR2: []models.Driver{{FirstName: "Frank", LastName: "Flynn", CarNumber: 6}},
		})
		Expect(err).NotTo(HaveOccurred())
		texts := getCapturedTexts()
		joined := strings.Join(texts, " ")
		Expect(joined).To(ContainSubstring("Frank"))
		Expect(joined).NotTo(ContainSubstring("carried over"))
	})

	It("includes driver without '(carried over)' for PitStartsR1", func() {
		_, err := client.GenerateBriefing(conf, &models.Penalties{
			PitStartsR1: []models.Driver{{FirstName: "Grace", LastName: "Green", CarNumber: 7}},
		})
		Expect(err).NotTo(HaveOccurred())
		texts := getCapturedTexts()
		joined := strings.Join(texts, " ")
		Expect(joined).To(ContainSubstring("Grace"))
		Expect(joined).NotTo(ContainSubstring("carried over"))
	})

	It("includes driver without '(carried over)' for PitStartsR2", func() {
		_, err := client.GenerateBriefing(conf, &models.Penalties{
			PitStartsR2: []models.Driver{{FirstName: "Hank", LastName: "Harris", CarNumber: 8}},
		})
		Expect(err).NotTo(HaveOccurred())
		texts := getCapturedTexts()
		joined := strings.Join(texts, " ")
		Expect(joined).To(ContainSubstring("Hank"))
		Expect(joined).NotTo(ContainSubstring("carried over"))
	})

	// BUG DOCUMENTATION: penaltyStartIndex is int64, starts at 0, and the guard
	// is `if penaltyStartIndex < 0`. Since 0 is never < 0, a doc with no Stream
	// heading silently uses index 0 instead of returning an error. This test
	// documents the current (buggy) behavior so any future fix is caught.
	It("silently uses index 0 when doc has no Stream heading (documents bug)", func() {
		noStreamDoc := &docs.Document{
			Body: &docs.Body{
				Content: []*docs.StructuralElement{
					{
						Paragraph: &docs.Paragraph{
							Elements: []*docs.ParagraphElement{
								{TextRun: &docs.TextRun{Content: "Some other heading\n"}},
							},
							ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_3"},
						},
						StartIndex: 1,
						EndIndex:   20,
					},
				},
			},
		}
		fakeDocsService.GetDocumentReturns(noStreamDoc, nil)
		fakeDocsService.BatchUpdateDocumentReturns(&docs.BatchUpdateDocumentResponse{}, nil)

		// BUG: should return error "no Stream heading found", but currently succeeds
		_, err := client.GenerateBriefing(conf, &models.Penalties{})
		Expect(err).NotTo(HaveOccurred()) // documents current buggy behavior
	})
})
