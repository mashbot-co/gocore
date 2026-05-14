package crud

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// widget is a minimal test model. UUID primary key for parity with the real
// mixin-bearing models, but no mixins — these tests exercise crud helpers
// independently of mixin behavior.
type widget struct {
	ID    uuid.UUID `gorm:"type:text;primaryKey"`
	Name  string
	Color string
}

// owner is a target type used to exercise EnrichBelongsTo.
type owner struct {
	OwnerID uuid.UUID `gorm:"type:text;primaryKey;column:owner_id"`
	Label   string
}

// itemWithOwner is a source type with a FK + association field, used by the
// EnrichBelongsTo tests.
type itemWithOwner struct {
	ID      uuid.UUID `gorm:"type:text;primaryKey"`
	OwnerID uuid.UUID `gorm:"type:text;column:owner_id"`
	Owner   *owner    `gorm:"-"`
}

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	if err := db.AutoMigrate(&widget{}, &owner{}, &itemWithOwner{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func mustCreate(t *testing.T, db *gorm.DB, w *widget) *widget {
	t.Helper()
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return w
}

// --- FindByID ---

func TestFindByID_ReturnsRecord(t *testing.T) {
	db := openDB(t)
	want := mustCreate(t, db, &widget{Name: "alpha", Color: "red"})

	got, err := FindByID[widget](context.Background(), db, want.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.ID != want.ID || got.Name != "alpha" {
		t.Errorf("got %+v, want id=%v name=alpha", got, want.ID)
	}
}

func TestFindByID_NotFoundError(t *testing.T) {
	db := openDB(t)
	if _, err := FindByID[widget](context.Background(), db, uuid.New()); err == nil {
		t.Fatal("expected error for missing record")
	}
}

func TestFindByIDWithPreload_NoPreloads(t *testing.T) {
	db := openDB(t)
	want := mustCreate(t, db, &widget{Name: "beta"})

	got, err := FindByIDWithPreload[widget](context.Background(), db, want.ID)
	if err != nil {
		t.Fatalf("FindByIDWithPreload: %v", err)
	}
	if got.Name != "beta" {
		t.Errorf("got name %q, want beta", got.Name)
	}
}

func TestFindByIDWithPreload_NotFound(t *testing.T) {
	db := openDB(t)
	if _, err := FindByIDWithPreload[widget](context.Background(), db, uuid.New(), "Anything"); err == nil {
		t.Fatal("expected error for missing record")
	}
}

// --- List / ListWithFilter / Count ---

func TestList_ReturnsAll(t *testing.T) {
	db := openDB(t)
	mustCreate(t, db, &widget{Name: "a"})
	mustCreate(t, db, &widget{Name: "b"})
	mustCreate(t, db, &widget{Name: "c"})

	got, err := List[widget](context.Background(), db)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d, want 3", len(got))
	}
}

func TestListWithFilter_NilFilterReturnsAll(t *testing.T) {
	db := openDB(t)
	mustCreate(t, db, &widget{Name: "x"})
	mustCreate(t, db, &widget{Name: "y"})

	got, err := ListWithFilter[widget](context.Background(), db, nil)
	if err != nil {
		t.Fatalf("ListWithFilter: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d, want 2", len(got))
	}
}

func TestListWithFilter_AppliesFilter(t *testing.T) {
	db := openDB(t)
	mustCreate(t, db, &widget{Name: "match", Color: "blue"})
	mustCreate(t, db, &widget{Name: "skip", Color: "red"})

	got, err := ListWithFilter[widget](context.Background(), db, &widget{Color: "blue"})
	if err != nil {
		t.Fatalf("ListWithFilter: %v", err)
	}
	if len(got) != 1 || got[0].Name != "match" {
		t.Errorf("filter mismatch: %+v", got)
	}
}

func TestCount_NilFilter(t *testing.T) {
	db := openDB(t)
	mustCreate(t, db, &widget{Name: "a"})
	mustCreate(t, db, &widget{Name: "b"})

	got, err := Count[widget](context.Background(), db, nil)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

func TestCount_WithFilter(t *testing.T) {
	db := openDB(t)
	mustCreate(t, db, &widget{Name: "a", Color: "red"})
	mustCreate(t, db, &widget{Name: "b", Color: "red"})
	mustCreate(t, db, &widget{Name: "c", Color: "blue"})

	got, err := Count[widget](context.Background(), db, &widget{Color: "red"})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

// --- Create / Update / Save / Delete ---

func TestCreate_InsertsRecord(t *testing.T) {
	db := openDB(t)
	w := &widget{ID: uuid.New(), Name: "created"}

	got, err := Create(context.Background(), db, w)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.Name != "created" {
		t.Errorf("got name %q", got.Name)
	}

	// Verify it's actually persisted.
	var found widget
	if err := db.First(&found, w.ID).Error; err != nil {
		t.Fatalf("not persisted: %v", err)
	}
}

func TestCreate_PropagatesError(t *testing.T) {
	db := openDB(t)
	id := uuid.New()
	mustCreate(t, db, &widget{ID: id, Name: "dup"})

	// Re-insert with same PK should fail.
	if _, err := Create(context.Background(), db, &widget{ID: id, Name: "second"}); err == nil {
		t.Fatal("expected duplicate-PK error")
	}
}

func TestUpdate_AppliesChanges(t *testing.T) {
	db := openDB(t)
	w := mustCreate(t, db, &widget{Name: "before", Color: "red"})

	got, err := Update[widget](context.Background(), db, w.ID, map[string]any{"name": "after"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Name != "after" || got.Color != "red" {
		t.Errorf("got %+v, want name=after color=red", got)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	db := openDB(t)
	if _, err := Update[widget](context.Background(), db, uuid.New(), map[string]any{"name": "x"}); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestSave_PersistsChanges(t *testing.T) {
	db := openDB(t)
	w := mustCreate(t, db, &widget{Name: "before"})

	w.Name = "after"
	if _, err := Save(context.Background(), db, w); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var found widget
	if err := db.First(&found, w.ID).Error; err != nil {
		t.Fatalf("re-fetch: %v", err)
	}
	if found.Name != "after" {
		t.Errorf("got %q, want after", found.Name)
	}
}

func TestDelete_RemovesRecord(t *testing.T) {
	db := openDB(t)
	w := mustCreate(t, db, &widget{Name: "doomed"})

	if err := Delete[widget](context.Background(), db, w.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var count int64
	db.Model(&widget{}).Where("id = ?", w.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after delete, got %d", count)
	}
}

func TestDelete_NotFoundWrapsError(t *testing.T) {
	db := openDB(t)
	err := Delete[widget](context.Background(), db, uuid.New())
	if err == nil {
		t.Fatal("expected error for missing record")
	}
}

// --- ListPaginated ---

func TestListPaginated_NoFilterNoPreload(t *testing.T) {
	db := openDB(t)
	for i := 0; i < 5; i++ {
		mustCreate(t, db, &widget{Name: "n"})
	}

	page, err := ListPaginated[widget](context.Background(), db, nil, 2, 0)
	if err != nil {
		t.Fatalf("ListPaginated: %v", err)
	}
	if page.TotalCount != 5 {
		t.Errorf("TotalCount: got %d, want 5", page.TotalCount)
	}
	if len(page.Items) != 2 {
		t.Errorf("Items: got %d, want 2", len(page.Items))
	}
}

func TestListPaginated_AppliesFilter(t *testing.T) {
	db := openDB(t)
	mustCreate(t, db, &widget{Name: "a", Color: "red"})
	mustCreate(t, db, &widget{Name: "b", Color: "red"})
	mustCreate(t, db, &widget{Name: "c", Color: "blue"})

	page, err := ListPaginated[widget](context.Background(), db, &widget{Color: "red"}, 10, 0)
	if err != nil {
		t.Fatalf("ListPaginated: %v", err)
	}
	if page.TotalCount != 2 {
		t.Errorf("TotalCount: got %d, want 2", page.TotalCount)
	}
}

func TestListPaginated_AppliesOffset(t *testing.T) {
	db := openDB(t)
	for i := 0; i < 5; i++ {
		mustCreate(t, db, &widget{Name: "n"})
	}

	page, err := ListPaginated[widget](context.Background(), db, nil, 2, 3)
	if err != nil {
		t.Fatalf("ListPaginated: %v", err)
	}
	if len(page.Items) != 2 {
		t.Errorf("expected 2 items (offset 3 of 5), got %d", len(page.Items))
	}
}

func TestListPaginated_DefaultsLimitWhenZeroOrNegative(t *testing.T) {
	db := openDB(t)
	for i := 0; i < 3; i++ {
		mustCreate(t, db, &widget{Name: "n"})
	}

	// limit=0 should fall through to the default of 25, so all 3 items return.
	page, err := ListPaginated[widget](context.Background(), db, nil, 0, 0)
	if err != nil {
		t.Fatalf("ListPaginated: %v", err)
	}
	if len(page.Items) != 3 {
		t.Errorf("got %d items, want 3", len(page.Items))
	}
}

// --- EnrichBelongsTo ---

func TestEnrichBelongsTo_AssignsLoadedAssociations(t *testing.T) {
	db := openDB(t)

	o1 := &owner{OwnerID: uuid.New(), Label: "first"}
	o2 := &owner{OwnerID: uuid.New(), Label: "second"}
	if err := db.Create(o1).Error; err != nil {
		t.Fatalf("seed o1: %v", err)
	}
	if err := db.Create(o2).Error; err != nil {
		t.Fatalf("seed o2: %v", err)
	}

	items := []itemWithOwner{
		{ID: uuid.New(), OwnerID: o1.OwnerID},
		{ID: uuid.New(), OwnerID: o2.OwnerID},
	}

	EnrichBelongsTo(context.Background(), db, items, "OwnerID", "Owner", (*owner)(nil), "OwnerID", "owner_id")

	if items[0].Owner == nil || items[0].Owner.Label != "first" {
		t.Errorf("items[0].Owner: got %+v, want label=first", items[0].Owner)
	}
	if items[1].Owner == nil || items[1].Owner.Label != "second" {
		t.Errorf("items[1].Owner: got %+v, want label=second", items[1].Owner)
	}
}

func TestEnrichBelongsTo_EmptyItemsIsNoOp(t *testing.T) {
	db := openDB(t)
	var items []itemWithOwner
	// Should not panic.
	EnrichBelongsTo(context.Background(), db, items, "OwnerID", "Owner", (*owner)(nil), "OwnerID", "owner_id")
}

func TestEnrichBelongsTo_NilFKsLeadToNoLookup(t *testing.T) {
	db := openDB(t)
	items := []itemWithOwner{
		{ID: uuid.New(), OwnerID: uuid.Nil},
		{ID: uuid.New(), OwnerID: uuid.Nil},
	}
	EnrichBelongsTo(context.Background(), db, items, "OwnerID", "Owner", (*owner)(nil), "OwnerID", "owner_id")

	for i, it := range items {
		if it.Owner != nil {
			t.Errorf("items[%d].Owner: got %+v, want nil", i, it.Owner)
		}
	}
}

func TestEnrichBelongsTo_UnknownFKFieldReturnsEarly(t *testing.T) {
	db := openDB(t)
	items := []itemWithOwner{{ID: uuid.New(), OwnerID: uuid.New()}}
	// Misspelled FK field — function should detect and bail without panic.
	EnrichBelongsTo(context.Background(), db, items, "NotAField", "Owner", (*owner)(nil), "OwnerID", "owner_id")

	if items[0].Owner != nil {
		t.Errorf("expected Owner to remain nil, got %+v", items[0].Owner)
	}
}

// --- fieldByPath ---

func TestFieldByPath_DirectField(t *testing.T) {
	type s struct{ X int }
	v := reflect.ValueOf(s{X: 42})
	got := fieldByPath(v, "Missing", "X")
	if !got.IsValid() || got.Int() != 42 {
		t.Errorf("got %v, want 42", got)
	}
}

func TestFieldByPath_DescendsIntoEmbedded(t *testing.T) {
	type inner struct{ Y int }
	type outer struct {
		Embedded inner
	}
	v := reflect.ValueOf(outer{Embedded: inner{Y: 99}})
	got := fieldByPath(v, "Embedded", "Y")
	if !got.IsValid() || got.Int() != 99 {
		t.Errorf("got %v, want 99", got)
	}
}
