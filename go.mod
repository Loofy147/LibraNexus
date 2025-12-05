module libranexus

go 1.24.3

require (
	github.com/google/uuid v1.6.0
	github.com/jules-labs/go-chaos v0.0.0-00010101000000-000000000000
	github.com/jules-labs/go-eventstore v0.0.0-00010101000000-000000000000
	github.com/lib/pq v1.10.9
	github.com/stretchr/testify v1.11.1
	golang.org/x/crypto v0.45.0
	golang.org/x/time v0.14.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/metric v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/jules-labs/go-eventstore => ./go-eventstore

replace github.com/jules-labs/go-chaos => ./go-chaos
