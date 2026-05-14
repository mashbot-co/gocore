package main

import (
	"fmt"
	"strings"
)

// goTypeToGraphQL maps Go types to GraphQL scalar types. Slice types like
// []string become non-null lists of non-null elements (`[String!]!`).
func goTypeToGraphQL(goType string, isPointer bool) string {
	if strings.HasPrefix(goType, "[]") {
		inner := goTypeToGraphQL(strings.TrimPrefix(goType, "[]"), false)
		return "[" + inner + "]!"
	}

	base := strings.TrimPrefix(goType, "*")

	var gqlType string
	switch base {
	case "string":
		gqlType = "String"
	case "int":
		gqlType = "Int"
	case "float64":
		gqlType = "Float"
	case "bool":
		gqlType = "Boolean"
	case "uuid.UUID":
		gqlType = "UUID"
	case "time.Time":
		gqlType = "DateTime"
	case "json.RawMessage":
		gqlType = "JSON"
	case "decimal.Decimal":
		gqlType = "Float"
	default:
		gqlType = "String"
	}

	if isPointer || strings.HasPrefix(goType, "*") {
		return gqlType
	}
	return gqlType + "!"
}

// toCamelCase converts snake_case to camelCase for GraphQL field names.
func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// toGraphQLName converts a PascalCase Go struct name to a camelCase GraphQL query name.
// Handles common acronyms: API → api, ID → id, URL → url.
func toGraphQLName(name string) string {
	snake := toSnakeCase(name)
	return toCamelCase(snake)
}

// pluralize applies basic English pluralization rules.
func pluralize(s string) string {
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "x") || strings.HasSuffix(s, "sh") || strings.HasSuffix(s, "ch") {
		return s + "es"
	}
	if strings.HasSuffix(s, "y") && len(s) > 1 {
		prev := s[len(s)-2]
		if prev != 'a' && prev != 'e' && prev != 'i' && prev != 'o' && prev != 'u' {
			return s[:len(s)-1] + "ies"
		}
	}
	return s + "s"
}

// GenerateSchema produces GraphQL schema files from parsed model definitions.
func GenerateSchema(models []ModelDef) map[string]string {
	files := make(map[string]string)

	files["types.graphql"] = generateTypes(models)
	files["inputs.graphql"] = generateInputs(models)
	files["queries.graphql"] = generateQueries(models)
	files["mutations.graphql"] = generateMutations(models)
	files["custom_mutations.graphql"] = generateCustomMutations(models)
	files["connections.graphql"] = generateConnections(models)

	return files
}

// writeDescription writes a GraphQL description string (quoted) if non-empty.
func writeDescription(sb *strings.Builder, desc string, indent string) {
	if desc == "" {
		return
	}
	// Escape quotes in the description
	desc = strings.ReplaceAll(desc, `"`, `\"`)
	sb.WriteString(fmt.Sprintf("%s\"%s\"\n", indent, desc))
}

func generateTypes(models []ModelDef) string {
	var sb strings.Builder
	sb.WriteString("# AUTO-GENERATED — do not edit. Generated from Go models by tools/gqlgen-schema.\n\n")

	for _, m := range models {
		writeDescription(&sb, m.Description, "")
		sb.WriteString(fmt.Sprintf("type %s {\n", m.Name))

		for _, f := range m.Fields {
			if f.GraphQLTag == "-" {
				continue
			}
			gqlName := toCamelCase(f.JSONName)
			gqlType := goTypeToGraphQL(f.GoType, f.IsPointer)
			writeDescription(&sb, f.Description, "  ")
			sb.WriteString(fmt.Sprintf("  %s: %s\n", gqlName, gqlType))
		}

		for _, r := range m.Relationships {
			gqlName := toCamelCase(r.JSONName)
			writeDescription(&sb, r.Description, "  ")
			if r.IsList {
				sb.WriteString(fmt.Sprintf("  %s: [%s!]!\n", gqlName, r.GoType))
			} else {
				sb.WriteString(fmt.Sprintf("  %s: %s\n", gqlName, r.GoType))
			}
		}

		sb.WriteString("}\n\n")
	}

	return sb.String()
}

func generateInputs(models []ModelDef) string {
	var sb strings.Builder
	sb.WriteString("# AUTO-GENERATED — do not edit. Generated from Go models by tools/gqlgen-schema.\n\n")

	for _, m := range models {
		// Create input — writable fields only
		createFields := writableFields(m)
		if len(createFields) > 0 {
			writeDescription(&sb, fmt.Sprintf("Input for creating a new %s.", m.Name), "")
			sb.WriteString(fmt.Sprintf("input Create%sInput {\n", m.Name))
			for _, f := range createFields {
				gqlName := toCamelCase(f.JSONName)
				gqlType := goTypeToGraphQL(f.GoType, true)
				if !f.IsPointer {
					gqlType = goTypeToGraphQL(f.GoType, false)
				}
				writeDescription(&sb, f.Description, "  ")
				sb.WriteString(fmt.Sprintf("  %s: %s\n", gqlName, gqlType))
			}
			sb.WriteString("}\n\n")
		}

		// Update input — all fields optional
		if len(createFields) > 0 {
			writeDescription(&sb, fmt.Sprintf("Input for updating an existing %s. All fields are optional.", m.Name), "")
			sb.WriteString(fmt.Sprintf("input Update%sInput {\n", m.Name))
			for _, f := range createFields {
				gqlName := toCamelCase(f.JSONName)
				gqlType := goTypeToGraphQL(f.GoType, true)
				writeDescription(&sb, f.Description, "  ")
				sb.WriteString(fmt.Sprintf("  %s: %s\n", gqlName, gqlType))
			}
			sb.WriteString("}\n\n")
		}

		// Filter input — filterable fields
		filterFields := filterableFields(m)
		if len(filterFields) > 0 {
			writeDescription(&sb, fmt.Sprintf("Filter criteria for querying %s records. All fields are optional and combined with AND logic.", pluralize(m.Name)), "")
			sb.WriteString(fmt.Sprintf("input %sFilter {\n", m.Name))
			for _, f := range filterFields {
				gqlName := toCamelCase(f.JSONName)
				gqlType := goTypeToGraphQL(f.GoType, true)
				filterDesc := inferFilterDescription(f)
				writeDescription(&sb, filterDesc, "  ")
				sb.WriteString(fmt.Sprintf("  %s: %s\n", gqlName, gqlType))
			}
			sb.WriteString("}\n\n")
		}
	}

	return sb.String()
}

func generateQueries(models []ModelDef) string {
	// First pass: do any of the models actually contribute auto queries?
	// When all models opt out via skipQueries the GraphQL parser would
	// choke on an empty `type Query { }`. Emit a comment-only file in
	// that case; consumers declare `type Query` themselves in a custom
	// schema file (typically graph/schema/custom/_base.graphql).
	hasAny := false
	for _, m := range models {
		if primaryKeyField(m) != nil && !m.SkipQueries {
			hasAny = true
			break
		}
	}
	if !hasAny {
		return "# AUTO-GENERATED — no auto queries to emit; all models opted out via // graphql:skipQueries.\n# The root `type Query` must be declared in a custom schema file (e.g. graph/schema/custom/_base.graphql).\n"
	}

	var sb strings.Builder
	sb.WriteString("# AUTO-GENERATED — do not edit. Generated from Go models by tools/gqlgen-schema.\n\n")
	sb.WriteString("type Query {\n")

	for _, m := range models {
		pkField := primaryKeyField(m)
		if pkField == nil {
			continue
		}
		// Models that opt out of auto queries supply hand-written resolvers
		// and frequently a hand-written schema entry in graph/schema/custom/.
		if m.SkipQueries {
			continue
		}

		singular := toGraphQLName(m.Name)
		plural := pluralize(singular)
		humanName := toHumanReadable(m.Name)

		// Single by ID
		writeDescription(&sb, fmt.Sprintf("Retrieve a single %s by its unique identifier.", humanName), "  ")
		sb.WriteString(fmt.Sprintf("  %s(%s: UUID!): %s\n", singular, pkParamName(m), m.Name))

		// List with optional filter and pagination
		hasFilter := len(filterableFields(m)) > 0
		if hasFilter {
			writeDescription(&sb, fmt.Sprintf("List %s with optional filtering and pagination.", pluralize(humanName)), "  ")
			sb.WriteString(fmt.Sprintf("  %s(filter: %sFilter, pagination: PaginationInput): %sList!\n", plural, m.Name, m.Name))
		} else {
			writeDescription(&sb, fmt.Sprintf("List %s with pagination.", pluralize(humanName)), "  ")
			sb.WriteString(fmt.Sprintf("  %s(pagination: PaginationInput): %sList!\n", plural, m.Name))
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

func generateMutations(models []ModelDef) string {
	var sb strings.Builder
	sb.WriteString("# AUTO-GENERATED — do not edit. Generated from Go models by tools/gqlgen-schema.\n\n")
	sb.WriteString("type Mutation {\n")

	for _, m := range models {
		createFields := writableFields(m)
		if len(createFields) == 0 {
			continue
		}

		humanName := toHumanReadable(m.Name)

		writeDescription(&sb, fmt.Sprintf("Create a new %s.", humanName), "  ")
		sb.WriteString(fmt.Sprintf("  create%s(input: Create%sInput!): %s!\n", m.Name, m.Name, m.Name))
		writeDescription(&sb, fmt.Sprintf("Update an existing %s by ID.", humanName), "  ")
		sb.WriteString(fmt.Sprintf("  update%s(%s: UUID!, input: Update%sInput!): %s!\n", m.Name, pkParamName(m), m.Name, m.Name))
		writeDescription(&sb, fmt.Sprintf("Delete a %s by ID.", humanName), "  ")
		sb.WriteString(fmt.Sprintf("  delete%s(%s: UUID!): Boolean!\n", m.Name, pkParamName(m)))
	}

	sb.WriteString("}\n")
	return sb.String()
}

func generateCustomMutations(models []ModelDef) string {
	var sb strings.Builder
	sb.WriteString("# AUTO-GENERATED — do not edit. Generated from // graphql:mutation annotations on Go model methods.\n\n")

	// Check if any models have mutations
	hasMutations := false
	for _, m := range models {
		if len(m.Mutations) > 0 {
			hasMutations = true
			break
		}
	}
	if !hasMutations {
		return sb.String()
	}

	sb.WriteString("extend type Mutation {\n")

	for _, m := range models {
		for _, mut := range m.Mutations {
			// Build the mutation name: lowercase first char of method + model name
			// e.g., AgentVersion.Promote → promoteAgentVersion
			// e.g., SkillVersion.Activate → activateSkillVersion
			mutName := toCamelCase(toSnakeCase(mut.MethodName)) + m.Name
			mutName = strings.ToLower(mutName[:1]) + mutName[1:]

			// Build parameter list — always starts with the model's PK arg.
			params := pkParamName(m) + ": UUID!"
			for _, p := range mut.Params {
				gqlType := goTypeToGraphQL(p.GoType, false)
				params += fmt.Sprintf(", %s: %s", p.Name, gqlType)
			}

			// Description
			if mut.Description != "" {
				writeDescription(&sb, mut.Description, "  ")
			}
			sb.WriteString(fmt.Sprintf("  %s(%s): %s!\n", mutName, params, m.Name))
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

// GenerateStaticQueriesSchema produces GraphQL schema for class-level (static) queries.
func GenerateStaticQueriesSchema(queries []StaticQueryDef) string {
	var sb strings.Builder
	sb.WriteString("# AUTO-GENERATED — do not edit. Generated from // graphql:query annotations on Go functions.\n\n")

	if len(queries) == 0 {
		return sb.String()
	}

	sb.WriteString("extend type Query {\n")

	for _, q := range queries {
		var params []string
		for _, p := range q.Params {
			params = append(params, fmt.Sprintf("%s: %s", p.Name, goParamToGraphQL(p.GoType)))
		}

		if q.Description != "" {
			writeDescription(&sb, q.Description, "  ")
		}

		paramStr := ""
		if len(params) > 0 {
			paramStr = "(" + strings.Join(params, ", ") + ")"
		}

		if q.ReturnsList {
			sb.WriteString(fmt.Sprintf("  %s%s: [%s!]!\n", q.QueryName, paramStr, q.ReturnType))
		} else {
			sb.WriteString(fmt.Sprintf("  %s%s: %s\n", q.QueryName, paramStr, q.ReturnType))
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

// GenerateStaticMutationsSchema produces GraphQL schema for class-level (static) mutations.
func GenerateStaticMutationsSchema(statics []StaticMutationDef) string {
	var sb strings.Builder
	sb.WriteString("# AUTO-GENERATED — do not edit. Generated from // graphql:static annotations on Go functions.\n\n")

	if len(statics) == 0 {
		return sb.String()
	}

	sb.WriteString("extend type Mutation {\n")

	for _, s := range statics {
		// Build parameter list
		var params []string
		for _, p := range s.Params {
			gqlType := goParamToGraphQL(p.GoType)
			params = append(params, fmt.Sprintf("%s: %s", p.Name, gqlType))
		}

		if s.Description != "" {
			writeDescription(&sb, s.Description, "  ")
		}

		returnType := s.ReturnType
		if s.ReturnsList {
			sb.WriteString(fmt.Sprintf("  %s(%s): [%s!]!\n", s.MutationName, strings.Join(params, ", "), returnType))
		} else {
			sb.WriteString(fmt.Sprintf("  %s(%s): %s!\n", s.MutationName, strings.Join(params, ", "), returnType))
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

// GenerateServiceTypesSchema produces GraphQL type definitions for service return types.
func GenerateServiceTypesSchema(types []ServiceTypeDef) string {
	var sb strings.Builder
	sb.WriteString("# AUTO-GENERATED — do not edit. Generated from Go structs in service packages.\n\n")

	for _, t := range types {
		sb.WriteString(fmt.Sprintf("type %s {\n", t.Name))
		for _, f := range t.Fields {
			gqlName := toCamelCase(f.JSONName)
			gqlType := goTypeToGraphQL(f.GoType, f.IsPointer)
			sb.WriteString(fmt.Sprintf("  %s: %s\n", gqlName, gqlType))
		}
		sb.WriteString("}\n\n")
	}

	return sb.String()
}

// goParamToGraphQL converts a Go parameter type to a GraphQL type for schema generation.
func goParamToGraphQL(goType string) string {
	switch goType {
	case "string":
		return "String!"
	case "*string":
		return "String"
	case "uuid.UUID":
		return "UUID!"
	case "*uuid.UUID":
		return "UUID"
	case "int":
		return "Int!"
	case "*int":
		return "Int"
	case "bool":
		return "Boolean!"
	case "*bool":
		return "Boolean"
	case "time.Time":
		return "DateTime!"
	case "*time.Time":
		return "DateTime"
	case "[]uuid.UUID":
		return "[UUID!]!"
	case "[]string":
		return "[String!]!"
	case "[]int":
		return "[Int!]"
	default:
		return "String!"
	}
}

func generateConnections(models []ModelDef) string {
	var sb strings.Builder
	sb.WriteString("# AUTO-GENERATED — do not edit. Generated from Go models by tools/gqlgen-schema.\n\n")

	// Pagination input
	sb.WriteString(`"Cursor-based pagination parameters for list queries."
input PaginationInput {
  "Return the first N items after the cursor."
  first: Int
  "Cursor to start after (from a previous pageInfo.endCursor)."
  after: String
  "Return the last N items before the cursor."
  last: Int
  "Cursor to end before (from a previous pageInfo.startCursor)."
  before: String
}

"Pagination metadata returned with every list query."
type PageInfo {
  "Whether more items exist after the last item in this page."
  hasNextPage: Boolean!
  "Whether more items exist before the first item in this page."
  hasPreviousPage: Boolean!
  "Cursor of the first item in this page."
  startCursor: String
  "Cursor of the last item in this page."
  endCursor: String
  "Total number of items matching the query (across all pages)."
  totalCount: Int!
}

`)

	// Paginated list type per model
	for _, m := range models {
		humanName := toHumanReadable(m.Name)
		writeDescription(&sb, fmt.Sprintf("Paginated list of %s records.", pluralize(humanName)), "")
		sb.WriteString(fmt.Sprintf("type %sList {\n", m.Name))
		writeDescription(&sb, fmt.Sprintf("The %s records in this page.", humanName), "  ")
		sb.WriteString(fmt.Sprintf("  items: [%s!]!\n", m.Name))
		writeDescription(&sb, "Pagination metadata for navigating results.", "  ")
		sb.WriteString("  pageInfo: PageInfo!\n")
		sb.WriteString("}\n\n")
	}

	return sb.String()
}

// writableFields returns fields that should appear in create/update inputs.
// Excludes: primary keys, readonly, excluded, computed, and mixin-contributed readonly fields.
func writableFields(m ModelDef) []FieldDef {
	var result []FieldDef
	for _, f := range m.Fields {
		if f.IsPrimaryKey {
			continue
		}
		if f.GraphQLTag == "-" || f.GraphQLTag == "readonly" || f.GraphQLTag == "computed" {
			continue
		}
		result = append(result, f)
	}
	return result
}

// filterableFields returns fields suitable for filtering.
// Includes: ID fields (foreign keys), string fields with limited values (status, type, category),
// and fields explicitly tagged as filterable.
func filterableFields(m ModelDef) []FieldDef {
	var result []FieldDef
	for _, f := range m.Fields {
		if f.GraphQLTag == "-" {
			continue
		}
		if f.GraphQLTag == "filterable" {
			result = append(result, f)
			continue
		}
		// Auto-include foreign key UUID fields (not the PK)
		if !f.IsPrimaryKey && strings.HasSuffix(f.JSONName, "_id") {
			result = append(result, f)
			continue
		}
		// Auto-include status/type/category fields
		name := f.JSONName
		if name == "status" || name == "category" || strings.HasSuffix(name, "_type") || strings.HasSuffix(name, "_mode") {
			result = append(result, f)
			continue
		}
	}
	return result
}

func primaryKeyField(m ModelDef) *FieldDef {
	for _, f := range m.Fields {
		if f.IsPrimaryKey {
			return &f
		}
	}
	return nil
}

// pkParamName returns the GraphQL argument name to use for the single-key
// path of a model (get / update / delete / per-mutation-on-instance). It's
// the camelCase form of the PK's JSON column name — e.g. "membership_id" →
// "membershipId" — so the schema reads as `membership(membershipId: UUID!)`
// matching the type's field name rather than a bland `id`. Falls back to
// "id" only for models without a discoverable PK; the callers already guard
// on primaryKeyField != nil so the fallback is belt-and-braces.
func pkParamName(m ModelDef) string {
	if pk := primaryKeyField(m); pk != nil {
		return toCamelCase(pk.JSONName)
	}
	return "id"
}

// toHumanReadable converts PascalCase to a human-readable string.
// e.g., "AgentVersion" → "agent version", "APIKey" → "API key"
func toHumanReadable(name string) string {
	snake := toSnakeCase(name)
	return strings.ReplaceAll(snake, "_", " ")
}

// inferFilterDescription generates a description for a filter field based on its name and type.
func inferFilterDescription(f FieldDef) string {
	if f.Description != "" {
		return "Filter by " + strings.ToLower(f.Description[:1]) + f.Description[1:]
	}
	name := f.JSONName
	if strings.HasSuffix(name, "_id") {
		entity := strings.TrimSuffix(name, "_id")
		entity = strings.ReplaceAll(entity, "_", " ")
		return fmt.Sprintf("Filter by %s ID.", entity)
	}
	if name == "status" {
		return "Filter by current status."
	}
	if name == "category" {
		return "Filter by category."
	}
	if strings.HasSuffix(name, "_type") {
		entity := strings.TrimSuffix(name, "_type")
		entity = strings.ReplaceAll(entity, "_", " ")
		return fmt.Sprintf("Filter by %s type.", entity)
	}
	if strings.HasSuffix(name, "_mode") {
		entity := strings.TrimSuffix(name, "_mode")
		entity = strings.ReplaceAll(entity, "_", " ")
		return fmt.Sprintf("Filter by %s mode.", entity)
	}
	humanName := strings.ReplaceAll(name, "_", " ")
	return fmt.Sprintf("Filter by %s.", humanName)
}

// toSnakeCase converts PascalCase to snake_case.
// Handles acronyms: APIKey → api_key, UserID → user_id.
func toSnakeCase(s string) string {
	var result strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				prev := runes[i-1]
				if prev >= 'a' && prev <= 'z' {
					result.WriteByte('_')
				} else if prev >= 'A' && prev <= 'Z' && i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z' {
					result.WriteByte('_')
				}
			}
			result.WriteRune(r + 32)
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// GenerateBindings produces a Go file that maps GraphQL types to GORM models
// for gqlgen's model configuration.
func GenerateBindings(models []ModelDef, modelsImportPath string) string {
	var sb strings.Builder
	sb.WriteString("// Code generated by tools/gqlgen-schema. DO NOT EDIT.\n\n")
	sb.WriteString("package model\n\n")
	sb.WriteString("// This file documents the GraphQL-to-Go-model bindings.\n")
	sb.WriteString("// Configure these in gqlgen.yml under the 'models' section:\n")
	sb.WriteString("//\n")

	for _, m := range models {
		sb.WriteString(fmt.Sprintf("//   %s:\n", m.Name))
		sb.WriteString(fmt.Sprintf("//     model: %s.%s\n", modelsImportPath, m.Name))
	}

	sb.WriteString("//\n")
	sb.WriteString("// Input types (Create*Input, Update*Input, *Filter) are auto-generated by gqlgen.\n")

	return sb.String()
}
