package mixins

import (
	"reflect"

	"github.com/mashbot-co/gocore/db/connection"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func init() {
	connection.OnInitialize(RegisterProjectCallbacks)
}

// ProjectMixin adds project-scoped isolation. Embed in models that belong to a
// single project. Project scoping is enforced automatically via GORM callbacks:
//   - On create: project_id is set from the request context
//   - On query/update/delete: a WHERE project_id = ? clause is added automatically
//
// The project ID must be set on the context via connection.WithCurrentProject()
// before any database operation. The auth middleware handles this.
type ProjectMixin struct {
	ProjectID uuid.UUID `gorm:"index;type:uuid;not null" json:"project_id"`
}

// ForProject returns a GORM scope that filters by project_id.
// This is available for manual use but is applied automatically by callbacks.
func ForProject(projectID uuid.UUID) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("project_id = ?", projectID)
	}
}

// RegisterProjectCallbacks registers GORM callbacks that automatically enforce
// project isolation on any model embedding ProjectMixin.
func RegisterProjectCallbacks(gormDB *gorm.DB) {
	gormDB.Callback().Create().Before("gorm:create").Register("project:before_create", projectBeforeCreate)
	gormDB.Callback().Query().Before("gorm:query").Register("project:before_query", projectBeforeQuery)
	gormDB.Callback().Update().Before("gorm:update").Register("project:before_update", projectBeforeUpdate)
	gormDB.Callback().Delete().Before("gorm:delete").Register("project:before_delete", projectBeforeDelete)
}

func projectBeforeCreate(tx *gorm.DB) {
	if tx.Error != nil || tx.Statement.Dest == nil {
		return
	}
	project := findProjectMixin(tx.Statement.Dest)
	if project == nil {
		return
	}
	if project.ProjectID == uuid.Nil {
		if projectID := connection.CurrentProject(tx.Statement.Context); projectID != uuid.Nil {
			project.ProjectID = projectID
		}
	}
}

func projectBeforeQuery(tx *gorm.DB) {
	if tx.Error != nil {
		return
	}
	if !isProjectScopedModel(tx) {
		return
	}
	projectID := connection.CurrentProject(tx.Statement.Context)
	if projectID == uuid.Nil {
		return
	}
	tx.Statement.AddClauseIfNotExists(clause.Where{
		Exprs: []clause.Expression{
			clause.Eq{Column: clause.Column{Name: "project_id"}, Value: projectID},
		},
	})
}

func projectBeforeUpdate(tx *gorm.DB) {
	if tx.Error != nil {
		return
	}
	if !isProjectScopedModel(tx) {
		return
	}
	projectID := connection.CurrentProject(tx.Statement.Context)
	if projectID == uuid.Nil {
		return
	}
	tx.Statement.AddClauseIfNotExists(clause.Where{
		Exprs: []clause.Expression{
			clause.Eq{Column: clause.Column{Name: "project_id"}, Value: projectID},
		},
	})
}

func projectBeforeDelete(tx *gorm.DB) {
	if tx.Error != nil {
		return
	}
	if !isProjectScopedModel(tx) {
		return
	}
	projectID := connection.CurrentProject(tx.Statement.Context)
	if projectID == uuid.Nil {
		return
	}
	tx.Statement.AddClauseIfNotExists(clause.Where{
		Exprs: []clause.Expression{
			clause.Eq{Column: clause.Column{Name: "project_id"}, Value: projectID},
		},
	})
}

func isProjectScopedModel(tx *gorm.DB) bool {
	if tx.Statement.Model != nil {
		return hasProjectMixin(tx.Statement.Model)
	}
	if tx.Statement.Dest != nil {
		return hasProjectMixin(tx.Statement.Dest)
	}
	return false
}

func hasProjectMixin(dest interface{}) bool {
	t := reflect.TypeOf(dest)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Slice {
		t = t.Elem()
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	_, found := t.FieldByName("ProjectMixin")
	return found
}

func findProjectMixin(dest interface{}) *ProjectMixin {
	val := reflect.ValueOf(dest)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil
	}
	field := val.FieldByName("ProjectMixin")
	if !field.IsValid() || !field.CanAddr() {
		return nil
	}
	if project, ok := field.Addr().Interface().(*ProjectMixin); ok {
		return project
	}
	return nil
}
