package mixins

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// VersionedMixin adds fields for immutable, append-only version records:
// version_label, status, authored_by, change_summary, policy_check_*.
// It carries no behavior (no callbacks) — it's a data-shape mixin.

func TestVersionedMixin_FieldsExist(t *testing.T) {
	// Compile-time guarantee — failing build means the mixin shape changed.
	v := VersionedMixin{}
	_ = v.VersionLabel
	_ = v.Status
	_ = v.AuthoredBy
	_ = v.ChangeSummary
	_ = v.PolicyCheckStatus
	_ = v.PolicyCheckAt
	_ = v.PolicyCheckNote
}

func TestVersionedMixin_DefaultStatusIsDraft(t *testing.T) {
	type versionedSample struct {
		ID uuid.UUID `gorm:"type:text;primaryKey"`
		BaseModel
		VersionedMixin
	}

	db := setupTestDB(t)
	if err := db.AutoMigrate(&versionedSample{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	v := &versionedSample{
		ID: uuid.New(),
		VersionedMixin: VersionedMixin{
			VersionLabel: "v1",
			AuthoredBy:   uuid.New(),
		},
	}
	if err := db.Create(v).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	if v.Status != "draft" {
		t.Errorf("expected default Status='draft', got %q", v.Status)
	}
}

func TestVersionedMixin_PolicyCheckNullable(t *testing.T) {
	type versionedSample struct {
		ID uuid.UUID `gorm:"type:text;primaryKey"`
		BaseModel
		VersionedMixin
	}

	db := setupTestDB(t)
	if err := db.AutoMigrate(&versionedSample{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	v := &versionedSample{
		ID: uuid.New(),
		VersionedMixin: VersionedMixin{
			VersionLabel: "v1",
			AuthoredBy:   uuid.New(),
		},
	}
	if err := db.Create(v).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// PolicyCheck* fields are pointers and should default to nil.
	if v.PolicyCheckStatus != nil {
		t.Errorf("expected nil PolicyCheckStatus by default, got %v", *v.PolicyCheckStatus)
	}
	if v.PolicyCheckAt != nil {
		t.Errorf("expected nil PolicyCheckAt by default, got %v", *v.PolicyCheckAt)
	}
	if v.PolicyCheckNote != nil {
		t.Errorf("expected nil PolicyCheckNote by default, got %v", *v.PolicyCheckNote)
	}
}

func TestVersionedMixin_PolicyCheckFieldsCanBePopulated(t *testing.T) {
	type versionedSample struct {
		ID uuid.UUID `gorm:"type:text;primaryKey"`
		BaseModel
		VersionedMixin
	}

	db := setupTestDB(t)
	if err := db.AutoMigrate(&versionedSample{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	status := "passed"
	note := "all good"
	checkedAt := time.Now()

	v := &versionedSample{
		ID: uuid.New(),
		VersionedMixin: VersionedMixin{
			VersionLabel:      "v1",
			AuthoredBy:        uuid.New(),
			PolicyCheckStatus: &status,
			PolicyCheckAt:     &checkedAt,
			PolicyCheckNote:   &note,
		},
	}
	if err := db.Create(v).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var fresh versionedSample
	if err := db.First(&fresh, "id = ?", v.ID).Error; err != nil {
		t.Fatalf("reread: %v", err)
	}
	if fresh.PolicyCheckStatus == nil || *fresh.PolicyCheckStatus != "passed" {
		t.Errorf("expected PolicyCheckStatus=passed, got %v", fresh.PolicyCheckStatus)
	}
}
