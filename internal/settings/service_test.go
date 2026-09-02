package settings

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

func TestUpdateValidatesClosedRegistryAndNormalizesStrings(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)
	actor := auth.Principal{UserID: "admin-1"}

	settings, err := service.Update(context.Background(), actor, map[string]string{
		"application_name":  "  Konkit Nasional  ",
		"timezone":          "Asia/Makassar",
		"date_format":       "02/01/2006",
		"locale":            "id-ID",
		"organization_name": "  PT Kian Santang Muliatama Tbk  ",
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if repository.values["application_name"] != "Konkit Nasional" || repository.values["organization_name"] != "PT Kian Santang Muliatama Tbk" {
		t.Fatalf("values were not normalized: %+v", repository.values)
	}
	if len(settings) != 5 {
		t.Fatalf("expected updated settings, got %d", len(settings))
	}
}

func TestUpdateRejectsUnknownAndInvalidValuesBeforePersistence(t *testing.T) {
	tests := []map[string]string{
		{"database_url": "postgres://secret"},
		{"timezone": "UTC"},
		{"locale": "en-US"},
		{"application_name": ""},
	}
	for _, values := range tests {
		repository := &fakeRepository{}
		_, err := NewService(repository).Update(context.Background(), auth.Principal{UserID: "admin-1"}, values, auth.ClientMeta{})
		if !errors.Is(err, ErrInvalidSetting) {
			t.Fatalf("expected ErrInvalidSetting for %+v, got %v", values, err)
		}
		if repository.values != nil {
			t.Fatal("invalid setting reached repository")
		}
	}
}

type fakeRepository struct {
	values map[string]string
}

func (f *fakeRepository) List(context.Context) ([]Setting, error) {
	return []Setting{}, nil
}

func (f *fakeRepository) Update(_ context.Context, _ auth.Principal, values map[string]string, _ auth.ClientMeta) ([]Setting, error) {
	f.values = values
	result := make([]Setting, 0, len(values))
	for key, value := range values {
		result = append(result, Setting{Key: key, Value: value})
	}
	return result, nil
}
