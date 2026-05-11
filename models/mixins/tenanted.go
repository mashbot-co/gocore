package mixins

import (
	"reflect"

	"github.com/mashbot-co/gocore/connection"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func init() {
	connection.OnInitialize(RegisterTenantCallbacks)
}

// TenantMixin adds multi-tenant support. Embed in tenanted models.
// Tenant scoping is enforced automatically via GORM callbacks:
//   - On create: tenant_id is set from the request context
//   - On query/update/delete: a WHERE tenant_id = ? clause is added automatically
//
// The tenant ID must be set on the context via connection.WithCurrentTenant()
// before any database operation. The auth middleware handles this.
type TenantMixin struct {
	TenantID uuid.UUID `gorm:"index;type:uuid;not null" json:"tenant_id"`
}

// ForTenant returns a GORM scope that filters by tenant_id.
// This is available for manual use but is applied automatically by callbacks.
func ForTenant(tenantID uuid.UUID) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("tenant_id = ?", tenantID)
	}
}

// RegisterTenantCallbacks registers GORM callbacks that automatically enforce
// tenant isolation on any model embedding TenantMixin.
func RegisterTenantCallbacks(gormDB *gorm.DB) {
	gormDB.Callback().Create().Before("gorm:create").Register("tenant:before_create", tenantBeforeCreate)
	gormDB.Callback().Query().Before("gorm:query").Register("tenant:before_query", tenantBeforeQuery)
	gormDB.Callback().Update().Before("gorm:update").Register("tenant:before_update", tenantBeforeUpdate)
	gormDB.Callback().Delete().Before("gorm:delete").Register("tenant:before_delete", tenantBeforeDelete)
}

// tenantBeforeCreate sets the tenant_id from context on new records.
func tenantBeforeCreate(tx *gorm.DB) {
	if tx.Error != nil || tx.Statement.Dest == nil {
		return
	}
	tenant := findTenantMixin(tx.Statement.Dest)
	if tenant == nil {
		return
	}
	// Only set from context if the tenant_id isn't already set
	if tenant.TenantID == uuid.Nil {
		if tenantID := connection.CurrentTenant(tx.Statement.Context); tenantID != uuid.Nil {
			tenant.TenantID = tenantID
		}
	}
}

// tenantBeforeQuery adds WHERE tenant_id = ? to all queries on tenanted models.
func tenantBeforeQuery(tx *gorm.DB) {
	if tx.Error != nil {
		return
	}
	if !isTenantedModel(tx) {
		return
	}
	tenantID := connection.CurrentTenant(tx.Statement.Context)
	if tenantID == uuid.Nil {
		return
	}
	tx.Statement.AddClauseIfNotExists(clause.Where{
		Exprs: []clause.Expression{
			clause.Eq{Column: clause.Column{Name: "tenant_id"}, Value: tenantID},
		},
	})
}

// tenantBeforeUpdate adds WHERE tenant_id = ? to all updates on tenanted models.
func tenantBeforeUpdate(tx *gorm.DB) {
	if tx.Error != nil {
		return
	}
	if !isTenantedModel(tx) {
		return
	}
	tenantID := connection.CurrentTenant(tx.Statement.Context)
	if tenantID == uuid.Nil {
		return
	}
	tx.Statement.AddClauseIfNotExists(clause.Where{
		Exprs: []clause.Expression{
			clause.Eq{Column: clause.Column{Name: "tenant_id"}, Value: tenantID},
		},
	})
}

// tenantBeforeDelete adds WHERE tenant_id = ? to all deletes on tenanted models.
func tenantBeforeDelete(tx *gorm.DB) {
	if tx.Error != nil {
		return
	}
	if !isTenantedModel(tx) {
		return
	}
	tenantID := connection.CurrentTenant(tx.Statement.Context)
	if tenantID == uuid.Nil {
		return
	}
	tx.Statement.AddClauseIfNotExists(clause.Where{
		Exprs: []clause.Expression{
			clause.Eq{Column: clause.Column{Name: "tenant_id"}, Value: tenantID},
		},
	})
}

// isTenantedModel checks whether the query's model embeds TenantMixin.
func isTenantedModel(tx *gorm.DB) bool {
	if tx.Statement.Model != nil {
		return hasTenantMixin(tx.Statement.Model)
	}
	if tx.Statement.Dest != nil {
		return hasTenantMixin(tx.Statement.Dest)
	}
	return false
}

func hasTenantMixin(dest interface{}) bool {
	t := reflect.TypeOf(dest)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	// Handle slices (e.g., []Agent or []*Agent)
	if t.Kind() == reflect.Slice {
		t = t.Elem()
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	_, found := t.FieldByName("TenantMixin")
	return found
}

// findTenantMixin uses reflection to find an embedded TenantMixin in a model.
func findTenantMixin(dest interface{}) *TenantMixin {
	val := reflect.ValueOf(dest)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil
	}
	field := val.FieldByName("TenantMixin")
	if !field.IsValid() || !field.CanAddr() {
		return nil
	}
	if tenant, ok := field.Addr().Interface().(*TenantMixin); ok {
		return tenant
	}
	return nil
}
