.PHONY: test test-short test-pg cover cover-html cover-func check-coverage tidy

# Fast tests only (skip anything that needs Postgres).
test-short:
	go test -short -count=1 ./...

# All tests; assumes Postgres reachable at localhost:5432 with postgres/postgres.
test-pg:
	go test -count=1 ./...

# Default = fast tests.
test: test-short

# Per-package coverage summary.
cover:
	go test -short -cover ./...

# Function-level coverage report. Writes cov.out then prints the breakdown.
cover-func:
	go test -short -coverpkg=./... -coverprofile=cov.out ./...
	go tool cover -func=cov.out

# Browsable HTML coverage report. Writes cov.html.
cover-html:
	go test -short -coverpkg=./... -coverprofile=cov.out ./...
	go tool cover -html=cov.out -o cov.html
	@echo ""
	@echo "Open cov.html in a browser to inspect line-by-line coverage."

# Enforce per-package coverage threshold (default 90%).
# Usage: make check-coverage           — uses 90% default
#        make check-coverage T=80      — override threshold
check-coverage:
	@./scripts/check-coverage.sh $(or $(T),90)

# Tidy module deps.
tidy:
	go mod tidy
