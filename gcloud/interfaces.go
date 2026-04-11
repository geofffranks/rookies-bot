package gcloud

import (
	"context"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate . DocsServicer
type DocsServicer interface {
	GetDocument(ctx context.Context, id string) (*docs.Document, error)
	BatchUpdateDocument(ctx context.Context, id string, req *docs.BatchUpdateDocumentRequest) (*docs.BatchUpdateDocumentResponse, error)
}

//counterfeiter:generate . DriveServicer
type DriveServicer interface {
	CopyFile(ctx context.Context, templateID, folderID, title string) (*drive.File, error)
}
