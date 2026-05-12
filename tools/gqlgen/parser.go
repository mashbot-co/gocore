package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// ModelDef represents a parsed Go model struct.
type ModelDef struct {
	Name          string
	Description   string
	Fields        []FieldDef
	Relationships []RelationshipDef
	Mixins        []string
	Mutations     []MutationDef
	// SkipCRUD suppresses generation of the auto Create/Update/Delete resolver
	// methods for this model. The schema mutations are still emitted (callers
	// rely on them), so a hand-written resolver file in the resolver package
	// must provide the corresponding CreateX/UpdateX/DeleteX methods.
	// Set by `// graphql:skipCrud` on the struct doc.
	SkipCRUD bool
}

// MutationDef represents a GraphQL mutation method on a model.
// Parsed from methods annotated with // graphql:mutation comments.
type MutationDef struct {
	MethodName  string     // Go method name (e.g., "Promote")
	Description string     // Doc comment
	Params      []ParamDef // GraphQL-exposed parameters (excluding receiver and context-injected ones)
}

// ParamDef represents a parameter on a mutation method.
type ParamDef struct {
	Name   string // GraphQL param name (e.g., "targetStatus")
	GoType string // Go type (e.g., "string")
}

// StaticMutationDef represents a class-level (package function) GraphQL mutation.
// Parsed from functions annotated with // graphql:static comments.
type StaticMutationDef struct {
	FuncName     string     // Go function name
	MutationName string     // GraphQL mutation name (from annotation)
	Description  string     // Doc comment
	Params       []ParamDef // GraphQL-exposed parameters
	ReturnType   string     // GraphQL return type name
	ReturnsList  bool       // Whether the return is a list
	PackageName  string     // Go package name (e.g., "tenant")
	ImportPath   string     // Go import path (e.g., "github.com/mashbot-co/gocore/services/tenant")
	IsPublic     bool       // Whether this endpoint is public (default: protected)
}

// StaticQueryDef represents a class-level (package function) GraphQL query.
type StaticQueryDef struct {
	FuncName    string     // Go function name
	QueryName   string     // GraphQL query name (from annotation)
	Description string     // Doc comment
	Params      []ParamDef // GraphQL-exposed parameters
	ReturnType  string     // GraphQL return type name
	ReturnsList bool       // Whether the return is a list
	PackageName string     // Go package name (e.g., "tenant")
	ImportPath  string     // Go import path (e.g., "github.com/mashbot-co/gocore/services/tenant")
	IsPublic    bool       // Whether this endpoint is public (default: protected)
}

// ServiceTypeDef represents a Go struct in a service package that should become
// a GraphQL type. Unlike models, these are lightweight response types — no mixins,
// no CRUD, no filtering.
type ServiceTypeDef struct {
	Name        string     // Go struct name (e.g., "SwitchTenantResult")
	Fields      []FieldDef // Scalar fields
	PackageName string     // Go package name
	ImportPath  string     // Go import path
}

// ParseResult holds models, static mutations, and static queries.
type ParseResult struct {
	Models          []ModelDef
	StaticMutations []StaticMutationDef
	StaticQueries   []StaticQueryDef
	ServiceTypes    []ServiceTypeDef
}

// FieldDef represents a scalar field on a model.
type FieldDef struct {
	GoName       string
	GoType       string
	JSONName     string
	Description  string
	IsPointer    bool
	IsPrimaryKey bool
	GraphQLTag   string // raw graphql tag value
}

// RelationshipDef represents a relationship field (pointer-to-struct or slice-of-struct).
type RelationshipDef struct {
	GoName      string
	GoType      string // the struct name (without * or [])
	JSONName    string
	Description string
	IsList      bool
	GraphQLTag  string // raw graphql tag value (e.g., "preload", "preload:Tenant", "-")
}

// Known mixin types to skip as fields but track for metadata.
var knownMixins = map[string]bool{
	"BaseModel":       true,
	"TenantMixin":     true,
	"TrackedMixin":    true,
	"SoftDeleteMixin": true,
	"AuditedMixin":    true,
	"VersionedMixin":  true,
	"DirtyMixin":      true,
	"GraphQLMixin":    true,
}

// Fields contributed by each mixin.
var mixinFields = map[string][]FieldDef{
	"BaseModel": {
		{GoName: "CreatedAt", GoType: "time.Time", JSONName: "created_at", GraphQLTag: "readonly"},
		{GoName: "UpdatedAt", GoType: "time.Time", JSONName: "updated_at", GraphQLTag: "readonly"},
	},
	"TenantMixin": {
		{GoName: "TenantID", GoType: "uuid.UUID", JSONName: "tenant_id", GraphQLTag: "readonly"},
	},
	"TrackedMixin": {
		{GoName: "CreatedBy", GoType: "*uuid.UUID", JSONName: "created_by", IsPointer: true, GraphQLTag: "readonly"},
		{GoName: "UpdatedBy", GoType: "*uuid.UUID", JSONName: "updated_by", IsPointer: true, GraphQLTag: "readonly"},
	},
	"SoftDeleteMixin": {
		{GoName: "DeletedAt", GoType: "*time.Time", JSONName: "deleted_at", IsPointer: true, GraphQLTag: "-"},
		{GoName: "DeletedBy", GoType: "*uuid.UUID", JSONName: "deleted_by", IsPointer: true, GraphQLTag: "-"},
	},
	"VersionedMixin": {
		{GoName: "VersionLabel", GoType: "string", JSONName: "version_label"},
		{GoName: "Status", GoType: "string", JSONName: "status", GraphQLTag: "readonly"},
		{GoName: "AuthoredBy", GoType: "uuid.UUID", JSONName: "authored_by"},
		{GoName: "ChangeSummary", GoType: "*string", JSONName: "change_summary", IsPointer: true},
		{GoName: "PolicyCheckStatus", GoType: "*string", JSONName: "policy_check_status", IsPointer: true, GraphQLTag: "readonly"},
		{GoName: "PolicyCheckAt", GoType: "*time.Time", JSONName: "policy_check_at", IsPointer: true, GraphQLTag: "readonly"},
		{GoName: "PolicyCheckNote", GoType: "*string", JSONName: "policy_check_note", IsPointer: true, GraphQLTag: "readonly"},
	},
}

// ParseModels reads all .go files in the given directory and returns
// model definitions for structs that embed GraphQLMixin,
// plus any static mutations (package-level functions with // graphql:static annotations).
func ParseModels(dir string) (*ParseResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()

	modelMap := make(map[string]*ModelDef)
	var modelOrder []string
	var staticMutations []StaticMutationDef
	var staticQueries []StaticQueryDef

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}

		// Find struct type declarations
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}

			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}

				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}

				docComment := ""
				if typeSpec.Doc != nil {
					docComment = cleanComment(typeSpec.Doc.Text())
				} else if genDecl.Doc != nil {
					docComment = cleanComment(genDecl.Doc.Text())
				}

				model := parseStruct(typeSpec.Name.Name, docComment, structType)
				if model != nil {
					modelMap[model.Name] = model
					modelOrder = append(modelOrder, model.Name)
				}
			}
		}

		// Find methods annotated with // graphql:mutation
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Recv == nil {
				continue
			}

			recvType := receiverTypeName(funcDecl.Recv)
			if recvType == "" {
				continue
			}

			mutation := parseMutationAnnotation(funcDecl)
			if mutation == nil {
				continue
			}

			if model, ok := modelMap[recvType]; ok {
				model.Mutations = append(model.Mutations, *mutation)
			}
		}

		// Find package-level functions annotated with // graphql:mutation (class-level)
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Recv != nil { // skip methods (they have receivers)
				continue
			}

			staticMut := parseStaticMutationAnnotation(funcDecl)
			if staticMut != nil {
				staticMutations = append(staticMutations, *staticMut)
			}

			staticQuery := parseStaticQueryAnnotation(funcDecl)
			if staticQuery != nil {
				staticQueries = append(staticQueries, *staticQuery)
			}
		}
	}

	var models []ModelDef
	for _, name := range modelOrder {
		models = append(models, *modelMap[name])
	}
	return &ParseResult{Models: models, StaticMutations: staticMutations, StaticQueries: staticQueries}, nil
}

// ParseService reads all .go files in the given directory and extracts
// static query/mutation annotations and struct types used as return values.
func ParseService(dir string, importPath string) ([]StaticQueryDef, []StaticMutationDef, []ServiceTypeDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, nil, err
	}

	fset := token.NewFileSet()
	var queries []StaticQueryDef
	var mutations []StaticMutationDef
	structDefs := make(map[string]ServiceTypeDef) // all structs found in the package

	// Derive package name from the last segment of the directory
	pkgName := filepath.Base(dir)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil, nil, nil, err
		}

		// Use the actual package name from the file
		if file.Name != nil {
			pkgName = file.Name.Name
		}

		// Collect struct type definitions
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				st := parseServiceStruct(typeSpec.Name.Name, structType)
				if st != nil {
					st.PackageName = pkgName
					st.ImportPath = importPath
					structDefs[st.Name] = *st
				}
			}
		}

		// Collect annotated functions
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Recv != nil {
				continue
			}

			if q := parseStaticQueryAnnotation(funcDecl); q != nil {
				q.PackageName = pkgName
				q.ImportPath = importPath
				queries = append(queries, *q)
			}

			if m := parseStaticMutationAnnotation(funcDecl); m != nil {
				m.PackageName = pkgName
				m.ImportPath = importPath
				mutations = append(mutations, *m)
			}
		}
	}

	// Only include structs that are referenced as return types
	returnTypes := make(map[string]bool)
	for _, q := range queries {
		returnTypes[q.ReturnType] = true
	}
	for _, m := range mutations {
		returnTypes[m.ReturnType] = true
	}

	var serviceTypes []ServiceTypeDef
	for name, st := range structDefs {
		if returnTypes[name] {
			serviceTypes = append(serviceTypes, st)
		}
	}

	return queries, mutations, serviceTypes, nil
}

// parseServiceStruct parses a struct into a ServiceTypeDef by extracting
// exported fields with json tags.
func parseServiceStruct(name string, st *ast.StructType) *ServiceTypeDef {
	var fields []FieldDef

	for _, field := range st.Fields.List {
		if len(field.Names) == 0 || !ast.IsExported(field.Names[0].Name) {
			continue
		}

		fieldName := field.Names[0].Name
		tags := parseStructTags(field.Tag)
		jsonTag := tags["json"]
		jsonName := parseJSONName(jsonTag)
		if jsonName == "" || jsonName == "-" {
			continue
		}

		goType := exprTypeName(field.Type)
		isPointer := strings.HasPrefix(goType, "*")

		fields = append(fields, FieldDef{
			GoName:    fieldName,
			GoType:    goType,
			JSONName:  jsonName,
			IsPointer: isPointer,
		})
	}

	if len(fields) == 0 {
		return nil
	}

	return &ServiceTypeDef{
		Name:   name,
		Fields: fields,
	}
}

// receiverTypeName extracts the type name from a method receiver.
// Handles both value receivers (v Type) and pointer receivers (v *Type).
func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	t := recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if ident, ok := t.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// parseMutationAnnotation checks if a function declaration has a
// // graphql:mutation annotation and extracts the mutation definition.
func parseMutationAnnotation(funcDecl *ast.FuncDecl) *MutationDef {
	if funcDecl.Doc == nil {
		return nil
	}

	for _, comment := range funcDecl.Doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))

		if !strings.HasPrefix(text, "graphql:mutation") {
			continue
		}

		mutation := &MutationDef{
			MethodName: funcDecl.Name.Name,
		}

		// Extract description from the remaining doc comments (skip the annotation line)
		var descLines []string
		for _, c := range funcDecl.Doc.List {
			ct := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if !strings.HasPrefix(ct, "graphql:") {
				descLines = append(descLines, ct)
			}
		}
		mutation.Description = strings.Join(descLines, " ")

		// Parse parameters from annotation: graphql:mutation:name(param1 type1, param2 type2)
		after := strings.TrimPrefix(text, "graphql:mutation")
		if strings.HasPrefix(after, ":") {
			after = strings.TrimPrefix(after, ":")
			// Extract params from parentheses
			if lparen := strings.Index(after, "("); lparen >= 0 {
				rparen := strings.Index(after, ")")
				if rparen > lparen {
					paramStr := after[lparen+1 : rparen]
					for _, p := range strings.Split(paramStr, ",") {
						p = strings.TrimSpace(p)
						parts := strings.Fields(p)
						if len(parts) == 2 {
							mutation.Params = append(mutation.Params, ParamDef{
								Name:   parts[0],
								GoType: parts[1],
							})
						}
					}
				}
			}
		}

		return mutation
	}

	return nil
}

// parseStaticQueryAnnotation checks if a package-level function has a
// // graphql:query:queryName(params) ReturnType annotation.
func parseStaticQueryAnnotation(funcDecl *ast.FuncDecl) *StaticQueryDef {
	if funcDecl.Doc == nil {
		return nil
	}

	for _, comment := range funcDecl.Doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))

		if !strings.HasPrefix(text, "graphql:query:") {
			continue
		}

		after := strings.TrimPrefix(text, "graphql:query:")

		q := &StaticQueryDef{
			FuncName: funcDecl.Name.Name,
		}

		lparen := strings.Index(after, "(")
		if lparen < 0 {
			continue
		}
		q.QueryName = after[:lparen]

		rparen := strings.Index(after, ")")
		if rparen < 0 {
			continue
		}

		paramStr := after[lparen+1 : rparen]
		if paramStr != "" {
			for _, p := range strings.Split(paramStr, ",") {
				p = strings.TrimSpace(p)
				parts := strings.Fields(p)
				if len(parts) == 2 {
					q.Params = append(q.Params, ParamDef{Name: parts[0], GoType: parts[1]})
				}
			}
		}

		returnPart := strings.TrimSpace(after[rparen+1:])
		if strings.Contains(returnPart, "@public") {
			q.IsPublic = true
			returnPart = strings.TrimSpace(strings.ReplaceAll(returnPart, "@public", ""))
		}
		if strings.HasPrefix(returnPart, "[]") {
			q.ReturnsList = true
			q.ReturnType = strings.TrimPrefix(returnPart, "[]")
		} else {
			q.ReturnType = returnPart
		}

		var descLines []string
		for _, c := range funcDecl.Doc.List {
			ct := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if !strings.HasPrefix(ct, "graphql:") {
				descLines = append(descLines, ct)
			}
		}
		q.Description = strings.Join(descLines, " ")

		return q
	}

	return nil
}

// parseStaticMutationAnnotation checks if a package-level function has a
// // graphql:mutation:mutationName(params) ReturnType annotation.
// Same annotation as instance methods — the generator infers class-level
// behavior from the fact that the function has no receiver.
func parseStaticMutationAnnotation(funcDecl *ast.FuncDecl) *StaticMutationDef {
	if funcDecl.Doc == nil {
		return nil
	}

	for _, comment := range funcDecl.Doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))

		if !strings.HasPrefix(text, "graphql:mutation:") {
			continue
		}

		after := strings.TrimPrefix(text, "graphql:mutation:")

		mut := &StaticMutationDef{
			FuncName: funcDecl.Name.Name,
		}

		// Parse: mutationName(param1 type1, param2 type2) ReturnType
		// Find the mutation name (before the parenthesis)
		lparen := strings.Index(after, "(")
		if lparen < 0 {
			continue
		}
		mut.MutationName = after[:lparen]

		rparen := strings.Index(after, ")")
		if rparen < 0 {
			continue
		}

		// Parse parameters
		paramStr := after[lparen+1 : rparen]
		for _, p := range strings.Split(paramStr, ",") {
			p = strings.TrimSpace(p)
			parts := strings.Fields(p)
			if len(parts) == 2 {
				mut.Params = append(mut.Params, ParamDef{
					Name:   parts[0],
					GoType: parts[1],
				})
			}
		}

		// Parse return type (after the closing paren)
		returnPart := strings.TrimSpace(after[rparen+1:])
		if strings.Contains(returnPart, "@public") {
			mut.IsPublic = true
			returnPart = strings.TrimSpace(strings.ReplaceAll(returnPart, "@public", ""))
		}
		if strings.HasPrefix(returnPart, "[]") {
			mut.ReturnsList = true
			mut.ReturnType = strings.TrimPrefix(returnPart, "[]")
		} else {
			mut.ReturnType = returnPart
		}

		// Extract description
		var descLines []string
		for _, c := range funcDecl.Doc.List {
			ct := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if !strings.HasPrefix(ct, "graphql:") {
				descLines = append(descLines, ct)
			}
		}
		mut.Description = strings.Join(descLines, " ")

		return mut
	}

	return nil
}

func parseStruct(name string, description string, st *ast.StructType) *ModelDef {
	hasGraphQLMixin := false
	var mixins []string
	var fields []FieldDef
	var rels []RelationshipDef

	for _, field := range st.Fields.List {
		// Extract field comment (doc comment above the field, or inline comment)
		fieldDesc := ""
		if field.Doc != nil {
			fieldDesc = cleanComment(field.Doc.Text())
		} else if field.Comment != nil {
			fieldDesc = cleanComment(field.Comment.Text())
		}

		// Anonymous (embedded) fields — check for mixins
		if len(field.Names) == 0 {
			typeName := exprTypeName(field.Type)
			shortName := shortTypeName(typeName)

			if shortName == "GraphQLMixin" {
				hasGraphQLMixin = true
			}

			if knownMixins[shortName] {
				mixins = append(mixins, shortName)
				continue
			}
		}

		// Named fields
		if len(field.Names) == 0 {
			continue
		}

		fieldName := field.Names[0].Name
		if !ast.IsExported(fieldName) {
			continue
		}

		tags := parseStructTags(field.Tag)
		jsonTag := tags["json"]
		gormTag := tags["gorm"]
		graphqlTag := tags["graphql"]

		// Parse JSON name
		jsonName := parseJSONName(jsonTag)
		if jsonName == "" || jsonName == "-" {
			continue
		}

		goType := exprTypeName(field.Type)
		isPointer := strings.HasPrefix(goType, "*")
		baseType := strings.TrimPrefix(goType, "*")
		baseType = strings.TrimPrefix(baseType, "[]")

		// Determine if this is a relationship (pointer-to-struct or slice-of-struct)
		if isRelationship(field.Type) {
			isList := strings.HasPrefix(goType, "[]")
			relType := baseType
			relType = strings.TrimPrefix(relType, "*")
			rels = append(rels, RelationshipDef{
				GoName:      fieldName,
				GoType:      relType,
				JSONName:    jsonName,
				Description: fieldDesc,
				IsList:      isList,
				GraphQLTag:  graphqlTag,
			})
			continue
		}

		// Scalar field
		isPK := strings.Contains(gormTag, "primaryKey")

		fields = append(fields, FieldDef{
			GoName:       fieldName,
			GoType:       goType,
			JSONName:     jsonName,
			Description:  fieldDesc,
			IsPointer:    isPointer,
			IsPrimaryKey: isPK,
			GraphQLTag:   graphqlTag,
		})
	}

	if !hasGraphQLMixin {
		return nil
	}

	// Add mixin fields
	for _, mixin := range mixins {
		if mf, ok := mixinFields[mixin]; ok {
			fields = append(fields, mf...)
		}
	}

	skipCRUD := strings.Contains(description, "graphql:skipCrud")
	if skipCRUD {
		// Strip the annotation token from the description so generated docs stay clean.
		description = strings.TrimSpace(strings.ReplaceAll(description, "graphql:skipCrud", ""))
	}

	return &ModelDef{
		Name:          name,
		Description:   description,
		Fields:        fields,
		Relationships: rels,
		Mixins:        mixins,
		SkipCRUD:      skipCRUD,
	}
}

// cleanComment strips leading/trailing whitespace and removes Go comment prefixes.
func cleanComment(text string) string {
	text = strings.TrimSpace(text)
	// Remove lines that are just the struct name repeated (go/ast includes "TypeName ..." prefix sometimes)
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || isSectionHeader(line) {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.Join(cleaned, " ")
}

// isSectionHeader returns true if the line is a decorative section header
// like "── Identity ─────" that uses box-drawing characters.
func isSectionHeader(line string) bool {
	return strings.Contains(line, "──")
}

func isRelationship(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.StarExpr:
		// *SomeStruct — check if the inner type is a struct identifier (not a basic type)
		inner := exprTypeName(t.X)
		return isModelType(inner)
	case *ast.ArrayType:
		// []SomeStruct
		return true
	}
	return false
}

// isModelType returns true if the type name looks like a model (starts with uppercase, not a known scalar)
func isModelType(name string) bool {
	scalars := map[string]bool{
		"string": true, "int": true, "float64": true, "bool": true,
		"time.Time": true, "uuid.UUID": true, "json.RawMessage": true,
		"decimal.Decimal": true, "gorm.DeletedAt": true,
	}
	return !scalars[name] && len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
}

func exprTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return exprTypeName(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + exprTypeName(t.X)
	case *ast.ArrayType:
		return "[]" + exprTypeName(t.Elt)
	}
	return ""
}

func shortTypeName(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		return name[idx+1:]
	}
	return name
}

func parseStructTags(tagLit *ast.BasicLit) map[string]string {
	result := make(map[string]string)
	if tagLit == nil {
		return result
	}

	raw := tagLit.Value
	// Remove backticks
	if len(raw) >= 2 {
		raw = raw[1 : len(raw)-1]
	}

	tag := reflect.StructTag(raw)
	for _, key := range []string{"json", "gorm", "graphql"} {
		if v, ok := tag.Lookup(key); ok {
			result[key] = v
		}
	}
	return result
}

func parseJSONName(tag string) string {
	if tag == "" {
		return ""
	}
	parts := strings.Split(tag, ",")
	return parts[0]
}
