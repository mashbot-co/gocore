# gocore

Shared Go utilities for Mashbot-built backend services.

A small, opinionated set of building blocks that every Go service we ship
benefits from: SSM-driven config, GORM connection with multi-tenant callbacks,
gormigrate runner, SendGrid mailer, composable model mixins, and a GraphQL
schema/resolver generator.

## Packages

| Path | Purpose |
|---|---|
| `config/` | Load environment variables from AWS SSM Parameter Store at cold start. |
| `connection/` | GORM connection (Postgres) with `OnInitialize` callback registry and `WithCurrentUser` / `WithCurrentTenant` context helpers. |
| `mailer/` | SendGrid v3 wrapper with template rendering. |
| `migrations/` | gormigrate runner (registry + Up / Down / RollbackTo). |
| `models/mixins/` | Composable GORM mixins: `BaseModel`, `TrackedMixin`, `SoftDeleteMixin`, `TenantMixin`, `AuditedMixin`, `DirtyMixin`, `VersionedMixin`, `GraphQLMixin`, plus a global UUID primary-key generator. |
| `tools/gqlgen/` | Code generator that scans GORM models marked with `GraphQLMixin` and emits GraphQL schema (types, queries, mutations, inputs, filters, pagination) plus stub resolvers wiring them to GORM via `Preload`-based eager loading. |

## Usage

Add as a dependency:

```sh
go get github.com/mashbot-co/gocore@latest
```

Compose a model with mixins:

```go
package models

import (
    "github.com/google/uuid"
    "github.com/mashbot-co/gocore/models/mixins"
)

type Project struct {
    ProjectID uuid.UUID `gorm:"type:uuid;primaryKey" json:"project_id"`
    Name      string    `gorm:"size:255;not null" json:"name"`

    mixins.BaseModel
    mixins.TenantMixin
    mixins.TrackedMixin
    mixins.SoftDeleteMixin
    mixins.AuditedMixin
    mixins.GraphQLMixin
}
```

Initialize the DB and let mixins self-register their GORM callbacks:

```go
import (
    db "github.com/mashbot-co/gocore/connection"
    _ "github.com/mashbot-co/gocore/models/mixins" // init() calls connection.OnInitialize
)

gormDB, err := db.Setup()
```

Generate GraphQL for all `GraphQLMixin`-tagged models in a package:

```sh
go run github.com/mashbot-co/gocore/tools/gqlgen \
  <models-package-dir> \
  <output-schema-dir>
```

## Development

The repo is a single Go module — `cd gocore && go test ./...` runs all tests.

For active development across consumer projects, use a Go workspace overlay
in the consumer's `go.work`:

```
use ../gocore   # local override; remove before pushing
```

While that line is present, all `github.com/mashbot-co/gocore/...` imports
resolve to your local checkout. Remove it to fall back to the version pinned
in `go.mod`.

## Versioning

`v0.x.y` — pre-stable. Minor versions may introduce breaking changes.
`v1.0.0` once the API surface has settled. Breaking changes after v1 require
a `v2` import path (`github.com/mashbot-co/gocore/v2/...`).
